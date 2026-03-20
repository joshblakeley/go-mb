# HTML Report Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--output report.html` flag to `bench run` that writes a self-contained HTML file with four Chart.js charts (rates, throughput, publish latency, E2E latency) and an aggregated latency summary table.

**Architecture:** One new package `internal/results` owns the data types (`DataPoint`, `RunMeta`, `FinalSummary`, `Run`), constructor functions, and `WriteHTML`. The existing `bench.go` reporter goroutine appends a `DataPoint` per tick; after the final report it calls `results.WriteHTML` if `--output` is set. No existing package boundaries change.

**Tech Stack:** Go 1.25, `html/template` (stdlib), `encoding/json` (stdlib), Chart.js 4.x via CDN.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `internal/results/results.go` | Create | Data types + constructors (`DataPointFromSnapshot`, `FinalSummaryFromSnapshot`, `RunMetaFromConfig`) |
| `internal/results/html.go` | Create | `WriteHTML` + HTML template constant |
| `internal/results/results_test.go` | Create | 5 unit tests covering types and HTML output |
| `internal/config/config.go` | Modify | Add `OutputFile string` field |
| `cmd/bench/main.go` | Modify | Add `--output` flag |
| `internal/bench/bench.go` | Modify | Collect DataPoints + call WriteHTML |

---

## Task 1: Data types and constructors (`results.go`)

**Files:**
- Create: `internal/results/results_test.go`
- Create: `internal/results/results.go`

### Background

`metrics.Snapshot` (from `internal/metrics`) has these relevant fields:
- `MessagesSent int64`, `BytesSent int64`, `MessagesReceived int64`, `BytesReceived int64`
- `TotalSent int64`, `TotalReceived int64` (cumulative — never reset)
- `PublishLatency *histogram.Histogram`, `EndToEndLatency *histogram.Histogram`

`histogram.Histogram` methods: `Mean() float64`, `Max() int64`, `ValueAtPercentile(p float64) int64`.

All histogram values are in **microseconds**. Convert to ms by dividing by 1000.0.

- [ ] **Step 1: Write the failing tests**

Create `internal/results/results_test.go`:

```go
package results_test

import (
	"testing"
	"time"

	"github.com/redpanda-data/go-bench/internal/config"
	"github.com/redpanda-data/go-bench/internal/histogram"
	"github.com/redpanda-data/go-bench/internal/metrics"
	"github.com/redpanda-data/go-bench/internal/results"
)

// makeSnap constructs a metrics.Snapshot for testing.
// pubVals/e2eVals are histogram values in microseconds.
func makeSnap(pubVals, e2eVals []int64, msgsSent, bytesSent, msgsRecv, bytesRecv, totalSent, totalRecv int64) metrics.Snapshot {
	pubH := histogram.New()
	for _, v := range pubVals {
		pubH.RecordValue(v)
	}
	e2eH := histogram.New()
	for _, v := range e2eVals {
		e2eH.RecordValue(v)
	}
	return metrics.Snapshot{
		MessagesSent:     msgsSent,
		BytesSent:        bytesSent,
		MessagesReceived: msgsRecv,
		BytesReceived:    bytesRecv,
		TotalSent:        totalSent,
		TotalReceived:    totalRecv,
		PublishLatency:   pubH,
		EndToEndLatency:  e2eH,
	}
}

func TestDataPointFromSnapshot(t *testing.T) {
	// pub histogram [200, 200, 300, 400] µs; e2e [300, 300, 500, 600] µs
	snap := makeSnap(
		[]int64{200, 200, 300, 400},
		[]int64{300, 300, 500, 600},
		1000, 1024*1000, // msgs sent, bytes sent
		1000, 1024*1000, // msgs recv, bytes recv
		5000, 4900,      // totalSent, totalRecv
	)
	elapsed := 1 * time.Second
	elapsedSinceStart := 10.0

	dp := results.DataPointFromSnapshot(snap, elapsed, elapsedSinceStart)

	if dp.ElapsedSecs != 10.0 {
		t.Errorf("ElapsedSecs = %v, want 10.0", dp.ElapsedSecs)
	}
	if dp.PubRateMsgPerSec != 1000.0 {
		t.Errorf("PubRateMsgPerSec = %v, want 1000.0", dp.PubRateMsgPerSec)
	}
	// p99 of [200, 200, 300, 400] µs → 400 µs → 0.4 ms
	if dp.PubLatencyP99Ms != 0.4 {
		t.Errorf("PubLatencyP99Ms = %v, want 0.4", dp.PubLatencyP99Ms)
	}
	// p50 of [300, 300, 500, 600] µs → 300 µs → 0.3 ms
	if dp.E2ELatencyP50Ms != 0.3 {
		t.Errorf("E2ELatencyP50Ms = %v, want 0.3", dp.E2ELatencyP50Ms)
	}
	// BacklogK: (5000 - 4900) / 1000 = 0.1
	if dp.BacklogK != 0.1 {
		t.Errorf("BacklogK = %v, want 0.1", dp.BacklogK)
	}
}

func TestFinalSummaryFromSnapshot(t *testing.T) {
	// 100 values: 1..100 µs for pub; 2..200 µs for e2e
	pubH := histogram.New()
	e2eH := histogram.New()
	for i := int64(1); i <= 100; i++ {
		pubH.RecordValue(i)
		e2eH.RecordValue(i * 2)
	}
	snap := metrics.Snapshot{
		PublishLatency:  pubH,
		EndToEndLatency: e2eH,
	}

	s := results.FinalSummaryFromSnapshot(snap)

	// p95 of 1-100 µs ≈ 95 µs → 0.095 ms (HDR precision ±0.1%)
	if s.PubP95Ms < 0.094 || s.PubP95Ms > 0.096 {
		t.Errorf("PubP95Ms = %.4f, want ~0.095", s.PubP95Ms)
	}
	// p99.99 of 2-200 µs should be near max (200 µs → 0.200 ms)
	if s.E2EP9999Ms < 0.19 || s.E2EP9999Ms > 0.201 {
		t.Errorf("E2EP9999Ms = %.4f, want ~0.200", s.E2EP9999Ms)
	}
}

func TestRunMetaFromConfig(t *testing.T) {
	cfg := &config.Config{
		Brokers:        []string{"localhost:9092"},
		Topic:          "test-topic",
		Producers:      4,
		Consumers:      2,
		MessageSize:    2048,
		Duration:       30 * time.Second,
		ReportInterval: 5 * time.Second,
	}
	ts := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	meta := results.RunMetaFromConfig(cfg, ts)

	if meta.Producers != 4 {
		t.Errorf("Producers = %v, want 4", meta.Producers)
	}
	if meta.MessageSize != 2048 {
		t.Errorf("MessageSize = %v, want 2048", meta.MessageSize)
	}
	if meta.Duration != 30*time.Second {
		t.Errorf("Duration = %v, want 30s", meta.Duration)
	}
	if !meta.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", meta.Timestamp, ts)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/results/ -v
```

Expected: compile error — `package results` not found.

- [ ] **Step 3: Implement `results.go`**

Create `internal/results/results.go`:

```go
// Package results provides data types, constructors, and HTML report generation
// for benchmark run output.
package results

import (
	"time"

	"github.com/redpanda-data/go-bench/internal/config"
	"github.com/redpanda-data/go-bench/internal/metrics"
)

// DataPoint holds per-interval benchmark metrics for one reporting tick.
type DataPoint struct {
	ElapsedSecs       float64 // seconds since benchmark start (chart x-axis)
	PubRateMsgPerSec  float64
	PubRateMBPerSec   float64
	ConsRateMsgPerSec float64
	ConsRateMBPerSec  float64
	BacklogK          float64 // total in-flight messages / 1000
	PubLatencyAvgMs   float64
	PubLatencyP50Ms   float64
	PubLatencyP99Ms   float64
	PubLatencyP999Ms  float64
	PubLatencyMaxMs   float64
	E2ELatencyAvgMs   float64
	E2ELatencyP50Ms   float64
	E2ELatencyP99Ms   float64
	E2ELatencyP999Ms  float64
	E2ELatencyMaxMs   float64
}

// RunMeta holds display-relevant config for the report header.
// Operational fields (partitions, RF, consumer group, etc.) are intentionally excluded.
type RunMeta struct {
	Brokers        []string
	Topic          string
	Producers      int
	Consumers      int
	MessageSize    int
	Duration       time.Duration
	ReportInterval time.Duration
	Timestamp      time.Time // benchmark start (after warmup)
}

// FinalSummary holds aggregated latency percentiles for the summary table.
// Mirrors the percentiles in reporter.FormatFinal.
type FinalSummary struct {
	PubAvgMs   float64
	PubP50Ms   float64
	PubP95Ms   float64
	PubP99Ms   float64
	PubP999Ms  float64
	PubP9999Ms float64
	PubMaxMs   float64
	E2EAvgMs   float64
	E2EP50Ms   float64
	E2EP95Ms   float64
	E2EP99Ms   float64
	E2EP999Ms  float64
	E2EP9999Ms float64
	E2EMaxMs   float64
}

// Run bundles all data for a single benchmark run.
type Run struct {
	Meta    RunMeta
	Points  []DataPoint
	Summary FinalSummary
}

// DataPointFromSnapshot constructs a DataPoint from a period snapshot.
//
// elapsed is the period duration (t.Sub(lastTick)) — used to compute per-second rates.
// elapsedSinceStart is time.Since(benchmarkStart).Seconds() — the chart x-axis value.
// These are different: elapsed is the width of this interval; elapsedSinceStart is total
// time since the benchmark began.
func DataPointFromSnapshot(snap metrics.Snapshot, elapsed time.Duration, elapsedSinceStart float64) DataPoint {
	secs := elapsed.Seconds()
	if secs <= 0 {
		secs = 1
	}
	return DataPoint{
		ElapsedSecs:       elapsedSinceStart,
		PubRateMsgPerSec:  float64(snap.MessagesSent) / secs,
		PubRateMBPerSec:   float64(snap.BytesSent) / secs / 1024 / 1024,
		ConsRateMsgPerSec: float64(snap.MessagesReceived) / secs,
		ConsRateMBPerSec:  float64(snap.BytesReceived) / secs / 1024 / 1024,
		// TotalSent/TotalReceived are cumulative (not reset per interval).
		BacklogK:         float64(snap.TotalSent-snap.TotalReceived) / 1000.0,
		PubLatencyAvgMs:  snap.PublishLatency.Mean() / 1000.0,
		PubLatencyP50Ms:  float64(snap.PublishLatency.ValueAtPercentile(50)) / 1000.0,
		PubLatencyP99Ms:  float64(snap.PublishLatency.ValueAtPercentile(99)) / 1000.0,
		PubLatencyP999Ms: float64(snap.PublishLatency.ValueAtPercentile(99.9)) / 1000.0,
		PubLatencyMaxMs:  float64(snap.PublishLatency.Max()) / 1000.0,
		E2ELatencyAvgMs:  snap.EndToEndLatency.Mean() / 1000.0,
		E2ELatencyP50Ms:  float64(snap.EndToEndLatency.ValueAtPercentile(50)) / 1000.0,
		E2ELatencyP99Ms:  float64(snap.EndToEndLatency.ValueAtPercentile(99)) / 1000.0,
		E2ELatencyP999Ms: float64(snap.EndToEndLatency.ValueAtPercentile(99.9)) / 1000.0,
		E2ELatencyMaxMs:  float64(snap.EndToEndLatency.Max()) / 1000.0,
	}
}

// FinalSummaryFromSnapshot constructs a FinalSummary from a cumulative snapshot.
// Called with rec.Cumulative() — only histogram fields are populated in that snapshot.
// Uses float64 Mean() directly (no int64 cast) to preserve sub-microsecond precision.
func FinalSummaryFromSnapshot(snap metrics.Snapshot) FinalSummary {
	return FinalSummary{
		PubAvgMs:   snap.PublishLatency.Mean() / 1000.0,
		PubP50Ms:   float64(snap.PublishLatency.ValueAtPercentile(50)) / 1000.0,
		PubP95Ms:   float64(snap.PublishLatency.ValueAtPercentile(95)) / 1000.0,
		PubP99Ms:   float64(snap.PublishLatency.ValueAtPercentile(99)) / 1000.0,
		PubP999Ms:  float64(snap.PublishLatency.ValueAtPercentile(99.9)) / 1000.0,
		PubP9999Ms: float64(snap.PublishLatency.ValueAtPercentile(99.99)) / 1000.0,
		PubMaxMs:   float64(snap.PublishLatency.Max()) / 1000.0,
		E2EAvgMs:   snap.EndToEndLatency.Mean() / 1000.0,
		E2EP50Ms:   float64(snap.EndToEndLatency.ValueAtPercentile(50)) / 1000.0,
		E2EP95Ms:   float64(snap.EndToEndLatency.ValueAtPercentile(95)) / 1000.0,
		E2EP99Ms:   float64(snap.EndToEndLatency.ValueAtPercentile(99)) / 1000.0,
		E2EP999Ms:  float64(snap.EndToEndLatency.ValueAtPercentile(99.9)) / 1000.0,
		E2EP9999Ms: float64(snap.EndToEndLatency.ValueAtPercentile(99.99)) / 1000.0,
		E2EMaxMs:   float64(snap.EndToEndLatency.Max()) / 1000.0,
	}
}

// RunMetaFromConfig constructs a RunMeta from config.
// benchmarkStart is the time the benchmark phase began (after warmup) — used as Timestamp.
func RunMetaFromConfig(cfg *config.Config, benchmarkStart time.Time) RunMeta {
	return RunMeta{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		Producers:      cfg.Producers,
		Consumers:      cfg.Consumers,
		MessageSize:    cfg.MessageSize,
		Duration:       cfg.Duration,
		ReportInterval: cfg.ReportInterval,
		Timestamp:      benchmarkStart,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/results/ -v -run 'TestDataPointFromSnapshot|TestFinalSummaryFromSnapshot|TestRunMetaFromConfig'
```

Expected: 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/results/results.go internal/results/results_test.go
git commit -m "feat: add results package with DataPoint, RunMeta, FinalSummary types and constructors"
```

---

## Task 2: HTML generation (`html.go`)

**Files:**
- Modify: `internal/results/results_test.go` (add 2 tests)
- Create: `internal/results/html.go`

### Background

`WriteHTML` marshals `[]DataPoint` into a JSON array that is inlined into a `<script>` block in the HTML. Chart.js reads this JSON array and renders 4 line charts. The `html/template` package is used for rendering; the JSON data is wrapped in `template.JS` to prevent HTML-escaping.

The HTML file is self-contained — it references Chart.js from CDN (`https://cdn.jsdelivr.net/npm/chart.js`) but contains all data inline. No separate JSON file is needed.

- [ ] **Step 1: Add failing HTML tests to `results_test.go`**

Add `"os"`, `"path/filepath"`, and `"strings"` to the **existing** import block at the top of `internal/results/results_test.go` (do not add a second `import` block — merge into the one already there). Then append the two test functions below to the file:


func TestWriteHTMLCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.html")

	run := &results.Run{
		Meta: results.RunMeta{
			Brokers:        []string{"localhost:9092"},
			Topic:          "test-topic",
			Producers:      2,
			Consumers:      2,
			MessageSize:    1024,
			Duration:       30 * time.Second,
			ReportInterval: 5 * time.Second,
			Timestamp:      time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		},
		Points: []results.DataPoint{
			{ElapsedSecs: 5.0, PubRateMsgPerSec: 1000, ConsRateMsgPerSec: 1000,
				PubLatencyAvgMs: 0.2, E2ELatencyAvgMs: 0.3},
			{ElapsedSecs: 10.0, PubRateMsgPerSec: 1100, ConsRateMsgPerSec: 1100,
				PubLatencyAvgMs: 0.21, E2ELatencyAvgMs: 0.31},
		},
		Summary: results.FinalSummary{
			PubAvgMs: 0.20, PubP50Ms: 0.20, PubP95Ms: 0.27,
			PubP99Ms: 0.33, PubP999Ms: 0.46, PubP9999Ms: 0.81, PubMaxMs: 1.5,
			E2EAvgMs: 0.32, E2EP50Ms: 0.33, E2EP95Ms: 0.47,
			E2EP99Ms: 0.55, E2EP999Ms: 0.75, E2EP9999Ms: 1.43, E2EMaxMs: 2.2,
		},
	}

	if err := results.WriteHTML(run, path); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	html := string(content)

	checks := []struct {
		substr string
		desc   string
	}{
		{"chart.js", "Chart.js CDN reference"},
		{"localhost:9092", "broker address in report header"},
		{"Aggregated Latency Summary", "summary section heading"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.substr) {
			t.Errorf("expected HTML to contain %q (%s)", c.substr, c.desc)
		}
	}
}

func TestWriteHTMLInvalidPath(t *testing.T) {
	run := &results.Run{
		Meta: results.RunMeta{Brokers: []string{"b:9092"}},
	}
	err := results.WriteHTML(run, "/nonexistent-dir-gobench/report.html")
	if err == nil {
		t.Error("expected error writing to invalid path, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/results/ -v -run 'TestWriteHTML'
```

Expected: compile error — `WriteHTML` not defined.

- [ ] **Step 3: Implement `html.go`**

Create `internal/results/html.go`:

```go
package results

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"strings"
)

// chartData is the JSON payload embedded in the <script> block of the HTML report.
// All slice fields are parallel arrays indexed by reporting interval.
type chartData struct {
	Labels            []float64 `json:"labels"`
	PubRateMsgPerSec  []float64 `json:"pubRateMsgPerSec"`
	ConsRateMsgPerSec []float64 `json:"consRateMsgPerSec"`
	PubRateMBPerSec   []float64 `json:"pubRateMBPerSec"`
	ConsRateMBPerSec  []float64 `json:"consRateMBPerSec"`
	PubLatencyAvg     []float64 `json:"pubLatencyAvg"`
	PubLatencyP50     []float64 `json:"pubLatencyP50"`
	PubLatencyP99     []float64 `json:"pubLatencyP99"`
	PubLatencyP999    []float64 `json:"pubLatencyP999"`
	PubLatencyMax     []float64 `json:"pubLatencyMax"`
	E2ELatencyAvg     []float64 `json:"e2eLatencyAvg"`
	E2ELatencyP50     []float64 `json:"e2eLatencyP50"`
	E2ELatencyP99     []float64 `json:"e2eLatencyP99"`
	E2ELatencyP999    []float64 `json:"e2eLatencyP999"`
	E2ELatencyMax     []float64 `json:"e2eLatencyMax"`
}

// templateVars is passed to the HTML template.
type templateVars struct {
	BrokersStr  string
	Topic       string
	Producers   int
	Consumers   int
	MessageSize int
	Duration    string
	Timestamp   string
	Summary     FinalSummary
	ChartJSON   template.JS // pre-marshalled JSON, safe to embed in <script>
}

// WriteHTML renders a self-contained HTML report to path.
// Returns an error if the file cannot be created or the template fails to execute.
func WriteHTML(run *Run, path string) error {
	// Build parallel chart data arrays from DataPoints.
	cd := chartData{}
	for _, p := range run.Points {
		cd.Labels = append(cd.Labels, p.ElapsedSecs)
		cd.PubRateMsgPerSec = append(cd.PubRateMsgPerSec, p.PubRateMsgPerSec)
		cd.ConsRateMsgPerSec = append(cd.ConsRateMsgPerSec, p.ConsRateMsgPerSec)
		cd.PubRateMBPerSec = append(cd.PubRateMBPerSec, p.PubRateMBPerSec)
		cd.ConsRateMBPerSec = append(cd.ConsRateMBPerSec, p.ConsRateMBPerSec)
		cd.PubLatencyAvg = append(cd.PubLatencyAvg, p.PubLatencyAvgMs)
		cd.PubLatencyP50 = append(cd.PubLatencyP50, p.PubLatencyP50Ms)
		cd.PubLatencyP99 = append(cd.PubLatencyP99, p.PubLatencyP99Ms)
		cd.PubLatencyP999 = append(cd.PubLatencyP999, p.PubLatencyP999Ms)
		cd.PubLatencyMax = append(cd.PubLatencyMax, p.PubLatencyMaxMs)
		cd.E2ELatencyAvg = append(cd.E2ELatencyAvg, p.E2ELatencyAvgMs)
		cd.E2ELatencyP50 = append(cd.E2ELatencyP50, p.E2ELatencyP50Ms)
		cd.E2ELatencyP99 = append(cd.E2ELatencyP99, p.E2ELatencyP99Ms)
		cd.E2ELatencyP999 = append(cd.E2ELatencyP999, p.E2ELatencyP999Ms)
		cd.E2ELatencyMax = append(cd.E2ELatencyMax, p.E2ELatencyMaxMs)
	}

	jsonBytes, err := json.Marshal(cd)
	if err != nil {
		return fmt.Errorf("marshal chart data: %w", err)
	}

	vars := templateVars{
		BrokersStr:  strings.Join(run.Meta.Brokers, ", "),
		Topic:       run.Meta.Topic,
		Producers:   run.Meta.Producers,
		Consumers:   run.Meta.Consumers,
		MessageSize: run.Meta.MessageSize,
		Duration:    run.Meta.Duration.String(),
		Timestamp:   run.Meta.Timestamp.Format("2006-01-02 15:04:05"),
		Summary:     run.Summary,
		ChartJSON:   template.JS(jsonBytes),
	}

	tmpl, err := template.New("report").Parse(reportTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, vars); err != nil {
		return fmt.Errorf("render template: %w", err)
	}
	return nil
}

const reportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>go-bench Report</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
<style>
* { box-sizing: border-box; }
body { font-family: system-ui, sans-serif; max-width: 1200px; margin: 0 auto; padding: 24px; color: #222; }
.meta { background: #f8f8f8; border: 1px solid #e0e0e0; border-radius: 8px; padding: 20px; margin-bottom: 28px; }
.meta h1 { margin: 0 0 12px; font-size: 1.3em; }
.meta-grid { display: grid; grid-template-columns: auto 1fr; gap: 4px 20px; }
.meta-grid dt { font-weight: 600; }
.meta-grid dd { margin: 0; }
.charts { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; margin-bottom: 32px; }
.chart-box { border: 1px solid #e0e0e0; border-radius: 8px; padding: 16px; }
.chart-box h2 { margin: 0 0 12px; font-size: 0.85em; font-weight: 600; color: #555;
  text-transform: uppercase; letter-spacing: 0.05em; }
h2.section { font-size: 1.1em; color: #222; text-transform: none; letter-spacing: 0; margin-bottom: 12px; }
table { border-collapse: collapse; width: 100%; font-size: 0.9em; }
thead th { background: #f0f0f0; font-weight: 600; text-align: right; padding: 8px 12px;
  border-bottom: 2px solid #ccc; }
thead th:first-child { text-align: left; }
tbody td { padding: 8px 12px; text-align: right; border-bottom: 1px solid #eee; }
tbody td:first-child { text-align: left; font-weight: 500; }
</style>
</head>
<body>
<div class="meta">
  <h1>go-bench Report</h1>
  <dl class="meta-grid">
    <dt>Brokers</dt><dd>{{.BrokersStr}}</dd>
    <dt>Topic</dt><dd>{{.Topic}}</dd>
    <dt>Producers</dt><dd>{{.Producers}}</dd>
    <dt>Consumers</dt><dd>{{.Consumers}}</dd>
    <dt>Message Size</dt><dd>{{.MessageSize}} bytes</dd>
    <dt>Duration</dt><dd>{{.Duration}}</dd>
    <dt>Timestamp</dt><dd>{{.Timestamp}}</dd>
  </dl>
</div>
<div class="charts">
  <div class="chart-box"><h2>Publish &amp; Consume Rate (msg/s)</h2><canvas id="c1"></canvas></div>
  <div class="chart-box"><h2>Throughput (MB/s)</h2><canvas id="c2"></canvas></div>
  <div class="chart-box"><h2>Publish Latency (ms)</h2><canvas id="c3"></canvas></div>
  <div class="chart-box"><h2>E2E Latency (ms)</h2><canvas id="c4"></canvas></div>
</div>
<h2 class="section">Aggregated Latency Summary</h2>
<table>
  <thead>
    <tr>
      <th></th>
      <th>Avg</th><th>p50</th><th>p95</th><th>p99</th><th>p99.9</th><th>p99.99</th><th>Max</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>Publish (ms)</td>
      <td>{{printf "%.3f" .Summary.PubAvgMs}}</td>
      <td>{{printf "%.3f" .Summary.PubP50Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubP95Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubP99Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubP999Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubP9999Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubMaxMs}}</td>
    </tr>
    <tr>
      <td>E2E (ms)</td>
      <td>{{printf "%.3f" .Summary.E2EAvgMs}}</td>
      <td>{{printf "%.3f" .Summary.E2EP50Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EP95Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EP99Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EP999Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EP9999Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EMaxMs}}</td>
    </tr>
  </tbody>
</table>
<script>
var d = {{.ChartJSON}};
function mkChart(id, yLabel, datasets) {
  new Chart(document.getElementById(id), {
    type: 'line',
    data: {
      labels: d.labels,
      datasets: datasets.map(function(ds) {
        return {label: ds[0], data: d[ds[1]], borderWidth: 1.5, pointRadius: 3, tension: 0.1};
      })
    },
    options: {
      responsive: true,
      plugins: {legend: {position: 'bottom'}},
      scales: {
        x: {title: {display: true, text: 'Elapsed (s)'}},
        y: {title: {display: true, text: yLabel}, beginAtZero: true}
      }
    }
  });
}
mkChart('c1', 'msg/s', [
  ['Publish Rate', 'pubRateMsgPerSec'],
  ['Consume Rate', 'consRateMsgPerSec']
]);
mkChart('c2', 'MB/s', [
  ['Publish', 'pubRateMBPerSec'],
  ['Consume', 'consRateMBPerSec']
]);
mkChart('c3', 'ms', [
  ['Avg', 'pubLatencyAvg'],
  ['p50', 'pubLatencyP50'],
  ['p99', 'pubLatencyP99'],
  ['p99.9', 'pubLatencyP999'],
  ['Max', 'pubLatencyMax']
]);
mkChart('c4', 'ms', [
  ['Avg', 'e2eLatencyAvg'],
  ['p50', 'e2eLatencyP50'],
  ['p99', 'e2eLatencyP99'],
  ['p99.9', 'e2eLatencyP999'],
  ['Max', 'e2eLatencyMax']
]);
</script>
</body>
</html>`
```

- [ ] **Step 4: Run all results tests to verify they pass**

```bash
go test ./internal/results/ -v
```

Expected: 5 tests PASS (`TestDataPointFromSnapshot`, `TestFinalSummaryFromSnapshot`, `TestRunMetaFromConfig`, `TestWriteHTMLCreatesFile`, `TestWriteHTMLInvalidPath`).

- [ ] **Step 5: Commit**

```bash
git add internal/results/html.go internal/results/results_test.go
git commit -m "feat: add WriteHTML with Chart.js template for HTML report generation"
```

---

## Task 3: Wire config, CLI flag, and bench.go

**Files:**
- Modify: `internal/config/config.go` (line 24 — after `DeleteTopic bool`)
- Modify: `cmd/bench/main.go` (line 56 — after the `DeleteTopic` flag)
- Modify: `internal/bench/bench.go` (lines 78–105)

### Background for `bench.go`

Current structure of `Run()` in `bench.go`:

```
line 78:  var wg sync.WaitGroup
line 81:  wg.Add(1)
line 82:  go func() {          // reporter goroutine
line 84:      ticker := ...
line 86:      lastTick := time.Now()
line 89:      case t := <-ticker.C:
line 90:          elapsed := t.Sub(lastTick)
line 91:          lastTick = t
line 92:          snap := rec.Snapshot()
line 93:          reporter.PrintPeriod(snap, elapsed)
line 98:  }()
line 100: runWorkers(...)
line 101: wg.Wait()
line 104: reporter.PrintFinal(rec.Cumulative())
```

Add `benchmarkStart` and `points` **between line 78 (`var wg sync.WaitGroup`) and line 81 (`wg.Add(1)`)**.
Add `points = append(...)` **after line 93 (`reporter.PrintPeriod`)**, still inside the ticker case.
Add the WriteHTML block **after line 104 (`reporter.PrintFinal`)**, before the cleanup block.

The `points` slice is written only inside the reporter goroutine and read only after `wg.Wait()`, so `wg.Wait()` provides the happens-before guarantee — no mutex needed.

- [ ] **Step 1: Add `OutputFile` to `config.go`**

In `internal/config/config.go`, add `OutputFile string` as the last field in the `Config` struct, after `DeleteTopic bool`:

```go
type Config struct {
    // ... existing fields ...
    DeleteTopic       bool
    OutputFile        string // path to write HTML report; empty means disabled
}
```

No change to `Default()` or `Validate()` — zero value `""` is correct.

- [ ] **Step 2: Add `--output` flag to `main.go`**

In `cmd/bench/main.go`, add after the `DeleteTopic` flag registration (line 56):

```go
f.StringVar(&cfg.OutputFile, "output", cfg.OutputFile, "path to write HTML report (e.g. report.html; empty = disabled)")
```

- [ ] **Step 3: Update `bench.go` — collect DataPoints and call WriteHTML**

In `internal/bench/bench.go`:

**3a. Add import** for the results package (add to the import block):
```go
"github.com/redpanda-data/go-bench/internal/results"
```

**3b. Declare `benchmarkStart` and `points`** between `var wg sync.WaitGroup` and `wg.Add(1)`:

```go
var wg sync.WaitGroup

benchmarkStart := time.Now()
var points []results.DataPoint

// Reporter goroutine.
wg.Add(1)
```

**3c. Append DataPoint inside the ticker case**, immediately after `reporter.PrintPeriod(snap, elapsed)`:

```go
case t := <-ticker.C:
    elapsed := t.Sub(lastTick)
    lastTick = t
    snap := rec.Snapshot()
    reporter.PrintPeriod(snap, elapsed)
    points = append(points, results.DataPointFromSnapshot(snap, elapsed, time.Since(benchmarkStart).Seconds()))
```

**3d. Add WriteHTML block** after `reporter.PrintFinal(rec.Cumulative())`:

```go
// 5. Final report.
fmt.Println()
reporter.PrintFinal(rec.Cumulative())

// 6. HTML report (optional).
if cfg.OutputFile != "" {
    run := results.Run{
        Meta:    results.RunMetaFromConfig(cfg, benchmarkStart),
        Points:  points,
        Summary: results.FinalSummaryFromSnapshot(rec.Cumulative()),
    }
    if err := results.WriteHTML(&run, cfg.OutputFile); err != nil {
        fmt.Printf("Warning: failed to write report: %v\n", err)
    } else {
        fmt.Printf("Report written to %s\n", cfg.OutputFile)
    }
}

// 7. Cleanup.
if cfg.DeleteTopic {
```

- [ ] **Step 4: Verify the build and existing tests pass**

```bash
go build ./...
```

Expected: no errors.

```bash
go test ./...
```

Expected: all tests pass (existing tests unchanged, 5 new results tests pass).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go cmd/bench/main.go internal/bench/bench.go
git commit -m "feat: wire --output flag and HTML report generation into bench run"
```

---

## Manual Verification

With Redpanda running via `docker compose up -d`:

```bash
go build -o bench ./cmd/bench

./bench run \
  --brokers localhost:9092 \
  --producers 2 --consumers 2 \
  --partitions 4 \
  --duration 30s --warmup 3s \
  --report-interval 5s \
  --create-topic --delete-topic \
  --output /tmp/bench-report.html

open /tmp/bench-report.html
```

Expected:
- Terminal shows the usual periodic stats + final aggregated lines
- Final line: `Report written to /tmp/bench-report.html`
- Browser shows: run metadata header, 4 Chart.js charts, "Aggregated Latency Summary" table
