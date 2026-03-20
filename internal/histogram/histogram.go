// Package histogram wraps hdrhistogram-go with a minimal API for latency recording.
// Values are stored in microseconds. Callers must use consistent units.
package histogram

import hdr "github.com/HdrHistogram/hdrhistogram-go"

const (
	minValue   = 1
	maxValue   = 60_000_000 // 60 seconds in microseconds
	sigFigures = 3
)

// Histogram is a thin wrapper around an HDR histogram.
type Histogram struct {
	h *hdr.Histogram
}

// New returns a Histogram configured for microsecond latency values (1µs – 60s).
func New() *Histogram {
	return &Histogram{h: hdr.New(minValue, maxValue, sigFigures)}
}

// RecordValue records a single value (in microseconds). Silently drops out-of-range values.
func (h *Histogram) RecordValue(v int64) {
	_ = h.h.RecordValue(v) // ignore range errors — clamp at caller if needed
}

// ValueAtPercentile returns the value at the given percentile (0–100).
func (h *Histogram) ValueAtPercentile(p float64) int64 {
	return h.h.ValueAtPercentile(p)
}

// Mean returns the arithmetic mean of recorded values.
func (h *Histogram) Mean() float64 {
	return h.h.Mean()
}

// Max returns the maximum recorded value.
func (h *Histogram) Max() int64 {
	return h.h.Max()
}

// TotalCount returns the number of recorded values.
func (h *Histogram) TotalCount() int64 {
	return h.h.TotalCount()
}

// Reset clears all recorded data.
func (h *Histogram) Reset() {
	h.h.Reset()
}

// Merge adds all values from other into h.
func (h *Histogram) Merge(other *Histogram) {
	h.h.Merge(other.h)
}

// Clone returns a deep copy of this histogram.
func (h *Histogram) Clone() *Histogram {
	return &Histogram{h: hdr.Import(h.h.Export())}
}
