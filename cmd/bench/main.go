package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/redpanda-data/go-bench/internal/bench"
	"github.com/redpanda-data/go-bench/internal/config"
)

func main() {
	root := &cobra.Command{
		Use:   "bench",
		Short: "Kafka/Redpanda single-node benchmark tool",
	}
	root.AddCommand(newRunCommand())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRunCommand() *cobra.Command {
	cfg := config.Default()
	var brokersFlag string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a benchmark against a Kafka/Redpanda cluster",
		Example: `  bench run --brokers localhost:9092 --producers 4 --consumers 4 --partitions 16 --duration 60s
  bench run --brokers broker1:9092,broker2:9092 --message-size 4096 --rate 50000 --duration 5m
  bench run --brokers localhost:9092 --duration 60s --output report.html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Brokers = strings.Split(brokersFlag, ",")
			return bench.Run(context.Background(), &cfg)
		},
	}

	f := cmd.Flags()
	f.StringVar(&brokersFlag, "brokers", "localhost:9092", "Comma-separated broker addresses")
	f.StringVar(&cfg.Topic, "topic", cfg.Topic, "Topic name")
	f.IntVar(&cfg.Partitions, "partitions", cfg.Partitions, "Number of partitions (used when creating the topic)")
	f.IntVar(&cfg.ReplicationFactor, "replication-factor", cfg.ReplicationFactor, "Replication factor (used when creating the topic)")
	f.IntVar(&cfg.Producers, "producers", cfg.Producers, "Number of concurrent producer goroutines")
	f.IntVar(&cfg.Consumers, "consumers", cfg.Consumers, "Number of concurrent consumer goroutines")
	f.IntVar(&cfg.MessageSize, "message-size", cfg.MessageSize, "Message size in bytes (min 8)")
	f.IntVar(&cfg.ProduceRate, "rate", cfg.ProduceRate, "Target produce rate in msg/s per producer (0 = unlimited)")
	f.DurationVar(&cfg.Duration, "duration", cfg.Duration, "Benchmark duration")
	f.DurationVar(&cfg.WarmupDuration, "warmup", cfg.WarmupDuration, "Warmup duration before recording metrics (0 = no warmup)")
	f.DurationVar(&cfg.ReportInterval, "report-interval", cfg.ReportInterval, "How often to print periodic stats")
	f.StringVar(&cfg.ConsumerGroup, "consumer-group", cfg.ConsumerGroup, "Consumer group ID")
	f.BoolVar(&cfg.CreateTopic, "create-topic", cfg.CreateTopic, "Create topic before benchmark")
	f.BoolVar(&cfg.DeleteTopic, "delete-topic", cfg.DeleteTopic, "Delete topic after benchmark")
	f.StringVar(&cfg.OutputFile, "output", cfg.OutputFile, "path to write HTML report (e.g. report.html; empty = disabled)")
	f.StringVar(&cfg.Acks, "acks", cfg.Acks, `producer acks: "0" (none), "1" (leader), "all" (all ISR)`)
	f.StringVar(&cfg.Compression, "compression", cfg.Compression, `compression codec: none, gzip, snappy, lz4, zstd`)
	f.IntVar(&cfg.LingerMs, "linger-ms", cfg.LingerMs, "producer linger in milliseconds (0 = send immediately)")
	f.IntVar(&cfg.BatchMaxBytes, "batch-max-bytes", cfg.BatchMaxBytes, "max producer batch size in bytes (0 = franz-go default ~1MB)")

	return cmd
}
