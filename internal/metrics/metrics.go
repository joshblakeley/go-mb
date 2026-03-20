// Package metrics provides a thread-safe recorder for benchmark metrics.
// Producers call RecordSend; consumers call RecordReceive.
// A reporter goroutine calls Snapshot() on each interval to get period stats,
// and Cumulative() at the end for the final aggregated report.
package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/redpanda-data/go-bench/internal/histogram"
)

// Snapshot is a point-in-time view of period metrics. Histograms are cloned
// copies — safe to read after the Recorder has been further updated.
type Snapshot struct {
	MessagesSent     int64
	BytesSent        int64
	MessagesReceived int64
	BytesReceived    int64
	PublishErrors    int64
	TotalSent        int64
	TotalReceived    int64
	PublishLatency   *histogram.Histogram
	EndToEndLatency  *histogram.Histogram
}

// Recorder accumulates metrics from concurrent producers and consumers.
type Recorder struct {
	startTime time.Time

	// Period counters — reset on Snapshot().
	mu               sync.Mutex
	messagesSent     int64
	bytesSent        int64
	messagesReceived int64
	bytesReceived    int64
	publishErrors    int64
	periodPublish    *histogram.Histogram
	periodE2E        *histogram.Histogram

	// Cumulative — never reset.
	totalSent     atomic.Int64
	totalReceived atomic.Int64
	cumMu         sync.Mutex
	cumPublish    *histogram.Histogram
	cumE2E        *histogram.Histogram
}

// NewRecorder returns an initialised Recorder.
func NewRecorder() *Recorder {
	return &Recorder{
		startTime:     time.Now(),
		periodPublish: histogram.New(),
		periodE2E:     histogram.New(),
		cumPublish:    histogram.New(),
		cumE2E:        histogram.New(),
	}
}

// RecordSend records a successful producer send.
// latencyMicros is the time from ProduceSync call to ack.
func (r *Recorder) RecordSend(bytes int, latencyMicros int64) {
	r.mu.Lock()
	r.messagesSent++
	r.bytesSent += int64(bytes)
	r.periodPublish.RecordValue(latencyMicros)
	r.totalSent.Add(1) // inside mu so Snapshot sees consistent period+cumulative counts
	r.mu.Unlock()

	r.cumMu.Lock()
	r.cumPublish.RecordValue(latencyMicros)
	r.cumMu.Unlock()
}

// RecordSendError records a failed producer send.
func (r *Recorder) RecordSendError() {
	r.mu.Lock()
	r.publishErrors++
	r.mu.Unlock()
}

// RecordReceive records a successfully consumed message.
// e2eLatencyMicros is computed from the timestamp embedded in the message payload.
func (r *Recorder) RecordReceive(bytes int, e2eLatencyMicros int64) {
	r.mu.Lock()
	r.messagesReceived++
	r.bytesReceived += int64(bytes)
	r.periodE2E.RecordValue(e2eLatencyMicros)
	r.totalReceived.Add(1) // inside mu so Snapshot sees consistent period+cumulative counts
	r.mu.Unlock()

	r.cumMu.Lock()
	r.cumE2E.RecordValue(e2eLatencyMicros)
	r.cumMu.Unlock()
}

// Snapshot returns period metrics and resets period counters/histograms.
// Safe to call from a single reporter goroutine.
func (r *Recorder) Snapshot() Snapshot {
	r.mu.Lock()
	s := Snapshot{
		MessagesSent:     r.messagesSent,
		BytesSent:        r.bytesSent,
		MessagesReceived: r.messagesReceived,
		BytesReceived:    r.bytesReceived,
		PublishErrors:    r.publishErrors,
		TotalSent:        r.totalSent.Load(),
		TotalReceived:    r.totalReceived.Load(),
		PublishLatency:   r.periodPublish.Clone(),
		EndToEndLatency:  r.periodE2E.Clone(),
	}
	// Reset period state.
	r.messagesSent = 0
	r.bytesSent = 0
	r.messagesReceived = 0
	r.bytesReceived = 0
	r.publishErrors = 0
	r.periodPublish.Reset()
	r.periodE2E.Reset()
	r.mu.Unlock()
	return s
}

// Cumulative returns aggregated metrics across all periods. Does NOT reset.
func (r *Recorder) Cumulative() Snapshot {
	r.cumMu.Lock()
	s := Snapshot{
		MessagesSent:     r.totalSent.Load(),
		MessagesReceived: r.totalReceived.Load(),
		TotalSent:        r.totalSent.Load(),
		TotalReceived:    r.totalReceived.Load(),
		PublishLatency:   r.cumPublish.Clone(),
		EndToEndLatency:  r.cumE2E.Clone(),
	}
	r.cumMu.Unlock()
	return s
}

// Elapsed returns time since the Recorder was created.
func (r *Recorder) Elapsed() time.Duration {
	return time.Since(r.startTime)
}
