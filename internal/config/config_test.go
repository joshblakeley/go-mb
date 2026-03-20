package config_test

import (
	"testing"
	"time"

	"github.com/redpanda-data/go-bench/internal/config"
)

func TestDefaultIsValid(t *testing.T) {
	cfg := config.Default()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Default config should be valid, got: %v", err)
	}
}

func TestValidateRequiresBrokers(t *testing.T) {
	cfg := config.Default()
	cfg.Brokers = nil
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when brokers is empty")
	}
}

func TestValidateRequiresTopic(t *testing.T) {
	cfg := config.Default()
	cfg.Topic = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when topic is empty")
	}
}

func TestValidateMessageSizeMin(t *testing.T) {
	cfg := config.Default()
	cfg.MessageSize = 4
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when message-size < 8")
	}
}

func TestValidateRequiresWorkers(t *testing.T) {
	cfg := config.Default()
	cfg.Producers = 0
	cfg.Consumers = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when both producers and consumers are 0")
	}
}

func TestValidateDuration(t *testing.T) {
	cfg := config.Default()
	cfg.Duration = -1 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for non-positive duration")
	}
}
