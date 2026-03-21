package histogram_test

import (
	"testing"

	"github.com/redpanda-data/go-bench/internal/histogram"
)

func TestRecordAndPercentile(t *testing.T) {
	h := histogram.New()
	for i := int64(1); i <= 100; i++ {
		h.RecordValue(i) // values 1..100 microseconds
	}
	p50 := h.ValueAtPercentile(50)
	if p50 < 49 || p50 > 51 {
		t.Errorf("p50 = %d, want ~50", p50)
	}
	p99 := h.ValueAtPercentile(99)
	if p99 < 98 || p99 > 100 {
		t.Errorf("p99 = %d, want ~99", p99)
	}
}

func TestMean(t *testing.T) {
	h := histogram.New()
	for i := int64(1); i <= 10; i++ {
		h.RecordValue(i)
	}
	mean := h.Mean()
	if mean < 5.0 || mean > 6.0 {
		t.Errorf("mean = %f, want ~5.5", mean)
	}
}

func TestMerge(t *testing.T) {
	a := histogram.New()
	b := histogram.New()
	a.RecordValue(10)
	b.RecordValue(20)
	a.Merge(b)
	if a.TotalCount() != 2 {
		t.Errorf("TotalCount = %d, want 2", a.TotalCount())
	}
}

func TestReset(t *testing.T) {
	h := histogram.New()
	h.RecordValue(100)
	h.Reset()
	if h.TotalCount() != 0 {
		t.Errorf("TotalCount after reset = %d, want 0", h.TotalCount())
	}
}

func TestMaxValue(t *testing.T) {
	h := histogram.New()
	for _, v := range []int64{1, 5, 3, 9, 2} {
		h.RecordValue(v)
	}
	if h.Max() != 9 {
		t.Errorf("Max = %d, want 9", h.Max())
	}
}

func TestRecordCorrectedValue(t *testing.T) {
	h := histogram.New()
	// Record 10000µs with expected interval 1000µs.
	// HDR injects phantom samples at: 9000, 8000, 7000, 6000, 5000, 4000, 3000, 2000, 1000
	// Total = 1 real + 9 phantom = 10
	h.RecordCorrectedValue(10000, 1000)
	if h.TotalCount() != 10 {
		t.Errorf("TotalCount = %d, want 10 (1 real + 9 phantom)", h.TotalCount())
	}
	// Max should be the real value
	if h.Max() < 9000 {
		t.Errorf("Max = %d, want >= 9000", h.Max())
	}
}
