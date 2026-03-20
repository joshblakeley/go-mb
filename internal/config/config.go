package config

import (
	"errors"
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
	if c.ReportInterval <= 0 {
		return errors.New("report-interval must be positive")
	}
	return nil
}
