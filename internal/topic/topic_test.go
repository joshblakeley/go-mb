package topic_test

import (
	"testing"

	"github.com/joshblakeley/go-mb/internal/topic"
)

// TestConfigValidation checks that empty topic name is rejected.
func TestConfigValidation(t *testing.T) {
	err := topic.ValidateName("")
	if err == nil {
		t.Error("expected error for empty topic name")
	}
	err = topic.ValidateName("valid-topic")
	if err != nil {
		t.Errorf("unexpected error for valid topic name: %v", err)
	}
}

// Integration tests are skipped unless BENCH_BROKERS env var is set.
// Run with: BENCH_BROKERS=localhost:9092 go test ./internal/topic/ -v -run Integration
