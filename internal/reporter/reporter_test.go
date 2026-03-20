package reporter_test

import (
	"strings"
	"testing"
	"time"

	"github.com/redpanda-data/go-bench/internal/histogram"
	"github.com/redpanda-data/go-bench/internal/metrics"
	"github.com/redpanda-data/go-bench/internal/reporter"
)

func makeSnapshot(sent, received int64, pubLatUS, e2eLatUS int64) metrics.Snapshot {
	pubH := histogram.New()
	pubH.RecordValue(pubLatUS)
	e2eH := histogram.New()
	e2eH.RecordValue(e2eLatUS)
	return metrics.Snapshot{
		MessagesSent:     sent,
		BytesSent:        sent * 1024,
		MessagesReceived: received,
		BytesReceived:    received * 1024,
		TotalSent:        sent,
		TotalReceived:    received,
		PublishLatency:   pubH,
		EndToEndLatency:  e2eH,
	}
}

func TestPeriodLineContainsKeyFields(t *testing.T) {
	snap := makeSnapshot(5000, 5000, 1000, 2000) // 1ms pub lat, 2ms e2e
	elapsed := 10 * time.Second
	out := reporter.FormatPeriod(snap, elapsed)

	for _, want := range []string{"msg/s", "MB/s", "Backlog", "Pub Latency", "E2E Latency"} {
		if !strings.Contains(out, want) {
			t.Errorf("period output missing %q\nOutput: %s", want, out)
		}
	}
}

func TestFinalLineContainsKeyFields(t *testing.T) {
	snap := makeSnapshot(100000, 100000, 1000, 2000)
	out := reporter.FormatFinal(snap)

	for _, want := range []string{"Aggregated Pub Latency", "Aggregated E2E Latency", "50%", "99%", "Max"} {
		if !strings.Contains(out, want) {
			t.Errorf("final output missing %q\nOutput: %s", want, out)
		}
	}
}

func TestRatesAreNonZero(t *testing.T) {
	snap := makeSnapshot(1000, 1000, 500, 1000)
	out := reporter.FormatPeriod(snap, 10*time.Second)
	// 1000 msgs / 10s = 100 msg/s — should appear in output
	if !strings.Contains(out, "100") {
		t.Errorf("expected rate ~100 in output, got: %s", out)
	}
}
