// Package bench orchestrates a complete benchmark run.
// It is the only package that imports all other internal packages.
package bench

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"

	"github.com/joshblakeley/go-mb/internal/config"
	"github.com/joshblakeley/go-mb/internal/consumer"
	"github.com/joshblakeley/go-mb/internal/metrics"
	"github.com/joshblakeley/go-mb/internal/producer"
	"github.com/joshblakeley/go-mb/internal/reporter"
	"github.com/joshblakeley/go-mb/internal/results"
	"github.com/joshblakeley/go-mb/internal/tlsconfig"
	"github.com/joshblakeley/go-mb/internal/topic"
)

// Run executes the benchmark described by cfg.
// It blocks until the benchmark completes or ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Build shared TLS and SASL options applied to topic admin, producer, and consumer clients.
	// cfg.TLSCACert non-empty implies TLS regardless of cfg.TLS; this is resolved here rather
	// than in Validate() so the config layer validates field values and the bench layer resolves
	// derived semantics.
	tlsCfg, err := tlsconfig.Build(cfg.TLS || cfg.TLSCACert != "", cfg.TLSCACert)
	if err != nil {
		return fmt.Errorf("build TLS config: %w", err)
	}
	var sharedOpts []kgo.Opt
	if tlsCfg != nil {
		sharedOpts = append(sharedOpts, kgo.DialTLSConfig(tlsCfg))
	}
	sharedOpts = append(sharedOpts, saslOpts(cfg)...)

	// 1. Topic management.
	if cfg.CreateTopic {
		fmt.Printf("Creating topic %q (%d partitions, RF=%d)...\n",
			cfg.Topic, cfg.Partitions, cfg.ReplicationFactor)
		if err := topic.Create(ctx, cfg.Brokers, cfg.Topic, cfg.Partitions, cfg.ReplicationFactor, sharedOpts...); err != nil {
			return fmt.Errorf("create topic: %w", err)
		}
	}

	// expectedIntervalMicros is the per-producer send cadence used for coordinated
	// omission correction. We use the per-producer rate (not aggregate) because each
	// goroutine corrects its own stall independently — a stall in one goroutine does
	// not imply missed sends in the others.
	var expectedIntervalMicros int64
	if cfg.ProduceRate > 0 {
		expectedIntervalMicros = 1_000_000 / int64(cfg.ProduceRate)
		if expectedIntervalMicros < 1 {
			expectedIntervalMicros = 1 // clamp: rates above 1M msg/s round to 1µs minimum
		}
	}
	rec := metrics.NewRecorder(expectedIntervalMicros)

	// 2. Build kafka clients.
	// Producers and consumers use separate clients so each pool can tune
	// client options independently in future.
	prodOpts := append(
		append([]kgo.Opt{
			kgo.SeedBrokers(cfg.Brokers...),
			kgo.DefaultProduceTopic(cfg.Topic),
		}, producerOpts(cfg)...),
		sharedOpts...,
	)
	producerClient, err := kgo.NewClient(prodOpts...)
	if err != nil {
		return fmt.Errorf("create producer client: %w", err)
	}
	defer producerClient.Close()

	consumerClient, err := kgo.NewClient(
		append([]kgo.Opt{
			kgo.SeedBrokers(cfg.Brokers...),
			kgo.ConsumeTopics(cfg.Topic),
			kgo.ConsumerGroup(cfg.ConsumerGroup),
		}, sharedOpts...)...,
	)
	if err != nil {
		return fmt.Errorf("create consumer client: %w", err)
	}
	defer consumerClient.Close()

	// 3. Warmup (optional).
	if cfg.WarmupDuration > 0 {
		fmt.Printf("----- Warming up for %s -----\n", cfg.WarmupDuration)
		wCtx, wCancel := context.WithTimeout(ctx, cfg.WarmupDuration)
		defer wCancel() // belt-and-suspenders: guards against future early returns

		if cfg.ProduceRate > 0 {
			// Linear ramp: create workers at rate 1, ramp to target over warmup duration.
			workers := producer.NewPool(producerClient, rec, cfg.Producers, cfg.Topic, cfg.MessageSize, 1)

			totalSteps := int(cfg.WarmupDuration.Seconds())
			if totalSteps < 1 {
				totalSteps = 1
			}

			var wg sync.WaitGroup

			// Ramp goroutine: updates all worker rates every second.
			// Tracked in wg to guarantee it exits before rec is replaced.
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				step := 0
				for {
					select {
					case <-ticker.C:
						step++
						currentRate := cfg.ProduceRate * step / totalSteps
						if currentRate < 1 {
							currentRate = 1
						}
						for _, w := range workers {
							w.SetRate(currentRate)
						}
						if step >= totalSteps {
							return
						}
					case <-wCtx.Done():
						return
					}
				}
			}()

			// Consumer goroutine — no ramp needed; tracked in wg.
			if cfg.Consumers > 0 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					consumer.RunPool(wCtx, consumerClient, rec, cfg.Consumers)
				}()
			}

			// Workers start at rate 1; the ramp goroutine applies the first interpolated
			// rate after 1 second. This is intentional — the first second acts as an
			// additional soft start before the ramp begins.
			producer.StartPool(wCtx, workers)
			// wCtx has already expired here (StartPool returned because the context
			// timed out), so the ramp goroutine will select wCtx.Done() immediately
			// and exit without waiting for the next ticker fire.
			wg.Wait()
		} else {
			// No rate limit — run workers at full speed during warmup.
			runWorkers(wCtx, cfg, producerClient, consumerClient, rec)
		}

		wCancel() // eager release of timer resource
		// Reset metrics after warmup so benchmark measurements are clean.
		rec = metrics.NewRecorder(expectedIntervalMicros)
	}

	// 4. Benchmark run.
	fmt.Printf("----- Starting benchmark (duration: %s, report interval: %s) -----\n",
		cfg.Duration, cfg.ReportInterval)

	runCtx, runCancel := context.WithTimeout(ctx, cfg.Duration)
	defer runCancel()

	var wg sync.WaitGroup
	benchmarkStart := time.Now()
	var points []results.DataPoint

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
				points = append(points, results.DataPointFromSnapshot(snap, elapsed, time.Since(benchmarkStart).Seconds()))
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

	// HTML report (optional).
	if cfg.OutputFile != "" {
		run := results.Run{
			Meta:    results.RunMetaFromConfig(cfg, benchmarkStart),
			Points:  points,
			Summary: results.FinalSummaryFromSnapshot(rec.Cumulative()),
		}
		if err := results.WriteHTML(&run, cfg.OutputFile); err != nil {
			fmt.Printf("Warning: failed to write report: %v\n", err)
		} else {
			fmt.Printf("Report written to %s\n", cfg.OutputFile)
		}
	}

	// 6. Cleanup.
	if cfg.DeleteTopic {
		fmt.Printf("Deleting topic %q...\n", cfg.Topic)
		if err := topic.Delete(context.Background(), cfg.Brokers, cfg.Topic, sharedOpts...); err != nil {
			fmt.Printf("Warning: failed to delete topic: %v\n", err)
		}
	}

	return nil
}

// producerOpts translates producer tuning config into franz-go client options.
// The acks option is always set explicitly — even for "all" (the default) — to
// make the producer's durability contract visible in code rather than relying on
// the franz-go default. The int options are omitted when zero so franz-go's own
// defaults apply, consistent with their "0 = use default" semantics.
// BatchMaxBytes is safe to cast to int32: Validate() caps it at math.MaxInt32.
func producerOpts(cfg *config.Config) []kgo.Opt {
	var opts []kgo.Opt

	switch cfg.Acks {
	case "0":
		opts = append(opts, kgo.RequiredAcks(kgo.NoAck()))
	case "1":
		opts = append(opts, kgo.RequiredAcks(kgo.LeaderAck()))
	case "all":
		opts = append(opts, kgo.RequiredAcks(kgo.AllISRAcks()))
	}

	switch cfg.Compression {
	case "gzip":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.GzipCompression()))
	case "snappy":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.SnappyCompression()))
	case "lz4":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.Lz4Compression()))
	case "zstd":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.ZstdCompression()))
		// "none": omit — franz-go default is no compression
	}

	if cfg.LingerMs > 0 {
		opts = append(opts, kgo.ProducerLinger(time.Duration(cfg.LingerMs)*time.Millisecond))
	}
	if cfg.BatchMaxBytes > 0 {
		opts = append(opts, kgo.ProducerBatchMaxBytes(int32(cfg.BatchMaxBytes)))
	}

	return opts
}

// saslOpts returns the franz-go SASL option for the configured mechanism.
// Returns nil when SASLMechanism is empty (no authentication); nil spreads
// cleanly with ... in append so no nil-guard is needed at call sites.
func saslOpts(cfg *config.Config) []kgo.Opt {
	switch cfg.SASLMechanism {
	case "plain":
		return []kgo.Opt{kgo.SASL(plain.Auth{
			User: cfg.SASLUsername,
			Pass: cfg.SASLPassword,
		}.AsMechanism())}
	case "scram-sha-256":
		return []kgo.Opt{kgo.SASL(scram.Auth{
			User: cfg.SASLUsername,
			Pass: cfg.SASLPassword,
		}.AsSha256Mechanism())}
	case "scram-sha-512":
		return []kgo.Opt{kgo.SASL(scram.Auth{
			User: cfg.SASLUsername,
			Pass: cfg.SASLPassword,
		}.AsSha512Mechanism())}
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
