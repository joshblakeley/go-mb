// Package bench orchestrates a complete benchmark run.
// It is the only package that imports all other internal packages.
package bench

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/redpanda-data/go-bench/internal/config"
	"github.com/redpanda-data/go-bench/internal/consumer"
	"github.com/redpanda-data/go-bench/internal/metrics"
	"github.com/redpanda-data/go-bench/internal/producer"
	"github.com/redpanda-data/go-bench/internal/reporter"
	"github.com/redpanda-data/go-bench/internal/topic"
)

// Run executes the benchmark described by cfg.
// It blocks until the benchmark completes or ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// 1. Topic management.
	if cfg.CreateTopic {
		fmt.Printf("Creating topic %q (%d partitions, RF=%d)...\n",
			cfg.Topic, cfg.Partitions, cfg.ReplicationFactor)
		if err := topic.Create(ctx, cfg.Brokers, cfg.Topic, cfg.Partitions, cfg.ReplicationFactor); err != nil {
			return fmt.Errorf("create topic: %w", err)
		}
	}

	rec := metrics.NewRecorder()

	// 2. Build kafka clients.
	// Producers and consumers use separate clients so each pool can tune
	// client options independently in future.
	producerClient, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.DefaultProduceTopic(cfg.Topic),
	)
	if err != nil {
		return fmt.Errorf("create producer client: %w", err)
	}
	defer producerClient.Close()

	consumerClient, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumeTopics(cfg.Topic),
		kgo.ConsumerGroup(cfg.ConsumerGroup),
	)
	if err != nil {
		return fmt.Errorf("create consumer client: %w", err)
	}
	defer consumerClient.Close()

	// 3. Warmup (optional).
	if cfg.WarmupDuration > 0 {
		fmt.Printf("----- Warming up for %s -----\n", cfg.WarmupDuration)
		wCtx, wCancel := context.WithTimeout(ctx, cfg.WarmupDuration)
		runWorkers(wCtx, cfg, producerClient, consumerClient, rec)
		wCancel()
		// Reset metrics after warmup.
		rec = metrics.NewRecorder()
	}

	// 4. Benchmark run.
	fmt.Printf("----- Starting benchmark (duration: %s, report interval: %s) -----\n",
		cfg.Duration, cfg.ReportInterval)

	runCtx, runCancel := context.WithTimeout(ctx, cfg.Duration)
	defer runCancel()

	var wg sync.WaitGroup

	// Reporter goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(cfg.ReportInterval)
		defer ticker.Stop()
		lastTick := time.Now()
		for {
			select {
			case t := <-ticker.C:
				elapsed := t.Sub(lastTick)
				lastTick = t
				snap := rec.Snapshot()
				reporter.PrintPeriod(snap, elapsed)
			case <-runCtx.Done():
				return
			}
		}
	}()

	runWorkers(runCtx, cfg, producerClient, consumerClient, rec)
	wg.Wait()

	// 5. Final report.
	fmt.Println()
	reporter.PrintFinal(rec.Cumulative())

	// 6. Cleanup.
	if cfg.DeleteTopic {
		fmt.Printf("Deleting topic %q...\n", cfg.Topic)
		if err := topic.Delete(context.Background(), cfg.Brokers, cfg.Topic); err != nil {
			fmt.Printf("Warning: failed to delete topic: %v\n", err)
		}
	}

	return nil
}

// runWorkers starts producer and consumer pools and blocks until ctx is done.
func runWorkers(ctx context.Context, cfg *config.Config, prod *kgo.Client, cons *kgo.Client, rec *metrics.Recorder) {
	var wg sync.WaitGroup

	if cfg.Producers > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			producer.RunPool(ctx, prod, rec, cfg.Producers, cfg.Topic, cfg.MessageSize, cfg.ProduceRate)
		}()
	}

	if cfg.Consumers > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumer.RunPool(ctx, cons, rec, cfg.Consumers)
		}()
	}

	wg.Wait()
}
