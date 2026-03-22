//go:build integration

package bench_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joshblakeley/go-mb/internal/bench"
	"github.com/joshblakeley/go-mb/internal/config"
)

// Run with: BENCH_BROKERS=localhost:9092 go test ./internal/bench/ -tags integration -v -run TestIntegrationSmoke
func TestIntegrationSmoke(t *testing.T) {
	brokerStr := os.Getenv("BENCH_BROKERS")
	if brokerStr == "" {
		t.Skip("BENCH_BROKERS not set — skipping integration test")
	}

	cfg := config.Default()
	cfg.Brokers = strings.Split(brokerStr, ",")
	cfg.Topic = "bench-integration-test"
	cfg.Partitions = 4
	cfg.Producers = 2
	cfg.Consumers = 2
	cfg.MessageSize = 1024
	cfg.Duration = 15 * time.Second
	cfg.WarmupDuration = 5 * time.Second
	cfg.ReportInterval = 5 * time.Second
	cfg.CreateTopic = true
	cfg.DeleteTopic = true

	ctx := context.Background()
	if err := bench.Run(ctx, &cfg); err != nil {
		t.Fatalf("bench.Run failed: %v", err)
	}
}

// TestIntegration_WarmupRamp exercises the rate-ramp warmup path (ProduceRate > 0).
// Run with: BENCH_BROKERS=localhost:9092 go test ./internal/bench/ -tags integration -v -run TestIntegration_WarmupRamp
func TestIntegration_WarmupRamp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	brokerStr := os.Getenv("BENCH_BROKERS")
	if brokerStr == "" {
		t.Skip("BENCH_BROKERS not set — skipping integration test")
	}

	cfg := config.Default()
	cfg.Brokers = strings.Split(brokerStr, ",")
	cfg.Topic = "integration-ramp-test"
	cfg.Producers = 1
	cfg.Consumers = 1
	cfg.MessageSize = 256
	cfg.ProduceRate = 500              // rate > 0 triggers ramp
	cfg.ReplicationFactor = 1          // single-node broker
	cfg.WarmupDuration = 5 * time.Second
	cfg.Duration = 10 * time.Second
	cfg.ReportInterval = 5 * time.Second
	cfg.CreateTopic = true
	cfg.DeleteTopic = true

	if err := bench.Run(context.Background(), &cfg); err != nil {
		t.Fatalf("bench.Run: %v", err)
	}
}
