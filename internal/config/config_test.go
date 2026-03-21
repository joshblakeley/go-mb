package config_test

import (
	"math"
	"testing"
	"time"

	"github.com/joshblakeley/go-mb/internal/config"
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

func TestValidateSASLMechanism(t *testing.T) {
	cfg := config.Default()
	cfg.SASLMechanism = "kerberos"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid sasl-mechanism")
	}
	for _, valid := range []string{"plain", "scram-sha-256", "scram-sha-512"} {
		cfg.SASLMechanism = valid
		cfg.SASLUsername = "user"
		cfg.SASLPassword = "pass"
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected no error for sasl-mechanism=%q, got: %v", valid, err)
		}
	}
}

func TestValidateSASLRequiresCredentials(t *testing.T) {
	cfg := config.Default()
	cfg.SASLMechanism = "plain"

	// Missing username
	cfg.SASLUsername = ""
	cfg.SASLPassword = "pass"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when sasl-mechanism set but username missing")
	}

	// Missing password
	cfg.SASLUsername = "user"
	cfg.SASLPassword = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when sasl-mechanism set but password missing")
	}
}

func TestValidateSASLCredentialsRequireMechanism(t *testing.T) {
	// Username without mechanism
	cfg := config.Default()
	cfg.SASLUsername = "user"
	cfg.SASLMechanism = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when sasl-username set but sasl-mechanism missing")
	}

	// Password without mechanism
	cfg2 := config.Default()
	cfg2.SASLPassword = "pass"
	cfg2.SASLMechanism = ""
	if err := cfg2.Validate(); err == nil {
		t.Error("expected error when sasl-password set but sasl-mechanism missing")
	}
}
