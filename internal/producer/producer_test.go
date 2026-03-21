package producer_test

import (
	"context"
	"testing"
	"time"

	"github.com/joshblakeley/go-mb/internal/metrics"
	"github.com/joshblakeley/go-mb/internal/producer"
)

func TestBuildPayload(t *testing.T) {
	payload := producer.BuildPayload(64)
	if len(payload) != 64 {
		t.Errorf("payload length = %d, want 64", len(payload))
	}
	// First 8 bytes are a timestamp placeholder (zero at build time).
	// Actual timestamp is stamped at send time; this just validates the size.
}

func TestBuildPayloadMinSize(t *testing.T) {
	// messageSize < 8 should still return 8 bytes (timestamp only)
	payload := producer.BuildPayload(4)
	if len(payload) < 8 {
		t.Errorf("payload length = %d, want >= 8", len(payload))
	}
}

func TestWorkerStopsOnContextCancel(t *testing.T) {
	rec := metrics.NewRecorder(0)
	// Use a no-op sender so we don't need a real broker.
	w := producer.NewWorker(producer.NoopSender{}, rec, "test-topic", 64, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker did not stop after context cancel")
	}
}
