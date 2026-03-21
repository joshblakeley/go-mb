package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joshblakeley/go-mb/internal/bench"
	"github.com/joshblakeley/go-mb/internal/config"
)

func main() {
	root := &cobra.Command{
		Use:   "go-mb",
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
		Example: `  go-mb run --brokers localhost:9092 --producers 4 --consumers 4 --partitions 16 --duration 60s
  go-mb run --brokers broker1:9092,broker2:9092 --message-size 4096 --rate 50000 --duration 5m
  go-mb run --brokers localhost:9092 --duration 60s --output report.html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Brokers = strings.Split(brokersFlag, ",")
			return bench.Run(context.Background(), &cfg)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&brokersFlag, "brokers", "b", "localhost:9092", "Comma-separated broker addresses")
	f.StringVarP(&cfg.Topic, "topic", "t", cfg.Topic, "Topic name")
	f.IntVarP(&cfg.Partitions, "partitions", "p", cfg.Partitions, "Number of partitions (used when creating the topic)")
	f.IntVarP(&cfg.ReplicationFactor, "replication-factor", "r", cfg.ReplicationFactor, "Replication factor (used when creating the topic)")
	f.IntVarP(&cfg.Producers, "producers", "P", cfg.Producers, "Number of concurrent producer goroutines")
	f.IntVarP(&cfg.Consumers, "consumers", "C", cfg.Consumers, "Number of concurrent consumer goroutines")
	f.IntVarP(&cfg.MessageSize, "message-size", "s", cfg.MessageSize, "Message size in bytes (min 8)")
	f.IntVarP(&cfg.ProduceRate, "rate", "R", cfg.ProduceRate, "Target produce rate in msg/s per producer (0 = unlimited)")
	f.DurationVarP(&cfg.Duration, "duration", "d", cfg.Duration, "Benchmark duration")
	f.DurationVarP(&cfg.WarmupDuration, "warmup", "w", cfg.WarmupDuration, "Warmup duration before recording metrics (0 = no warmup)")
	f.DurationVar(&cfg.ReportInterval, "report-interval", cfg.ReportInterval, "How often to print periodic stats")
	f.StringVar(&cfg.ConsumerGroup, "consumer-group", cfg.ConsumerGroup, "Consumer group ID")
	f.BoolVar(&cfg.CreateTopic, "create-topic", cfg.CreateTopic, "Create topic before benchmark")
	f.BoolVar(&cfg.DeleteTopic, "delete-topic", cfg.DeleteTopic, "Delete topic after benchmark")
	f.StringVarP(&cfg.OutputFile, "output", "o", cfg.OutputFile, "path to write HTML report (e.g. report.html; empty = disabled)")
	f.StringVar(&cfg.Acks, "acks", cfg.Acks, `producer acks: "0" (none), "1" (leader), "all" (all ISR)`)
	f.StringVar(&cfg.Compression, "compression", cfg.Compression, `compression codec: none, gzip, snappy, lz4, zstd`)
	f.IntVar(&cfg.LingerMs, "linger-ms", cfg.LingerMs, "producer linger in milliseconds (0 = send immediately)")
	f.IntVar(&cfg.BatchMaxBytes, "batch-max-bytes", cfg.BatchMaxBytes, "max producer batch size in bytes (0 = franz-go default ~1MB)")
	f.BoolVar(&cfg.TLS, "tls", cfg.TLS, "enable TLS using system root CAs")
	f.StringVar(&cfg.TLSCACert, "tls-ca-cert", cfg.TLSCACert, "path to CA certificate PEM file (implies --tls)")
	f.StringVar(&cfg.SASLMechanism, "sasl-mechanism", cfg.SASLMechanism, "SASL mechanism: plain, scram-sha-256, scram-sha-512")
	f.StringVar(&cfg.SASLUsername, "sasl-username", cfg.SASLUsername, "SASL username")
	f.StringVar(&cfg.SASLPassword, "sasl-password", cfg.SASLPassword, "SASL password")

	return cmd
}
