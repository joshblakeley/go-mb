# HTML Report Generation Design

**Date:** 2026-03-20
**Status:** Approved by user

## Goal

Allow a benchmark run to produce a self-contained HTML report with Chart.js charts visualising throughput and latency metrics over time, written to a user-specified output path via `--output report.html`.

## Background

The tool currently prints OMB-style periodic stats to stdout. There is no machine-readable output and no visualisation. The user wants an experience analogous to OMB's `generate_charts.py` but without requiring Python or any external tooling — a single HTML file the user can open in any browser.

## Architecture

The feature adds one new package (`internal/results`) and touches three existing files (`config/config.go`, `cmd/bench/main.go`, `bench/bench.go`). No existing package boundaries change.

```
internal/results/
  results.go      — DataPoint, RunMeta, FinalSummary, Run types
  html.go         — WriteHTML(run *Run, path string) error
  results_test.go — unit tests for data types and WriteHTML
```

The `bench.go` reporter goroutine already fires on each tick; it will additionally append a `DataPoint` to a slice. After `PrintFinal`, if `Config.OutputFile` is non-empty, it calls `results.WriteHTML`.

## Data Types

### DataPoint
One entry per reporting interval:

| Field | Type | Description |
|---|---|---|
| ElapsedSecs | float64 | Seconds since benchmark start at this tick |
| PubRateMsgPerSec | float64 | Messages published per second in this interval |
| PubRateMBPerSec | float64 | MB published per second in this interval |
| ConsRateMsgPerSec | float64 | Messages consumed per second in this interval |
| ConsRateMBPerSec | float64 | MB consumed per second in this interval |
| BacklogK | float64 | Unconsumed messages (thousands) at this tick |
| PubLatencyAvgMs | float64 | Publish latency avg (ms) |
| PubLatencyP50Ms | float64 | Publish latency p50 (ms) |
| PubLatencyP99Ms | float64 | Publish latency p99 (ms) |
| PubLatencyP999Ms | float64 | Publish latency p99.9 (ms) |
| PubLatencyMaxMs | float64 | Publish latency max (ms) |
| E2ELatencyAvgMs | float64 | E2E latency avg (ms) |
| E2ELatencyP50Ms | float64 | E2E latency p50 (ms) |
| E2ELatencyP99Ms | float64 | E2E latency p99 (ms) |
| E2ELatencyP999Ms | float64 | E2E latency p99.9 (ms) |
| E2ELatencyMaxMs | float64 | E2E latency max (ms) |

### RunMeta
Config snapshot for the report header. Only fields relevant to display are captured; operational fields (partitions, RF, consumer group, create/delete flags, etc.) are intentionally excluded.

| Field | Type |
|---|---|
| Brokers | []string |
| Topic | string |
| Producers | int |
| Consumers | int |
| MessageSize | int |
| Duration | time.Duration |
| ReportInterval | time.Duration |
| Timestamp | time.Time |

### FinalSummary
Aggregated latency for the summary table. Mirrors the percentiles in `reporter.FormatFinal` (avg, p50, p95, p99, p99.9, p99.99, max). Note: `DataPoint` captures only p50/p99/p99.9/max for periodic intervals — p95 and p99.99 are only in `FinalSummary`.

| Field | Type | Description |
|---|---|---|
| PubAvgMs | float64 | Publish latency avg |
| PubP50Ms | float64 | Publish latency p50 |
| PubP95Ms | float64 | Publish latency p95 |
| PubP99Ms | float64 | Publish latency p99 |
| PubP999Ms | float64 | Publish latency p99.9 |
| PubP9999Ms | float64 | Publish latency p99.99 |
| PubMaxMs | float64 | Publish latency max |
| E2EAvgMs | float64 | E2E latency avg |
| E2EP50Ms | float64 | E2E latency p50 |
| E2EP95Ms | float64 | E2E latency p95 |
| E2EP99Ms | float64 | E2E latency p99 |
| E2EP999Ms | float64 | E2E latency p99.9 |
| E2EP9999Ms | float64 | E2E latency p99.99 |
| E2EMaxMs | float64 | E2E latency max |

### Run
```go
type Run struct {
    Meta    RunMeta
    Points  []DataPoint
    Summary FinalSummary
}
```

## Constructors in `results.go`

### DataPointFromSnapshot

```go
func DataPointFromSnapshot(snap metrics.Snapshot, elapsed time.Duration, elapsedSinceStart float64) DataPoint
```

- `elapsed` is the period duration (`t.Sub(lastTick)`) — used to compute per-second rates (same as `reporter.FormatPeriod`): `float64(snap.MessagesSent) / elapsed.Seconds()`
- `elapsedSinceStart` is `time.Since(benchmarkStart).Seconds()` — the x-axis value in charts. This is NOT `elapsed.Seconds()` (the period width). Use wall-clock `time.Since` rather than `t.Sub(benchmarkStart)` (the difference is negligible for charting). Pass it as a pre-computed `float64` to keep the constructor dependency-free.
- `BacklogK` = `float64(snap.TotalSent - snap.TotalReceived) / 1000.0`. Note: `Snapshot()` resets per-interval fields (`MessagesSent`, `BytesSent`, etc.) on each call, but `TotalSent`/`TotalReceived` are cumulative counters — so `BacklogK` reflects total messages in-flight since the run started, not just the current interval.
- All latency `*Ms` fields are converted from microseconds stored in the histograms. Unlike the reporter which casts `Mean()` to `int64` first, use the `float64` return value directly to preserve sub-microsecond precision: `snap.PublishLatency.Mean() / 1000.0`. For percentile/max (return `int64`): `float64(snap.PublishLatency.ValueAtPercentile(99)) / 1000.0`.

### FinalSummaryFromSnapshot

```go
func FinalSummaryFromSnapshot(snap metrics.Snapshot) FinalSummary
```

Called with `rec.Cumulative()`. Note: `Cumulative()` only populates `TotalSent`, `TotalReceived`, `PublishLatency`, and `EndToEndLatency` — byte and message count fields are zero. `FinalSummaryFromSnapshot` uses only the histogram fields, so this is correct.

All `*Ms` fields use the same microsecond-to-millisecond conversion as `DataPointFromSnapshot`:
- `PubAvgMs = snap.PublishLatency.Mean() / 1000.0`
- `PubP50Ms = float64(snap.PublishLatency.ValueAtPercentile(50)) / 1000.0`
- `PubP95Ms = float64(snap.PublishLatency.ValueAtPercentile(95)) / 1000.0`
- `PubP99Ms = float64(snap.PublishLatency.ValueAtPercentile(99)) / 1000.0`
- `PubP999Ms = float64(snap.PublishLatency.ValueAtPercentile(99.9)) / 1000.0`
- `PubP9999Ms = float64(snap.PublishLatency.ValueAtPercentile(99.99)) / 1000.0`
- `PubMaxMs = float64(snap.PublishLatency.Max()) / 1000.0`
- Same pattern for all `E2E*` fields using `snap.EndToEndLatency`

### RunMetaFromConfig

```go
func RunMetaFromConfig(cfg *config.Config, benchmarkStart time.Time) RunMeta
```

Copies display-relevant config fields directly. `Timestamp` is set to `benchmarkStart` (the time the benchmark phase began, after warmup). The caller passes in the same `benchmarkStart` variable used for `elapsedSinceStart` computation.

## WriteHTML

`WriteHTML(run *Run, path string) error` renders a self-contained HTML file using `html/template`. Chart.js is loaded from CDN (`https://cdn.jsdelivr.net/npm/chart.js`). The data is inlined as a JSON object in a `<script>` block — no external data file needed.

### HTML Structure

```
<header>   — Run metadata (brokers, producers, consumers, msgSize, duration, timestamp)
<charts>
  Chart 1: Publish & Consume Rate (msg/s) — dual line, time x-axis
  Chart 2: Throughput (MB/s)              — pub + cons lines
  Chart 3: Publish Latency (ms)           — avg, p50, p99, p99.9, max lines
  Chart 4: E2E Latency (ms)               — avg, p50, p99, p99.9, max lines
<summary>  — HTML table with heading "Aggregated Latency Summary" and final aggregated percentiles (pub + E2E)
```

Each chart has the elapsed-seconds values on the x-axis. The four charts are arranged in a 2×2 CSS grid.

## Changes to Existing Code

### `internal/config/config.go`
- Add `OutputFile string` field to `Config` struct.
- No validation rule needed (empty string = disabled).
- No change to `Default()` required — zero value `""` is correct. Do not add an explicit entry.

### `cmd/bench/main.go`
- Add `--output` flag: `cmd.Flags().StringVar(&cfg.OutputFile, "output", "", "path to write HTML report (e.g. report.html)")`

### `internal/bench/bench.go`
Declare `benchmarkStart` and the points slice after `var wg sync.WaitGroup` and immediately before the `wg.Add(1)` reporter goroutine launch. Do not change `lastTick := time.Now()` inside the goroutine — it remains a goroutine-local variable. The `points` slice is written only inside the reporter goroutine and read only after `wg.Wait()`, so the happens-before guarantee from `wg.Wait()` makes this race-free without any additional locking:

```go
benchmarkStart := time.Now()
var points []results.DataPoint

// reporter goroutine — inside the ticker case:
elapsed := t.Sub(lastTick)
lastTick = t
snap := rec.Snapshot()
reporter.PrintPeriod(snap, elapsed)
points = append(points, results.DataPointFromSnapshot(snap, elapsed, time.Since(benchmarkStart).Seconds()))
```

After `reporter.PrintFinal`:
```go
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
```

`RunMetaFromConfig(cfg *config.Config, benchmarkStart time.Time) RunMeta` and `FinalSummaryFromSnapshot(snap metrics.Snapshot) FinalSummary` are also exported from `results.go`.

## Testing

### Unit tests (`internal/results/results_test.go`)

1. **TestDataPointFromSnapshot** — call `DataPointFromSnapshot` with a known `metrics.Snapshot` (construct histograms via `histogram.New()` and `RecordValue()`) and known `elapsed`/`elapsedSinceStart` values; assert `PubRateMsgPerSec`, `PubLatencyP99Ms`, `E2ELatencyP50Ms`, and `ElapsedSecs` are correct.
2. **TestFinalSummaryFromSnapshot** — call `FinalSummaryFromSnapshot` with a known `metrics.Snapshot`; assert `PubP95Ms` and `E2EP9999Ms` are correct.
3. **TestRunMetaFromConfig** — call `RunMetaFromConfig` with a known `config.Config`; assert `Producers`, `MessageSize`, `Duration` match.
4. **TestWriteHTMLCreatesFile** — call `WriteHTML` with a minimal `Run` (2 data points), assert file exists and HTML contains `"chart.js"` (the CDN URL substring), the broker string, and `"Aggregated Latency Summary"` (the exact summary heading).
5. **TestWriteHTMLInvalidPath** — call `WriteHTML` with an unwritable path, assert non-nil error returned.

### Manual integration
```bash
./bench run --brokers localhost:9092 --duration 30s --output /tmp/report.html
open /tmp/report.html
```

## Out of Scope

- Auto-opening the browser (user opens the file manually)
- Comparing multiple runs in one report
- JSON output format
- Serving the report over HTTP
- Dark mode / theming
