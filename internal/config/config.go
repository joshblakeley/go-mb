package config

import (
	"errors"
	"math"
	"time"
)

// Config holds all benchmark parameters parsed from CLI flags.
type Config struct {
	Brokers           []string
	Topic             string
	Partitions        int
	ReplicationFactor int
	Producers         int
	Consumers         int
	MessageSize       int // bytes
	ProduceRate       int // msg/s per producer; 0 = unlimited
	Duration          time.Duration
	WarmupDuration    time.Duration
	ReportInterval    time.Duration
	ConsumerGroup     string
	CreateTopic       bool
	DeleteTopic       bool
	OutputFile        string // path to write HTML report; empty means disabled
	Acks              string // "0", "1", "all"
	Compression       string // "none", "gzip", "snappy", "lz4", "zstd"
	LingerMs          int    // milliseconds; 0 = franz-go default (send immediately)
	BatchMaxBytes     int    // bytes; 0 = franz-go default (~1MB)
	TLS           bool   // enable TLS with system root CAs
	TLSCACert     string // path to CA cert PEM file; implies TLS
	SASLMechanism string // "plain", "scram-sha-256", "scram-sha-512"; "" = disabled
	SASLUsername  string // required when SASLMechanism is set
	SASLPassword  string // required when SASLMechanism is set
}

// Default returns a Config with sensible defaults matching OMB workload defaults.
func Default() Config {
	return Config{
		Brokers:           []string{"localhost:9092"},
		Topic:             "benchmark",
		Partitions:        1,
		ReplicationFactor: 1,
		Producers:         1,
		Consumers:         1,
		MessageSize:       1024,
		ProduceRate:       0,
		Duration:          1 * time.Minute,
		WarmupDuration:    0,
		ReportInterval:    10 * time.Second,
		ConsumerGroup:     "benchmark-group",
		CreateTopic:       true,
		DeleteTopic:       true,
		Acks:              "all",
		Compression:       "none",
		LingerMs:          0,
		BatchMaxBytes:     0,
		TLS:           false,
		TLSCACert:     "",
		SASLMechanism: "",
		SASLUsername:  "",
		SASLPassword:  "",
	}
}

// Validate returns an error if Config contains invalid values.
func (c *Config) Validate() error {
	if len(c.Brokers) == 0 {
		return errors.New("at least one broker address is required")
	}
	if c.Topic == "" {
		return errors.New("topic name is required")
	}
	if c.Partitions < 1 {
		return errors.New("partitions must be >= 1")
	}
	if c.ReplicationFactor < 1 {
		return errors.New("replication-factor must be >= 1")
	}
	if c.Producers < 0 {
		return errors.New("producers must be >= 0")
	}
	if c.Consumers < 0 {
		return errors.New("consumers must be >= 0")
	}
	if c.Producers == 0 && c.Consumers == 0 {
		return errors.New("at least one producer or consumer is required")
	}
	if c.MessageSize < 8 {
		return errors.New("message-size must be >= 8 (8 bytes reserved for timestamp)")
	}
	if c.Duration <= 0 {
		return errors.New("duration must be positive")
	}
	if c.WarmupDuration < 0 {
		return errors.New("warmup-duration must be >= 0")
	}
	if c.ReportInterval <= 0 {
		return errors.New("report-interval must be positive")
	}
	validAcks := map[string]bool{"0": true, "1": true, "all": true}
	if !validAcks[c.Acks] {
		return errors.New("acks must be 0, 1, or all")
	}
	validCompression := map[string]bool{"none": true, "gzip": true, "snappy": true, "lz4": true, "zstd": true}
	if !validCompression[c.Compression] {
		return errors.New("compression must be none, gzip, snappy, lz4, or zstd")
	}
	if c.LingerMs < 0 {
		return errors.New("linger-ms must be >= 0")
	}
	if c.BatchMaxBytes < 0 || c.BatchMaxBytes > math.MaxInt32 {
		return errors.New("batch-max-bytes must be between 0 and 2147483647")
	}
	validMechanisms := map[string]bool{"plain": true, "scram-sha-256": true, "scram-sha-512": true}
	if c.SASLMechanism != "" && !validMechanisms[c.SASLMechanism] {
		return errors.New("sasl-mechanism must be plain, scram-sha-256, or scram-sha-512")
	}
	if c.SASLMechanism != "" && c.SASLUsername == "" {
		return errors.New("sasl-username is required when sasl-mechanism is set")
	}
	if c.SASLMechanism != "" && c.SASLPassword == "" {
		return errors.New("sasl-password is required when sasl-mechanism is set")
	}
	if (c.SASLUsername != "" || c.SASLPassword != "") && c.SASLMechanism == "" {
		return errors.New("sasl-mechanism is required when sasl-username or sasl-password is set")
	}
	return nil
}
