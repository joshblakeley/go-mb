// Package producer implements benchmark producer workers.
// Each Worker runs in its own goroutine, stamping messages with a nanosecond
// timestamp and recording send latency in the shared Recorder.
package producer

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/time/rate"

	"github.com/joshblakeley/go-mb/internal/metrics"
)

// Sender is the subset of kgo.Client used by Worker, enabling test doubles.
type Sender interface {
	ProduceSync(ctx context.Context, records ...*kgo.Record) kgo.ProduceResults
}

// NoopSender is a Sender that immediately acks every record. Used in tests.
type NoopSender struct{}

func (NoopSender) ProduceSync(_ context.Context, records ...*kgo.Record) kgo.ProduceResults {
	results := make(kgo.ProduceResults, len(records))
	for i, r := range records {
		results[i] = kgo.ProduceResult{Record: r}
	}
	return results
}

// BuildPayload returns a byte slice of the requested size filled with random
// bytes, except the first 8 bytes which are reserved for the timestamp and
// zeroed here (stamped at send time). Random content prevents compressors from
// deflating payloads artificially, matching OMB behaviour.
// If size < 8, returns an 8-byte slice.
func BuildPayload(size int) []byte {
	if size < 8 {
		size = 8
	}
	buf := make([]byte, size)
	_, _ = rand.Read(buf[8:])
	return buf
}

// stampTime writes the current unix-nanosecond time into the first 8 bytes of payload.
func stampTime(payload []byte) {
	binary.BigEndian.PutUint64(payload[:8], uint64(time.Now().UnixNano()))
}

// Worker sends messages to a single Kafka topic.
type Worker struct {
	sender  Sender
	rec     *metrics.Recorder
	topic   string
	payload []byte
	limiter *rate.Limiter // nil when ProduceRate == 0
}

// NewWorker creates a Worker. produceRate is msg/s per worker; 0 means unlimited.
func NewWorker(sender Sender, rec *metrics.Recorder, topic string, messageSize int, produceRate int) *Worker {
	w := &Worker{
		sender:  sender,
		rec:     rec,
		topic:   topic,
		payload: BuildPayload(messageSize),
	}
	if produceRate > 0 {
		w.limiter = rate.NewLimiter(rate.Limit(produceRate), produceRate)
	}
	return w
}

// SetRate updates the worker's rate limit dynamically.
// Safe to call concurrently with Run — uses SetLimit/SetBurst which are
// documented as goroutine-safe on rate.Limiter.
// If the worker has no rate limiter (created with produceRate=0), this is a no-op.
// If r <= 0, sets the limiter to unlimited (rate.Inf) rather than nil,
// avoiding a pointer write that would race with Run.
func (w *Worker) SetRate(r int) {
	if w.limiter == nil {
		return
	}
	if r <= 0 {
		w.limiter.SetLimit(rate.Inf)
		w.limiter.SetBurst(1)
		return
	}
	w.limiter.SetLimit(rate.Limit(r))
	w.limiter.SetBurst(r)
}

// Run produces messages until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if w.limiter != nil {
			if err := w.limiter.Wait(ctx); err != nil {
				return // context cancelled
			}
		}

		stampTime(w.payload)
		// Copy the payload per-record: franz-go may hold a reference to Value
		// until the broker acks, so mutating w.payload on the next iteration
		// would be a data race.
		msg := make([]byte, len(w.payload))
		copy(msg, w.payload)
		rec := &kgo.Record{
			Topic: w.topic,
			Value: msg,
		}

		start := time.Now()
		results := w.sender.ProduceSync(ctx, rec)
		latencyMicros := time.Since(start).Microseconds()

		for _, res := range results {
			if res.Err != nil {
				w.rec.RecordSendError()
			} else {
				w.rec.RecordSend(len(w.payload), latencyMicros)
			}
		}
	}
}

// NewPool creates n Workers without starting them.
// Takes Sender (not *kgo.Client) to match NewWorker and preserve testability.
func NewPool(sender Sender, rec *metrics.Recorder, n int, topic string, messageSize int, produceRate int) []*Worker {
	workers := make([]*Worker, n)
	for i := range workers {
		workers[i] = NewWorker(sender, rec, topic, messageSize, produceRate)
	}
	return workers
}

// StartPool runs all workers concurrently and blocks until all have stopped.
func StartPool(ctx context.Context, workers []*Worker) {
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Run(ctx)
		}()
	}
	wg.Wait()
}

// RunPool starts n Worker goroutines sharing a single kgo.Client and Recorder.
// It blocks until all workers have stopped.
func RunPool(ctx context.Context, client *kgo.Client, rec *metrics.Recorder, n int, topic string, messageSize int, produceRate int) {
	StartPool(ctx, NewPool(client, rec, n, topic, messageSize, produceRate))
}
