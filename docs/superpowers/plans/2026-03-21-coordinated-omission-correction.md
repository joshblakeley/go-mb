# Coordinated Omission Correction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply HDR Histogram's coordinated omission correction (`RecordCorrectedValue`) to publish and E2E latency histograms when a target rate is set, matching OMB's tail-latency accuracy.

**Architecture:** Add `RecordCorrectedValue` to the histogram wrapper, store `expectedIntervalMicros` in the `Recorder`, and use corrected recording in `RecordSend`/`RecordReceive` when the interval is non-zero. The bench orchestrator computes the interval from `cfg.ProduceRate` and passes it to `NewRecorder`. No changes to producer or consumer packages — correction is transparent to call sites.

**Tech Stack:** `github.com/HdrHistogram/hdrhistogram-go` (`RecordCorrectedValue` already exists on the underlying HDR type), Go stdlib.

---

## File Map

| File | Change |
|---|---|
| `internal/histogram/histogram.go` | Add `RecordCorrectedValue(v, expectedInterval int64)` method |
| `internal/histogram/histogram_test.go` | Add `TestRecordCorrectedValue` |
| `internal/metrics/metrics.go` | Add `expectedIntervalMicros` field; `NewRecorder() → NewRecorder(int64)`; conditional corrected recording in `RecordSend` and `RecordReceive` |
| `internal/metrics/metrics_test.go` | Add 3 new tests; update 6 existing `NewRecorder()` → `NewRecorder(0)` |
| `internal/producer/producer_test.go` | Update 1 `NewRecorder()` → `NewRecorder(0)` |
| `internal/consumer/consumer_test.go` | Update 2 `NewRecorder()` → `NewRecorder(0)` |
| `internal/bench/bench.go` | Compute `expectedIntervalMicros`; update both `NewRecorder()` call sites |

---

## Task 1: Add `RecordCorrectedValue` to histogram

**Files:**
- Modify: `internal/histogram/histogram.go`
- Modify: `internal/histogram/histogram_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/histogram/histogram_test.go`:

```go
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
```

- [ ] **Step 2: Run test — expect compile error**

```bash
cd /Users/josh/go-bench && go test ./internal/histogram/ -v -run TestRecordCorrectedValue
```

Expected: `h.RecordCorrectedValue undefined`

- [ ] **Step 3: Add `RecordCorrectedValue` to `internal/histogram/histogram.go`**

Insert after the `RecordValue` method (after line 26):

```go
// RecordCorrectedValue records v and fills in phantom samples for any time
// skipped beyond expectedInterval (coordinated omission correction).
// If expectedInterval <= 0, behaves like RecordValue.
func (h *Histogram) RecordCorrectedValue(v, expectedInterval int64) {
	_ = h.h.RecordCorrectedValue(v, expectedInterval)
}
```

- [ ] **Step 4: Run test — expect PASS**

```bash
cd /Users/josh/go-bench && go test ./internal/histogram/ -v -run TestRecordCorrectedValue
```

Expected: `PASS`

- [ ] **Step 5: Run full histogram test suite**

```bash
cd /Users/josh/go-bench && go test ./internal/histogram/ -v
```

Expected: all 6 tests PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/josh/go-bench && git add internal/histogram/histogram.go internal/histogram/histogram_test.go
git commit -m "feat: add RecordCorrectedValue to histogram for coordinated omission correction"
```

---

## Task 2: Update `Recorder` to use corrected recording

**Files:**
- Modify: `internal/metrics/metrics.go`
- Modify: `internal/metrics/metrics_test.go`
- Modify: `internal/producer/producer_test.go`
- Modify: `internal/consumer/consumer_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/metrics/metrics_test.go`:

```go
func TestRecorderCorrectedSend(t *testing.T) {
	// Interval 1000µs; latency 5000µs → 1 real + 4 phantom (4000, 3000, 2000, 1000) = 5 total.
	r := metrics.NewRecorder(1000)
	r.RecordSend(1024, 5000)
	snap := r.Snapshot()
	if snap.PublishLatency.TotalCount() != 5 {
		t.Errorf("period PublishLatency.TotalCount = %d, want 5", snap.PublishLatency.TotalCount())
	}
	// Cumulative histogram must also receive corrected values.
	cum := r.Cumulative()
	if cum.PublishLatency.TotalCount() != 5 {
		t.Errorf("cumulative PublishLatency.TotalCount = %d, want 5", cum.PublishLatency.TotalCount())
	}
}

func TestRecorderCorrectedReceive(t *testing.T) {
	// Same arithmetic: interval 1000µs, latency 5000µs → 5 total samples.
	r := metrics.NewRecorder(1000)
	r.RecordReceive(512, 5000)
	snap := r.Snapshot()
	if snap.EndToEndLatency.TotalCount() != 5 {
		t.Errorf("EndToEndLatency.TotalCount = %d, want 5", snap.EndToEndLatency.TotalCount())
	}
}

func TestRecorderNoCorrectionWhenZeroInterval(t *testing.T) {
	// Zero interval (unlimited rate) → plain RecordValue, 1 sample only.
	r := metrics.NewRecorder(0)
	r.RecordSend(1024, 5000)
	snap := r.Snapshot()
	if snap.PublishLatency.TotalCount() != 1 {
		t.Errorf("PublishLatency.TotalCount = %d, want 1 (no correction)", snap.PublishLatency.TotalCount())
	}
}
```

- [ ] **Step 2: Run new tests — expect compile errors**

```bash
cd /Users/josh/go-bench && go test ./internal/metrics/ -v -run 'TestRecorderCorrected|TestRecorderNoCorrection'
```

Expected: compile errors — `NewRecorder` called with wrong number of args, plus existing tests in metrics_test.go, producer_test.go, consumer_test.go will fail too.

- [ ] **Step 3: Update `internal/metrics/metrics.go`**

**3a.** Add `expectedIntervalMicros int64` as the first field of `Recorder` (after `startTime`):

```go
type Recorder struct {
	startTime              time.Time
	expectedIntervalMicros int64 // 0 means no correction (unlimited rate)

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
```

**3b.** Replace `NewRecorder()` with `NewRecorder(expectedIntervalMicros int64)`:

```go
// NewRecorder returns an initialised Recorder.
// expectedIntervalMicros is 1_000_000/rate µs for rate-limited runs; 0 disables
// coordinated omission correction (unlimited rate).
func NewRecorder(expectedIntervalMicros int64) *Recorder {
	return &Recorder{
		startTime:              time.Now(),
		expectedIntervalMicros: expectedIntervalMicros,
		periodPublish:          histogram.New(),
		periodE2E:              histogram.New(),
		cumPublish:             histogram.New(),
		cumE2E:                 histogram.New(),
	}
}
```

**3c.** Replace `RecordSend` with the corrected version:

```go
// RecordSend records a successful producer send.
// latencyMicros is the time from ProduceSync call to ack.
// When expectedIntervalMicros > 0, HDR corrected recording injects phantom
// samples for any interval skipped (coordinated omission correction).
func (r *Recorder) RecordSend(bytes int, latencyMicros int64) {
	r.mu.Lock()
	r.messagesSent++
	r.bytesSent += int64(bytes)
	if r.expectedIntervalMicros > 0 {
		r.periodPublish.RecordCorrectedValue(latencyMicros, r.expectedIntervalMicros)
	} else {
		r.periodPublish.RecordValue(latencyMicros)
	}
	r.totalSent.Add(1) // inside mu so Snapshot sees consistent period+cumulative counts
	r.mu.Unlock()

	r.cumMu.Lock()
	if r.expectedIntervalMicros > 0 {
		r.cumPublish.RecordCorrectedValue(latencyMicros, r.expectedIntervalMicros)
	} else {
		r.cumPublish.RecordValue(latencyMicros)
	}
	r.cumMu.Unlock()
}
```

**3d.** Replace `RecordReceive` with the corrected version:

```go
// RecordReceive records a successfully consumed message.
// e2eLatencyMicros is computed from the timestamp embedded in the message payload.
// Correction uses the same expected interval as RecordSend: during a producer stall
// the broker receives no new messages, so consumers are equally affected.
func (r *Recorder) RecordReceive(bytes int, e2eLatencyMicros int64) {
	r.mu.Lock()
	r.messagesReceived++
	r.bytesReceived += int64(bytes)
	if r.expectedIntervalMicros > 0 {
		r.periodE2E.RecordCorrectedValue(e2eLatencyMicros, r.expectedIntervalMicros)
	} else {
		r.periodE2E.RecordValue(e2eLatencyMicros)
	}
	r.totalReceived.Add(1) // inside mu so Snapshot sees consistent period+cumulative counts
	r.mu.Unlock()

	r.cumMu.Lock()
	if r.expectedIntervalMicros > 0 {
		r.cumE2E.RecordCorrectedValue(e2eLatencyMicros, r.expectedIntervalMicros)
	} else {
		r.cumE2E.RecordValue(e2eLatencyMicros)
	}
	r.cumMu.Unlock()
}
```

- [ ] **Step 4: Fix broken callers — update all `NewRecorder()` → `NewRecorder(0)`**

**`internal/metrics/metrics_test.go`** — 6 sites (lines 11, 26, 38, 48, 58, 67). Change each `metrics.NewRecorder()` to `metrics.NewRecorder(0)`.

**`internal/producer/producer_test.go`** — 1 site (line 30). Change `metrics.NewRecorder()` to `metrics.NewRecorder(0)`.

**`internal/consumer/consumer_test.go`** — 2 sites (lines 35, 56). Change each `metrics.NewRecorder()` to `metrics.NewRecorder(0)`.

- [ ] **Step 5: Run all metrics + producer + consumer tests**

```bash
cd /Users/josh/go-bench && go test ./internal/metrics/ ./internal/producer/ ./internal/consumer/ -v
```

Expected: all tests PASS including the 3 new ones.

- [ ] **Step 6: Commit**

```bash
cd /Users/josh/go-bench && git add internal/metrics/metrics.go internal/metrics/metrics_test.go \
  internal/producer/producer_test.go internal/consumer/consumer_test.go
git commit -m "feat: add coordinated omission correction to Recorder via NewRecorder(expectedIntervalMicros)"
```

---

## Task 3: Wire expected interval into bench orchestrator

**Files:**
- Modify: `internal/bench/bench.go`

- [ ] **Step 1: Verify current build is broken**

```bash
cd /Users/josh/go-bench && go build ./...
```

Expected: `too few arguments in call to metrics.NewRecorder` (2 sites in bench.go)

- [ ] **Step 2: Update `internal/bench/bench.go`**

**2a.** Before the first `metrics.NewRecorder()` call (currently line 38), add the interval computation and update the call:

Replace:
```go
rec := metrics.NewRecorder()
```

With:
```go
var expectedIntervalMicros int64
if cfg.ProduceRate > 0 {
    expectedIntervalMicros = 1_000_000 / int64(cfg.ProduceRate)
}
rec := metrics.NewRecorder(expectedIntervalMicros)
```

**2b.** Inside the warmup block (currently line 69), update the post-warmup reset:

Replace:
```go
rec = metrics.NewRecorder()
```

With:
```go
rec = metrics.NewRecorder(expectedIntervalMicros)
```

Note: `expectedIntervalMicros` is now in scope from step 2a — no re-declaration needed.

- [ ] **Step 3: Build cleanly**

```bash
cd /Users/josh/go-bench && go build ./...
```

Expected: no errors

- [ ] **Step 4: Run full test suite**

```bash
cd /Users/josh/go-bench && go test ./...
```

Expected: all packages PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/josh/go-bench && git add internal/bench/bench.go
git commit -m "feat: wire coordinated omission correction into bench — compute expectedIntervalMicros from ProduceRate"
```
