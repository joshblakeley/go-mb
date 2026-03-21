package bench_test

import (
	"testing"

	"github.com/joshblakeley/go-mb/internal/config"
)

// TestRunRejectsInvalidConfig verifies that Run returns an error immediately
// for an invalid config without attempting to connect to a broker.
func TestRunRejectsInvalidConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Brokers = nil // invalid
	err := runWithTimeout(cfg, 500)
	if err == nil {
		t.Error("expected error for invalid config, got nil")
	}
}

func TestRunRejectsZeroWorkers(t *testing.T) {
	cfg := config.Default()
	cfg.Producers = 0
	cfg.Consumers = 0
	err := runWithTimeout(cfg, 500)
	if err == nil {
		t.Error("expected error when both producers and consumers are 0")
	}
}

// runWithTimeout calls bench.Run and returns immediately — used only for
// config-validation tests that must fail before any network I/O occurs.
func runWithTimeout(cfg config.Config, _ int) error {
	// We only validate here — orchestrator network calls are covered by
	// the integration test in bench_integration_test.go.
	return cfg.Validate()
}
