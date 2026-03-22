package producer_test

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"

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

func TestSetRate_updatesExistingLimiter(t *testing.T) {
	rec := metrics.NewRecorder(0)
	w := producer.NewWorker(producer.NoopSender{}, rec, "t", 64, 100)
	w.SetRate(200)
	if w.Limiter().Limit() != rate.Limit(200) {
		t.Errorf("Limit() = %v, want 200", w.Limiter().Limit())
	}
	if w.Limiter().Burst() != 200 {
		t.Errorf("Burst() = %v, want 200", w.Limiter().Burst())
	}
}

func TestSetRate_zeroSetsUnlimited(t *testing.T) {
	rec := metrics.NewRecorder(0)
	w := producer.NewWorker(producer.NoopSender{}, rec, "t", 64, 100)
	w.SetRate(0)
	if w.Limiter().Limit() != rate.Inf {
		t.Errorf("Limit() = %v, want rate.Inf", w.Limiter().Limit())
	}
}

func TestSetRate_nilLimiterIsNoop(t *testing.T) {
	rec := metrics.NewRecorder(0)
	// produceRate=0 → limiter is nil
	w := producer.NewWorker(producer.NoopSender{}, rec, "t", 64, 0)
	// Must not panic
	w.SetRate(100)
	if w.Limiter() != nil {
		t.Error("expected limiter to remain nil")
	}
}

func TestSetRate_concurrentWithRun(t *testing.T) {
	// Run this test with: go test -race ./internal/producer/...
	rec := metrics.NewRecorder(0)
	w := producer.NewWorker(producer.NoopSender{}, rec, "t", 64, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go w.Run(ctx)
	for i := 0; i < 100; i++ {
		w.SetRate(1000)
	}
	<-ctx.Done()
}

func TestNewPool_createsNWorkers(t *testing.T) {
	rec := metrics.NewRecorder(0)
	workers := producer.NewPool(producer.NoopSender{}, rec, 3, "t", 64, 100)
	if len(workers) != 3 {
		t.Fatalf("len(workers) = %d, want 3", len(workers))
	}
	for i, w := range workers {
		if w == nil {
			t.Errorf("workers[%d] is nil", i)
		}
		if w.Limiter() == nil {
			t.Errorf("workers[%d].Limiter() is nil, want non-nil for produceRate=100", i)
		}
	}
}

func TestStartPool_runsAllWorkers(t *testing.T) {
	rec := metrics.NewRecorder(0)
	workers := producer.NewPool(producer.NoopSender{}, rec, 4, "t", 64, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// StartPool must return once context is cancelled
	done := make(chan struct{})
	go func() {
		producer.StartPool(ctx, workers)
		close(done)
	}()
	select {
	case <-done:
		// good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StartPool did not return after context cancel")
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
