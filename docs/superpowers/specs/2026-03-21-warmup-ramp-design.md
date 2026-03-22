# Warmup Rate Ramp Design

**Date:** 2026-03-21
**Status:** Approved

## Summary

During the warmup phase, linearly ramp the produce rate from 0 to the target rate over the warmup duration, updating every 1 second. This matches OMB's warmup behaviour and prevents cold-start bursts from skewing broker state before the benchmark proper begins.

## Behaviour

- Only activates when both `--rate > 0` and `--warmup > 0`
- If `--rate 0` (unlimited) or no warmup, behaviour is unchanged
- Rate is updated every 1 second (hardcoded, no flag)
- At second `i` of `N` warmup seconds: `currentRate = targetRate * (i+1) / N`
- `N = int(warmupDuration.Seconds())`, minimum 1
- Rate starts at `targetRate / N` (first step, integer division) and reaches `targetRate` at the final step
- Warmup metrics are still discarded after warmup completes (no change)
- No new CLI flags required

**Low-rate edge case:** when `targetRate < N`, integer division means the first `N/targetRate` steps produce 0 — clamped to 1 msg/s. For example, `--rate 5 --warmup 60s` stays at 1 msg/s for the first ~12 seconds before the ramp begins differentiating. This is acceptable; operators should not use ramp with very low rates.

**Sub-second warmup edge case:** if `--warmup` is less than 1 second, `totalSteps = 1`. The first (and only) ticker tick sets the rate to the full target. The ticker and the context timeout may fire near-simultaneously — both code paths lead to the same end state (workers at full rate, context expired), so this is harmless.

## Code Changes

### 1. `internal/producer/producer.go`

**Add `SetRate(r int)` to `Worker`:**

`rate.Limiter.SetLimit()` and `SetBurst()` are documented as safe for concurrent use, so calling them from the ramp goroutine while `Worker.Run` holds the limiter is safe. To avoid a data race on the `w.limiter` pointer field itself, `SetRate` never reassigns the pointer — instead it uses `rate.Inf` to express "unlimited":

```go
// SetRate updates the worker's rate limit dynamically.
// Safe to call concurrently with Run — uses SetLimit/SetBurst which are
// documented as goroutine-safe on rate.Limiter.
// Precondition: w.limiter must be non-nil (worker created with produceRate > 0).
// If r <= 0, sets the limiter to unlimited (rate.Inf) rather than nil,
// avoiding a pointer write that would race with Run.
func (w *Worker) SetRate(r int) {
    if w.limiter == nil {
        // Should not happen when used via the ramp path (NewPool creates workers
        // with produceRate=1). Silently ignore to avoid a nil dereference.
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
```

**Add `NewPool` and `StartPool`:**

`NewPool` takes a `Sender` interface (matching `NewWorker`) to preserve the test-double pattern:

```go
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

**Update `RunPool` to use `NewPool` + `StartPool`** (no breaking change to signature):

```go
func RunPool(ctx context.Context, client *kgo.Client, rec *metrics.Recorder, n int, topic string, messageSize int, produceRate int) {
    StartPool(ctx, NewPool(client, rec, n, topic, messageSize, produceRate))
}
```

### 2. `internal/bench/bench.go`

**Update warmup block** to ramp rate when `cfg.ProduceRate > 0`.

The consumer goroutine and ramp goroutine are both tracked in a `sync.WaitGroup` to guarantee all goroutines have exited before `rec` is replaced. This matches the guarantee `runWorkers` provides in the non-ramp path.

`wCancel` is called eagerly at the end of the warmup block to release the timer resource promptly (the context has expired naturally by this point, but eager cancellation is still best practice). A `defer wCancel()` is also used as a belt-and-suspenders guard for any future early-return paths added to this block.

```go
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

        // Ramp goroutine: updates worker rates every second.
        // Tracked in wg so we guarantee it has exited before rec is replaced.
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
    rec = metrics.NewRecorder(expectedIntervalMicros)
}
```

### 3. No changes to `config.go` or `cmd/go-mb/main.go`

No new flags. Warmup ramp is automatic when `--rate > 0` and `--warmup > 0`.

## Tests

### `internal/producer/producer_test.go`

1. **TestSetRate_updatesExistingLimiter** — call `SetRate(200)` on a worker created with rate 100; verify `w.limiter.Limit()` equals 200 and burst equals 200
2. **TestSetRate_zeroSetsUnlimited** — call `SetRate(0)` on a rate-limited worker; verify `w.limiter.Limit()` equals `rate.Inf`
3. **TestSetRate_nilLimiterIsNoop** — call `SetRate(100)` on a worker created with rate 0 (nil limiter); verify no panic and limiter remains nil
4. **TestSetRate_concurrentWithRun** — create a worker with rate 1, start `Run` in a goroutine, call `SetRate(1000)` concurrently 100 times; run with `-race` to validate no data race
5. **TestNewPool_createsNWorkers** — verify `NewPool` returns slice of length n with non-nil workers, each with correct topic and limiter state
6. **TestStartPool_runsAllWorkers** — verify all workers run and stop when context is cancelled; check stop count equals n

### `internal/bench/bench_test.go` (integration)

**TestIntegration_WarmupRamp** (build tag: `integration`) — run a full benchmark with `--rate 500 --warmup 5s --duration 10s`; verify:
- Run completes without error
- At least some messages were produced during warmup (metrics reset, so benchmark metrics are from the flat phase)
- Benchmark phase produces messages at approximately 500 msg/s

This is distinct from the existing integration test which exercises the no-rate/no-ramp path.

## Invariants

- `SetRate` never reassigns `w.limiter` — the pointer is written once in `NewWorker` and never again, eliminating the data race
- Workers in the ramp path are always created with `produceRate = 1` so `w.limiter` is always non-nil when `SetRate` is called
- Both the ramp goroutine and consumer goroutine are tracked in `wg`; `wg.Wait()` completes before `rec` is replaced — matching the guarantee provided by `runWorkers`
- `RunPool` signature is unchanged — no breaking change to callers
- `wCancel` is both deferred (guard against early returns) and called eagerly at the end of the warmup block (prompt timer release)
- If warmup duration < 1 second, `totalSteps = 1` and workers step to the full target rate on the first tick
