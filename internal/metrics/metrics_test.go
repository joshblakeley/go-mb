package metrics_test

import (
	"testing"
	"time"

	"github.com/redpanda-data/go-bench/internal/metrics"
)

func TestRecordSend(t *testing.T) {
	r := metrics.NewRecorder()
	r.RecordSend(1024, 500) // 1024 bytes, 500µs publish latency
	snap := r.Snapshot()
	if snap.MessagesSent != 1 {
		t.Errorf("MessagesSent = %d, want 1", snap.MessagesSent)
	}
	if snap.BytesSent != 1024 {
		t.Errorf("BytesSent = %d, want 1024", snap.BytesSent)
	}
	if snap.PublishLatency.TotalCount() != 1 {
		t.Errorf("PublishLatency count = %d, want 1", snap.PublishLatency.TotalCount())
	}
}

func TestRecordReceive(t *testing.T) {
	r := metrics.NewRecorder()
	r.RecordReceive(512, 1200) // 512 bytes, 1200µs E2E latency
	snap := r.Snapshot()
	if snap.MessagesReceived != 1 {
		t.Errorf("MessagesReceived = %d, want 1", snap.MessagesReceived)
	}
	if snap.BytesReceived != 512 {
		t.Errorf("BytesReceived = %d, want 512", snap.BytesReceived)
	}
}

func TestSnapshotResetsCounters(t *testing.T) {
	r := metrics.NewRecorder()
	r.RecordSend(100, 100)
	r.Snapshot() // first snapshot drains
	snap2 := r.Snapshot()
	if snap2.MessagesSent != 0 {
		t.Errorf("MessagesSent after second snapshot = %d, want 0", snap2.MessagesSent)
	}
}

func TestCumulativeNotReset(t *testing.T) {
	r := metrics.NewRecorder()
	r.RecordSend(100, 100)
	r.Snapshot() // resets period counters
	cum := r.Cumulative()
	if cum.MessagesSent != 1 {
		t.Errorf("Cumulative MessagesSent = %d, want 1", cum.MessagesSent)
	}
}

func TestRecordSendError(t *testing.T) {
	r := metrics.NewRecorder()
	r.RecordSendError()
	snap := r.Snapshot()
	if snap.PublishErrors != 1 {
		t.Errorf("PublishErrors = %d, want 1", snap.PublishErrors)
	}
}

func TestElapsed(t *testing.T) {
	r := metrics.NewRecorder()
	time.Sleep(10 * time.Millisecond)
	if r.Elapsed() < 10*time.Millisecond {
		t.Error("Elapsed should be >= 10ms")
	}
}
