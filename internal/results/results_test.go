package results_test

import (
	"testing"
	"time"

	"github.com/redpanda-data/go-bench/internal/config"
	"github.com/redpanda-data/go-bench/internal/histogram"
	"github.com/redpanda-data/go-bench/internal/metrics"
	"github.com/redpanda-data/go-bench/internal/results"
)

// makeSnap constructs a metrics.Snapshot for testing.
// pubVals/e2eVals are histogram values in microseconds.
func makeSnap(pubVals, e2eVals []int64, msgsSent, bytesSent, msgsRecv, bytesRecv, totalSent, totalRecv int64) metrics.Snapshot {
	pubH := histogram.New()
	for _, v := range pubVals {
		pubH.RecordValue(v)
	}
	e2eH := histogram.New()
	for _, v := range e2eVals {
		e2eH.RecordValue(v)
	}
	return metrics.Snapshot{
		MessagesSent:     msgsSent,
		BytesSent:        bytesSent,
		MessagesReceived: msgsRecv,
		BytesReceived:    bytesRecv,
		TotalSent:        totalSent,
		TotalReceived:    totalRecv,
		PublishLatency:   pubH,
		EndToEndLatency:  e2eH,
	}
}

func TestDataPointFromSnapshot(t *testing.T) {
	// pub histogram [200, 200, 300, 400] µs; e2e [300, 300, 500, 600] µs
	snap := makeSnap(
		[]int64{200, 200, 300, 400},
		[]int64{300, 300, 500, 600},
		1000, 1024*1000, // msgs sent, bytes sent
		1000, 1024*1000, // msgs recv, bytes recv
		5000, 4900,      // totalSent, totalRecv
	)
	elapsed := 1 * time.Second
	elapsedSinceStart := 10.0

	dp := results.DataPointFromSnapshot(snap, elapsed, elapsedSinceStart)

	if dp.ElapsedSecs != 10.0 {
		t.Errorf("ElapsedSecs = %v, want 10.0", dp.ElapsedSecs)
	}
	if dp.PubRateMsgPerSec != 1000.0 {
		t.Errorf("PubRateMsgPerSec = %v, want 1000.0", dp.PubRateMsgPerSec)
	}
	// p99 of [200, 200, 300, 400] µs → 400 µs → 0.4 ms
	if dp.PubLatencyP99Ms != 0.4 {
		t.Errorf("PubLatencyP99Ms = %v, want 0.4", dp.PubLatencyP99Ms)
	}
	// p50 of [300, 300, 500, 600] µs → 300 µs → 0.3 ms
	if dp.E2ELatencyP50Ms != 0.3 {
		t.Errorf("E2ELatencyP50Ms = %v, want 0.3", dp.E2ELatencyP50Ms)
	}
	// BacklogK: (5000 - 4900) / 1000 = 0.1
	if dp.BacklogK != 0.1 {
		t.Errorf("BacklogK = %v, want 0.1", dp.BacklogK)
	}
}

func TestFinalSummaryFromSnapshot(t *testing.T) {
	// 100 values: 1..100 µs for pub; 2..200 µs for e2e
	pubH := histogram.New()
	e2eH := histogram.New()
	for i := int64(1); i <= 100; i++ {
		pubH.RecordValue(i)
		e2eH.RecordValue(i * 2)
	}
	snap := metrics.Snapshot{
		PublishLatency:  pubH,
		EndToEndLatency: e2eH,
	}

	s := results.FinalSummaryFromSnapshot(snap)

	// p95 of 1-100 µs ≈ 95 µs → 0.095 ms (HDR precision ±0.1%)
	if s.PubP95Ms < 0.094 || s.PubP95Ms > 0.096 {
		t.Errorf("PubP95Ms = %.4f, want ~0.095", s.PubP95Ms)
	}
	// p99.99 of 2-200 µs should be near max (200 µs → 0.200 ms)
	if s.E2EP9999Ms < 0.19 || s.E2EP9999Ms > 0.201 {
		t.Errorf("E2EP9999Ms = %.4f, want ~0.200", s.E2EP9999Ms)
	}
}

func TestRunMetaFromConfig(t *testing.T) {
	cfg := &config.Config{
		Brokers:        []string{"localhost:9092"},
		Topic:          "test-topic",
		Producers:      4,
		Consumers:      2,
		MessageSize:    2048,
		Duration:       30 * time.Second,
		ReportInterval: 5 * time.Second,
	}
	ts := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	meta := results.RunMetaFromConfig(cfg, ts)

	if meta.Producers != 4 {
		t.Errorf("Producers = %v, want 4", meta.Producers)
	}
	if meta.MessageSize != 2048 {
		t.Errorf("MessageSize = %v, want 2048", meta.MessageSize)
	}
	if meta.Duration != 30*time.Second {
		t.Errorf("Duration = %v, want 30s", meta.Duration)
	}
	if !meta.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", meta.Timestamp, ts)
	}
}
