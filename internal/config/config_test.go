package config_test

import (
	"math"
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

func TestValidateReportInterval(t *testing.T) {
	cfg := config.Default()
	cfg.ReportInterval = -1 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for non-positive report-interval")
	}
}

func TestValidateWarmupDuration(t *testing.T) {
	cfg := config.Default()
	cfg.WarmupDuration = -1 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative warmup-duration")
	}
}

func TestValidateAcks(t *testing.T) {
	cfg := config.Default()
	cfg.Acks = "2"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid acks value")
	}
	for _, valid := range []string{"0", "1", "all"} {
		cfg.Acks = valid
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected no error for acks=%q, got: %v", valid, err)
		}
	}
}

func TestValidateCompression(t *testing.T) {
	cfg := config.Default()
	cfg.Compression = "brotli"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid compression value")
	}
	for _, valid := range []string{"none", "gzip", "snappy", "lz4", "zstd"} {
		cfg.Compression = valid
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected no error for compression=%q, got: %v", valid, err)
		}
	}
}

func TestValidateLingerMs(t *testing.T) {
	cfg := config.Default()
	cfg.LingerMs = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative linger-ms")
	}
	cfg.LingerMs = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for linger-ms=0, got: %v", err)
	}
	cfg.LingerMs = 5
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for linger-ms=5, got: %v", err)
	}
}

func TestValidateBatchMaxBytes(t *testing.T) {
	cfg := config.Default()
	cfg.BatchMaxBytes = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative batch-max-bytes")
	}
	cfg.BatchMaxBytes = math.MaxInt32 + 1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for batch-max-bytes > MaxInt32")
	}
	cfg.BatchMaxBytes = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for batch-max-bytes=0, got: %v", err)
	}
	cfg.BatchMaxBytes = 131072
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for batch-max-bytes=131072, got: %v", err)
	}
}
