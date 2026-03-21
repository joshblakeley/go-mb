# Coordinated Omission Correction Design

## Problem

go-bench producers run a synchronous send loop: acquire rate-limiter token → `ProduceSync` → record latency. When `ProduceSync` is slow (broker hiccup, GC pause), the goroutine stalls, no new messages are sent, and the system gets breathing room it would not get under real continuous load. The messages that *would have arrived* during the slow period are never sent, so their latency is never measured. The result is artificially low tail-latency percentiles.

This is coordinated omission — the benchmark inadvertently coordinates with slowdowns rather than fighting through them.

## Solution

Use HDR Histogram's `RecordCorrectedValue(v, expectedInterval)` to inject phantom samples for any interval skipped beyond the expected send cadence. For a measured latency `L` and expected interval `E`:
- If `L <= E`: record one sample (normal)
- If `L > E`: record `L`, plus synthetic samples at `L-E`, `L-2E`, … down to `E`, representing the requests that would have arrived during the stall

This is the same correction OMB applies. The math is equivalent; OMB works in nanoseconds, go-bench works in microseconds.

## Scope

- Correction applies when `--rate > 0`. When rate is unlimited (`--rate 0`), there is no expected interval and no correction is possible.
- Correction applies to **both** publish latency and E2E latency histograms, using the same expected interval. The rationale for applying the producer's expected interval to consumer-side histograms: during a producer stall, the broker receives no new messages, so consumers also stop receiving. The phantom samples represent the messages that *would have arrived at consumers* at the expected send cadence had no stall occurred. This is the same reasoning OMB applies when correcting both histogram sides from the producer rate.
- Throughput numbers (msg/s, MB/s, backlog) are unaffected — they use raw message counts.

## Changes

### `internal/histogram/histogram.go`

Add one method:

```go
// RecordCorrectedValue records v and fills in phantom samples for any time
// skipped beyond expectedInterval (coordinated omission correction).
// If expectedInterval <= 0, behaves like RecordValue.
func (h *Histogram) RecordCorrectedValue(v, expectedInterval int64) {
    _ = h.h.RecordCorrectedValue(v, expectedInterval)
}
```

### `internal/metrics/metrics.go`

- Add `expectedIntervalMicros int64` field to `Recorder`.
- Change `NewRecorder()` → `NewRecorder(expectedIntervalMicros int64)`.
- In `RecordSend`: call `RecordCorrectedValue(latencyMicros, r.expectedIntervalMicros)` on both `periodPublish` and `cumPublish` when `r.expectedIntervalMicros > 0`; otherwise call `RecordValue` (unchanged behaviour for unlimited rate).
- In `RecordReceive`: same pattern on `periodE2E` and `cumE2E`.

### `internal/bench/bench.go`

Compute the expected interval before creating the recorder and pass it to `NewRecorder`:

```go
var expectedIntervalMicros int64
if cfg.ProduceRate > 0 {
    expectedIntervalMicros = 1_000_000 / int64(cfg.ProduceRate)
}
rec := metrics.NewRecorder(expectedIntervalMicros)
```

## Files Updated (signature change propagation)

`NewRecorder()` → `NewRecorder(expectedIntervalMicros int64)` breaks all existing callers. Pass `0` (no correction, matching current behaviour) in:

- `internal/metrics/metrics_test.go` — every `NewRecorder()` call (6 sites)
- `internal/producer/producer_test.go` — 1 site
- `internal/consumer/consumer_test.go` — 2 sites

`internal/bench/bench.go` has **two** `NewRecorder()` call sites:
1. Main recorder creation (pre-warmup)
2. Post-warmup reset: `rec = metrics.NewRecorder(...)` inside the warmup block

Both must receive `expectedIntervalMicros`.

## Files Not Changed

- `internal/producer/producer.go` — correction is transparent to call sites
- `internal/consumer/consumer.go` — correction is transparent to call sites
- `internal/reporter/reporter.go` — reads histograms; no awareness of correction needed
- `internal/results/` — reads histograms; no awareness of correction needed
- `internal/config/config.go` — no new flag; correction is always on when rate > 0

## Expected Interval Derivation

`expectedIntervalMicros = 1_000_000 / cfg.ProduceRate`

Each producer goroutine runs at `cfg.ProduceRate` msg/s, so the expected gap between sends is `1_000_000 / rate` µs. Each goroutine corrects independently — this is correct because each goroutine is an independent send stream.

Example: `--rate 1000` → expected interval = 1000 µs (1 ms). A 10 ms stall injects 9 phantom samples at 9 ms, 8 ms, … 1 ms, matching OMB's behaviour.

## Testing

### Unit tests (no broker required)

**`internal/histogram/histogram_test.go`** — `TestRecordCorrectedValue`: record one value of 10000 µs with expected interval 1000 µs; assert `TotalCount() == 10` (1 real + 9 phantom samples).

**`internal/metrics/metrics_test.go`**:
- `TestRecorderCorrectedSend`: `NewRecorder(1000)`, call `RecordSend(latency=5000 µs)`, snapshot, assert `PublishLatency.TotalCount() == 5` (1 real + 4 phantom). Also assert `cumPublish.TotalCount() == 5` to verify both period and cumulative histograms are corrected.
- `TestRecorderCorrectedReceive`: `NewRecorder(1000)`, call `RecordReceive(e2eLatency=5000 µs)`, snapshot, assert `EndToEndLatency.TotalCount() == 5`.
- `TestRecorderNoCorrectionWhenZeroInterval`: `NewRecorder(0)`, `RecordSend(latency=5000)`, assert `TotalCount() == 1` (no phantom samples).

## Behaviour Impact

- **p99 / p99.9 / p99.99 will increase** when stalls occur — this is correct and expected.
- Under perfectly stable load (no stalls), correction adds no phantom samples and results are identical to uncorrected.
- Correction is silent and always active when rate > 0, matching OMB's approach.
