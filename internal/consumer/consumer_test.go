package consumer_test

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/joshblakeley/go-mb/internal/consumer"
	"github.com/joshblakeley/go-mb/internal/metrics"
)

func TestExtractTimestamp(t *testing.T) {
	now := time.Now().UnixNano()
	payload := make([]byte, 64)
	binary.BigEndian.PutUint64(payload[:8], uint64(now))

	ts := consumer.ExtractTimestamp(payload)
	if ts != now {
		t.Errorf("ExtractTimestamp = %d, want %d", ts, now)
	}
}

func TestExtractTimestampShortPayload(t *testing.T) {
	// payload < 8 bytes should return 0 (no panic)
	ts := consumer.ExtractTimestamp([]byte{1, 2, 3})
	if ts != 0 {
		t.Errorf("expected 0 for short payload, got %d", ts)
	}
}

func TestWorkerStopsOnContextCancel(t *testing.T) {
	rec := metrics.NewRecorder(0)
	w := consumer.NewWorker(consumer.NoopFetcher{}, rec)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("consumer worker did not stop after context cancel")
	}
}

// NoopFetcherWithRecords returns a single batch of pre-crafted records.
func TestWorkerRecordsMetrics(t *testing.T) {
	rec := metrics.NewRecorder(0)

	// Craft a payload with a timestamp 5ms in the past.
	past := time.Now().Add(-5 * time.Millisecond).UnixNano()
	payload := make([]byte, 64)
	binary.BigEndian.PutUint64(payload[:8], uint64(past))

	fetcher := &consumer.SingleRecordFetcher{
		Records: []*kgo.Record{{Value: payload, Topic: "test"}},
	}
	w := consumer.NewWorker(fetcher, rec)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		w.Run(ctx)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	snap := rec.Snapshot()
	if snap.MessagesReceived == 0 {
		t.Error("expected MessagesReceived > 0")
	}
}
