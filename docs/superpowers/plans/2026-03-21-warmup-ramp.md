# Warmup Rate Ramp Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Linearly ramp the produce rate from 0 to the target during warmup, matching OMB behaviour.

**Architecture:** Add `SetRate`, `NewPool`, and `StartPool` to the producer package, then update the warmup block in bench.go to create workers at rate 1 and drive a ramp goroutine that calls `SetRate` every second. Both the ramp goroutine and consumer goroutine are tracked in a `sync.WaitGroup` to guarantee all writes to `rec` stop before it is replaced.

**Tech Stack:** `golang.org/x/time/rate` (already a dependency) — `rate.Limiter.SetLimit` and `SetBurst` are goroutine-safe and used for live rate updates.

---

### Task 1: Add `SetRate`, `NewPool`, `StartPool` to producer package

**Files:**
- Modify: `internal/producer/producer.go`
- Modify: `internal/producer/producer_test.go`

The existing `RunPool` function creates workers internally. This task adds:
- `SetRate(r int)` — updates the limiter in-place without reassigning the pointer (goroutine-safe)
- `NewPool(sender, rec, n, topic, messageSize, produceRate) []*Worker` — creates n workers without starting them
- `StartPool(ctx, workers)` — runs all workers and blocks until done
- Refactor `RunPool` to call `NewPool` + `StartPool` (no signature change)

**Key constraint:** `SetRate` must never reassign `w.limiter` — only call `SetLimit`/`SetBurst` on the existing pointer. This is critical to avoid a data race with the `Run` goroutine reading the pointer.

- [ ] **Step 1: Write failing tests for SetRate**

Add to `internal/producer/producer_test.go`:

```go
func TestSetRate_updatesExistingLimiter(t *testing.T) {
	rec := metrics.NewRecorder(0)
	w := producer.NewWorker(producer.NoopSender{}, rec, "t", 64, 100)
	w.SetRate(200)
	if w.Limiter().Limit() != rate.Limit(200) {
		t.Errorf("Limit() = %v, want 200", w.Limiter().Limit())
	}
	if w.Limiter().Burst() != 200 {
		t.Errorf("Burst() = %v, want 200", w.Limiter().Burst())
	}
}

func TestSetRate_zeroSetsUnlimited(t *testing.T) {
	rec := metrics.NewRecorder(0)
	w := producer.NewWorker(producer.NoopSender{}, rec, "t", 64, 100)
	w.SetRate(0)
	if w.Limiter().Limit() != rate.Inf {
		t.Errorf("Limit() = %v, want rate.Inf", w.Limiter().Limit())
	}
}

func TestSetRate_nilLimiterIsNoop(t *testing.T) {
	rec := metrics.NewRecorder(0)
	// produceRate=0 → limiter is nil
	w := producer.NewWorker(producer.NoopSender{}, rec, "t", 64, 0)
	// Must not panic
	w.SetRate(100)
	if w.Limiter() != nil {
		t.Error("expected limiter to remain nil")
	}
}

func TestSetRate_concurrentWithRun(t *testing.T) {
	// Run this test with: go test -race ./internal/producer/...
	rec := metrics.NewRecorder(0)
	w := producer.NewWorker(producer.NoopSender{}, rec, "t", 64, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go w.Run(ctx)
	for i := 0; i < 100; i++ {
		w.SetRate(1000)
	}
	<-ctx.Done()
}
```

Note: these tests call `w.Limiter()` — a new exported accessor that returns the `*rate.Limiter`. Add it alongside `SetRate`.

Add the required import `"golang.org/x/time/rate"` to the test file.

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/josh/go-mb
go test ./internal/producer/... -run "TestSetRate" -v
```

Expected: compile error — `SetRate` and `Limiter` not defined.

- [ ] **Step 3: Implement SetRate, Limiter, NewPool, StartPool; refactor RunPool**

In `internal/producer/producer.go`, add after the existing `NewWorker` function:

```go
// Limiter returns the worker's rate limiter. May be nil if no rate was set.
// Exposed for testing only.
func (w *Worker) Limiter() *rate.Limiter {
	return w.limiter
}

// SetRate updates the worker's rate limit dynamically.
// Safe to call concurrently with Run — uses SetLimit/SetBurst which are
// documented as goroutine-safe on rate.Limiter.
// Precondition: w.limiter must be non-nil (worker created with produceRate > 0).
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
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Run(ctx)
		}()
	}
	wg.Wait()
}
```

Add `"sync"` to the import block (it may already be present).

Replace the body of the existing `RunPool` function:

```go
// RunPool starts n Worker goroutines sharing a single kgo.Client and Recorder.
// It blocks until all workers have stopped.
func RunPool(ctx context.Context, client *kgo.Client, rec *metrics.Recorder, n int, topic string, messageSize int, produceRate int) {
	StartPool(ctx, NewPool(client, rec, n, topic, messageSize, produceRate))
}
```

- [ ] **Step 4: Write failing tests for NewPool and StartPool**

Add to `internal/producer/producer_test.go`:

```go
func TestNewPool_createsNWorkers(t *testing.T) {
	rec := metrics.NewRecorder(0)
	workers := producer.NewPool(producer.NoopSender{}, rec, 3, "t", 64, 100)
	if len(workers) != 3 {
		t.Fatalf("len(workers) = %d, want 3", len(workers))
	}
	for i, w := range workers {
		if w == nil {
			t.Errorf("workers[%d] is nil", i)
		}
		if w.Limiter() == nil {
			t.Errorf("workers[%d].Limiter() is nil, want non-nil for produceRate=100", i)
		}
	}
}

func TestStartPool_runsAllWorkers(t *testing.T) {
	rec := metrics.NewRecorder(0)
	workers := producer.NewPool(producer.NoopSender{}, rec, 4, "t", 64, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// StartPool must return once context is cancelled
	done := make(chan struct{})
	go func() {
		producer.StartPool(ctx, workers)
		close(done)
	}()
	select {
	case <-done:
		// good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StartPool did not return after context cancel")
	}
}
```

- [ ] **Step 5: Run all producer tests**

```bash
cd /Users/josh/go-mb
go test -race ./internal/producer/... -v
```

Expected: all tests PASS including `TestSetRate_concurrentWithRun` under the race detector.

- [ ] **Step 6: Verify build**

```bash
cd /Users/josh/go-mb
go build ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
cd /Users/josh/go-mb
git add internal/producer/producer.go internal/producer/producer_test.go
git commit -m "feat(producer): add SetRate, NewPool, StartPool for ramp support"
```

---

### Task 2: Ramp warmup in bench.go

**Files:**
- Modify: `internal/bench/bench.go`

Replace the warmup block with a ramp-aware version when `cfg.ProduceRate > 0`. The non-ramp path (`else` branch) is unchanged.

- [ ] **Step 1: Write the updated warmup block**

In `internal/bench/bench.go`, replace the existing warmup block (the `if cfg.WarmupDuration > 0` section, currently lines ~97–104) with:

```go
// 3. Warmup (optional).
if cfg.WarmupDuration > 0 {
	fmt.Printf("----- Warming up for %s -----\n", cfg.WarmupDuration)
	wCtx, wCancel := context.WithTimeout(ctx, cfg.WarmupDuration)
	defer wCancel() // belt-and-suspenders: guards against future early returns

	if cfg.ProduceRate > 0 {
		// Linear ramp: create workers at rate 1, ramp to target over warmup duration.
		workers := producer.NewPool(producerClient, rec, cfg.Producers, cfg.Topic, cfg.MessageSize, 1)

		totalSteps := int(cfg.WarmupDuration.Seconds())
		if totalSteps < 1 {
			totalSteps = 1
		}

		var wg sync.WaitGroup

		// Ramp goroutine: updates all worker rates every second.
		// Tracked in wg to guarantee it exits before rec is replaced.
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			step := 0
			for {
				select {
				case <-ticker.C:
					step++
					currentRate := cfg.ProduceRate * step / totalSteps
					if currentRate < 1 {
						currentRate = 1
					}
					for _, w := range workers {
						w.SetRate(currentRate)
					}
					if step >= totalSteps {
						return
					}
				case <-wCtx.Done():
					return
				}
			}
		}()

		// Consumer goroutine — no ramp needed; tracked in wg.
		if cfg.Consumers > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				consumer.RunPool(wCtx, consumerClient, rec, cfg.Consumers)
			}()
		}

		producer.StartPool(wCtx, workers)
		wg.Wait()
	} else {
		// No rate limit — run workers at full speed during warmup.
		runWorkers(wCtx, cfg, producerClient, consumerClient, rec)
	}

	wCancel() // eager release of timer resource
	// Reset metrics after warmup so benchmark measurements are clean.
	rec = metrics.NewRecorder(expectedIntervalMicros)
}
```

Ensure `"sync"` is already in the import block (it is, via the existing `sync.WaitGroup` in the benchmark section).

- [ ] **Step 2: Verify build**

```bash
cd /Users/josh/go-mb
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run unit tests**

```bash
cd /Users/josh/go-mb
go test -race ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
cd /Users/josh/go-mb
git add internal/bench/bench.go
git commit -m "feat(bench): linearly ramp produce rate during warmup"
```

---

### Task 3: Integration test for warmup ramp

**Files:**
- Modify: `internal/bench/bench_test.go`

Add a test that exercises the ramp code path (rate > 0, warmup > 0). This is distinct from the existing integration test which uses unlimited rate.

- [ ] **Step 1: Write the integration test**

In `internal/bench/bench_test.go`, add after the existing integration test:

```go
func TestIntegration_WarmupRamp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	brokers := os.Getenv("BENCH_BROKERS")
	if brokers == "" {
		t.Skip("BENCH_BROKERS not set — skipping integration test")
	}

	cfg := config.Default()
	cfg.Brokers = strings.Split(brokers, ",")
	cfg.Topic = "integration-ramp-test"
	cfg.Producers = 1
	cfg.Consumers = 1
	cfg.MessageSize = 256
	cfg.ProduceRate = 500        // rate > 0 triggers ramp
	cfg.WarmupDuration = 5 * time.Second
	cfg.Duration = 10 * time.Second
	cfg.ReportInterval = 5 * time.Second
	cfg.CreateTopic = true
	cfg.DeleteTopic = true

	err := bench.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("bench.Run: %v", err)
	}
}
```

Check the existing integration test for the correct build tag and file header — match the same `//go:build integration` tag and package declaration.

- [ ] **Step 2: Run the integration test (requires broker)**

```bash
cd /Users/josh/go-mb
BENCH_BROKERS=localhost:9092 go test ./internal/bench/ -tags integration -v \
  -run TestIntegration_WarmupRamp -timeout 60s
```

Expected: PASS. The run should complete without error; the warmup ramp message `----- Warming up for 5s -----` should appear in output.

- [ ] **Step 3: Commit**

```bash
cd /Users/josh/go-mb
git add internal/bench/bench_test.go
git commit -m "test(bench): add integration test for warmup rate ramp"
```
