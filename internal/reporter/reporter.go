// Package reporter formats and prints benchmark metrics to stdout,
// mirroring OMB's periodic and final report style.
package reporter

import (
	"fmt"
	"strings"
	"time"

	"github.com/redpanda-data/go-bench/internal/metrics"
)

// microsToMillis converts microseconds to milliseconds.
func microsToMillis(us int64) float64 {
	return float64(us) / 1000.0
}

// FormatPeriod formats a two-line period report string (does not print it).
// elapsed is the wall-clock duration of this reporting period.
func FormatPeriod(snap metrics.Snapshot, elapsed time.Duration) string {
	secs := elapsed.Seconds()
	if secs <= 0 {
		secs = 1
	}

	publishRate := float64(snap.MessagesSent) / secs
	consumeRate := float64(snap.MessagesReceived) / secs
	publishThroughputMBs := float64(snap.BytesSent) / secs / 1024 / 1024
	consumeThroughputMBs := float64(snap.BytesReceived) / secs / 1024 / 1024
	backlogK := float64(snap.TotalSent-snap.TotalReceived) / 1000.0

	var sb strings.Builder
	fmt.Fprintf(&sb,
		"Pub rate %.0f msg/s / %.2f MB/s | Cons rate %.0f msg/s / %.2f MB/s | Backlog: %.1f K | "+
			"Pub Latency (ms) avg: %.2f - 50%%: %.2f - 99%%: %.2f - 99.9%%: %.2f - Max: %.2f",
		publishRate, publishThroughputMBs,
		consumeRate, consumeThroughputMBs,
		backlogK,
		microsToMillis(int64(snap.PublishLatency.Mean())),
		microsToMillis(snap.PublishLatency.ValueAtPercentile(50)),
		microsToMillis(snap.PublishLatency.ValueAtPercentile(99)),
		microsToMillis(snap.PublishLatency.ValueAtPercentile(99.9)),
		microsToMillis(snap.PublishLatency.Max()),
	)
	fmt.Fprintf(&sb,
		"\nE2E Latency (ms) avg: %.2f - 50%%: %.2f - 99%%: %.2f - 99.9%%: %.2f - Max: %.2f",
		microsToMillis(int64(snap.EndToEndLatency.Mean())),
		microsToMillis(snap.EndToEndLatency.ValueAtPercentile(50)),
		microsToMillis(snap.EndToEndLatency.ValueAtPercentile(99)),
		microsToMillis(snap.EndToEndLatency.ValueAtPercentile(99.9)),
		microsToMillis(snap.EndToEndLatency.Max()),
	)
	return sb.String()
}

// FormatFinal formats the aggregated final summary (does not print it).
func FormatFinal(snap metrics.Snapshot) string {
	var sb strings.Builder
	fmt.Fprintf(&sb,
		"----- Aggregated Pub Latency (ms) avg: %.2f - 50%%: %.2f - 95%%: %.2f - 99%%: %.2f - 99.9%%: %.2f - 99.99%%: %.2f - Max: %.2f",
		microsToMillis(int64(snap.PublishLatency.Mean())),
		microsToMillis(snap.PublishLatency.ValueAtPercentile(50)),
		microsToMillis(snap.PublishLatency.ValueAtPercentile(95)),
		microsToMillis(snap.PublishLatency.ValueAtPercentile(99)),
		microsToMillis(snap.PublishLatency.ValueAtPercentile(99.9)),
		microsToMillis(snap.PublishLatency.ValueAtPercentile(99.99)),
		microsToMillis(snap.PublishLatency.Max()),
	)
	fmt.Fprintf(&sb,
		"\n----- Aggregated E2E Latency (ms) avg: %.2f - 50%%: %.2f - 95%%: %.2f - 99%%: %.2f - 99.9%%: %.2f - 99.99%%: %.2f - Max: %.2f",
		microsToMillis(int64(snap.EndToEndLatency.Mean())),
		microsToMillis(snap.EndToEndLatency.ValueAtPercentile(50)),
		microsToMillis(snap.EndToEndLatency.ValueAtPercentile(95)),
		microsToMillis(snap.EndToEndLatency.ValueAtPercentile(99)),
		microsToMillis(snap.EndToEndLatency.ValueAtPercentile(99.9)),
		microsToMillis(snap.EndToEndLatency.ValueAtPercentile(99.99)),
		microsToMillis(snap.EndToEndLatency.Max()),
	)
	return sb.String()
}

// PrintPeriod prints a formatted period report to stdout.
func PrintPeriod(snap metrics.Snapshot, elapsed time.Duration) {
	fmt.Println(FormatPeriod(snap, elapsed))
}

// PrintFinal prints the aggregated final summary to stdout.
func PrintFinal(snap metrics.Snapshot) {
	fmt.Println(FormatFinal(snap))
}
