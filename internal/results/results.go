// Package results provides data types, constructors, and HTML report generation
// for benchmark run output.
package results

import (
	"time"

	"github.com/joshblakeley/go-mb/internal/config"
	"github.com/joshblakeley/go-mb/internal/metrics"
)

// DataPoint holds per-interval benchmark metrics for one reporting tick.
type DataPoint struct {
	ElapsedSecs       float64 // seconds since benchmark start (chart x-axis)
	PubRateMsgPerSec  float64
	PubRateMBPerSec   float64
	ConsRateMsgPerSec float64
	ConsRateMBPerSec  float64
	BacklogK          float64 // total in-flight messages / 1000
	PubLatencyAvgMs   float64
	PubLatencyP50Ms   float64
	PubLatencyP99Ms   float64
	PubLatencyP999Ms  float64
	PubLatencyMaxMs   float64
	E2ELatencyAvgMs   float64
	E2ELatencyP50Ms   float64
	E2ELatencyP99Ms   float64
	E2ELatencyP999Ms  float64
	E2ELatencyMaxMs   float64
}

// RunMeta holds display-relevant config for the report header.
type RunMeta struct {
	Brokers           []string
	Topic             string
	Partitions        int
	ReplicationFactor int
	Producers         int
	Consumers         int
	MessageSize       int
	ProduceRate       int // 0 = unlimited
	Duration          time.Duration
	ReportInterval    time.Duration
	Timestamp         time.Time // benchmark start (after warmup)
	// Producer tuning
	Acks          string
	Compression   string
	LingerMs      int
	BatchMaxBytes int // 0 = franz-go default
	// Security
	TLS           bool
	SASLMechanism string // "" = none
}

// FinalSummary holds aggregated latency percentiles for the summary table.
// Mirrors the percentiles in reporter.FormatFinal.
type FinalSummary struct {
	PubAvgMs   float64
	PubP50Ms   float64
	PubP95Ms   float64
	PubP99Ms   float64
	PubP999Ms  float64
	PubP9999Ms float64
	PubMaxMs   float64
	E2EAvgMs   float64
	E2EP50Ms   float64
	E2EP95Ms   float64
	E2EP99Ms   float64
	E2EP999Ms  float64
	E2EP9999Ms float64
	E2EMaxMs   float64
}

// Run bundles all data for a single benchmark run.
type Run struct {
	Meta    RunMeta
	Points  []DataPoint
	Summary FinalSummary
}

// DataPointFromSnapshot constructs a DataPoint from a period snapshot.
//
// elapsed is the period duration (t.Sub(lastTick)) — used to compute per-second rates.
// elapsedSinceStart is time.Since(benchmarkStart).Seconds() — the chart x-axis value.
// These are different: elapsed is the width of this interval; elapsedSinceStart is total
// time since the benchmark began.
func DataPointFromSnapshot(snap metrics.Snapshot, elapsed time.Duration, elapsedSinceStart float64) DataPoint {
	secs := elapsed.Seconds()
	if secs <= 0 {
		secs = 1
	}
	return DataPoint{
		ElapsedSecs:       elapsedSinceStart,
		PubRateMsgPerSec:  float64(snap.MessagesSent) / secs,
		PubRateMBPerSec:   float64(snap.BytesSent) / secs / 1024 / 1024,
		ConsRateMsgPerSec: float64(snap.MessagesReceived) / secs,
		ConsRateMBPerSec:  float64(snap.BytesReceived) / secs / 1024 / 1024,
		// TotalSent/TotalReceived are cumulative (not reset per interval).
		BacklogK:         float64(snap.TotalSent-snap.TotalReceived) / 1000.0,
		PubLatencyAvgMs:  snap.PublishLatency.Mean() / 1000.0,
		PubLatencyP50Ms:  float64(snap.PublishLatency.ValueAtPercentile(50)) / 1000.0,
		PubLatencyP99Ms:  float64(snap.PublishLatency.ValueAtPercentile(99)) / 1000.0,
		PubLatencyP999Ms: float64(snap.PublishLatency.ValueAtPercentile(99.9)) / 1000.0,
		PubLatencyMaxMs:  float64(snap.PublishLatency.Max()) / 1000.0,
		E2ELatencyAvgMs:  snap.EndToEndLatency.Mean() / 1000.0,
		E2ELatencyP50Ms:  float64(snap.EndToEndLatency.ValueAtPercentile(50)) / 1000.0,
		E2ELatencyP99Ms:  float64(snap.EndToEndLatency.ValueAtPercentile(99)) / 1000.0,
		E2ELatencyP999Ms: float64(snap.EndToEndLatency.ValueAtPercentile(99.9)) / 1000.0,
		E2ELatencyMaxMs:  float64(snap.EndToEndLatency.Max()) / 1000.0,
	}
}

// FinalSummaryFromSnapshot constructs a FinalSummary from a cumulative snapshot.
// Called with rec.Cumulative() — only histogram fields are populated in that snapshot.
// Uses float64 Mean() directly (no int64 cast) to preserve sub-microsecond precision.
func FinalSummaryFromSnapshot(snap metrics.Snapshot) FinalSummary {
	return FinalSummary{
		PubAvgMs:   snap.PublishLatency.Mean() / 1000.0,
		PubP50Ms:   float64(snap.PublishLatency.ValueAtPercentile(50)) / 1000.0,
		PubP95Ms:   float64(snap.PublishLatency.ValueAtPercentile(95)) / 1000.0,
		PubP99Ms:   float64(snap.PublishLatency.ValueAtPercentile(99)) / 1000.0,
		PubP999Ms:  float64(snap.PublishLatency.ValueAtPercentile(99.9)) / 1000.0,
		PubP9999Ms: float64(snap.PublishLatency.ValueAtPercentile(99.99)) / 1000.0,
		PubMaxMs:   float64(snap.PublishLatency.Max()) / 1000.0,
		E2EAvgMs:   snap.EndToEndLatency.Mean() / 1000.0,
		E2EP50Ms:   float64(snap.EndToEndLatency.ValueAtPercentile(50)) / 1000.0,
		E2EP95Ms:   float64(snap.EndToEndLatency.ValueAtPercentile(95)) / 1000.0,
		E2EP99Ms:   float64(snap.EndToEndLatency.ValueAtPercentile(99)) / 1000.0,
		E2EP999Ms:  float64(snap.EndToEndLatency.ValueAtPercentile(99.9)) / 1000.0,
		E2EP9999Ms: float64(snap.EndToEndLatency.ValueAtPercentile(99.99)) / 1000.0,
		E2EMaxMs:   float64(snap.EndToEndLatency.Max()) / 1000.0,
	}
}

// RunMetaFromConfig constructs a RunMeta from config.
// benchmarkStart is the time the benchmark phase began (after warmup) — used as Timestamp.
func RunMetaFromConfig(cfg *config.Config, benchmarkStart time.Time) RunMeta {
	return RunMeta{
		Brokers:           cfg.Brokers,
		Topic:             cfg.Topic,
		Partitions:        cfg.Partitions,
		ReplicationFactor: cfg.ReplicationFactor,
		Producers:         cfg.Producers,
		Consumers:         cfg.Consumers,
		MessageSize:       cfg.MessageSize,
		ProduceRate:       cfg.ProduceRate,
		Duration:          cfg.Duration,
		ReportInterval:    cfg.ReportInterval,
		Timestamp:         benchmarkStart,
		Acks:              cfg.Acks,
		Compression:       cfg.Compression,
		LingerMs:          cfg.LingerMs,
		BatchMaxBytes:     cfg.BatchMaxBytes,
		TLS:               cfg.TLS || cfg.TLSCACert != "",
		SASLMechanism:     cfg.SASLMechanism,
	}
}
