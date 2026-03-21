// Package consumer implements benchmark consumer workers.
// Each Worker polls for messages, extracts the producer timestamp from the payload,
// and records end-to-end latency in the shared Recorder.
package consumer

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/joshblakeley/go-mb/internal/metrics"
)

// Fetcher is the subset of kgo.Client used by Worker, enabling test doubles.
type Fetcher interface {
	PollFetches(ctx context.Context) kgo.Fetches
}

// NoopFetcher is a Fetcher that returns empty fetches immediately. Used in tests.
type NoopFetcher struct{}

func (NoopFetcher) PollFetches(ctx context.Context) kgo.Fetches {
	// Yield to avoid busy-loop in tests.
	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Millisecond):
	}
	return kgo.Fetches{}
}

// SingleRecordFetcher returns the given records once, then empty fetches.
type SingleRecordFetcher struct {
	Records []*kgo.Record
	done    bool
}

func (f *SingleRecordFetcher) PollFetches(ctx context.Context) kgo.Fetches {
	if f.done {
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Millisecond):
		}
		return kgo.Fetches{}
	}
	f.done = true
	fetches := kgo.Fetches{
		{
			Topics: []kgo.FetchTopic{
				{
					Topic: "test",
					Partitions: []kgo.FetchPartition{
						{Records: f.Records},
					},
				},
			},
		},
	}
	return fetches
}

// ExtractTimestamp reads the unix-nanosecond timestamp from the first 8 bytes
// of a message payload. Returns 0 if payload is shorter than 8 bytes.
func ExtractTimestamp(payload []byte) int64 {
	if len(payload) < 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(payload[:8]))
}

// Worker polls messages from a Kafka topic and records E2E latency.
type Worker struct {
	fetcher Fetcher
	rec     *metrics.Recorder
}

// NewWorker creates a consumer Worker.
func NewWorker(fetcher Fetcher, rec *metrics.Recorder) *Worker {
	return &Worker{fetcher: fetcher, rec: rec}
}

// Run consumes messages until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		fetches := w.fetcher.PollFetches(ctx)
		now := time.Now().UnixNano()
		fetches.EachRecord(func(r *kgo.Record) {
			ts := ExtractTimestamp(r.Value)
			var e2eMicros int64
			if ts > 0 {
				e2eMicros = (now - ts) / 1000 // nanoseconds → microseconds
				if e2eMicros < 0 {
					e2eMicros = 0
				}
			}
			w.rec.RecordReceive(len(r.Value), e2eMicros)
		})
	}
}

// RunPool starts n consumer goroutines sharing a single kgo.Client and Recorder.
// It blocks until all workers have stopped.
func RunPool(ctx context.Context, client *kgo.Client, rec *metrics.Recorder, n int) {
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			w := NewWorker(client, rec)
			w.Run(ctx)
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
	// reporter is the only package that writes to stdout; no print here.
}
