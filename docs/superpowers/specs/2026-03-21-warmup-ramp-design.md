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
- Rate starts at `targetRate / N` (first step) and reaches `targetRate` at the final step
- Warmup metrics are still discarded after warmup completes (no change)
- No new CLI flags required

## Example

`--rate 10000 --warmup 30s`:
- Second 1: ~333 msg/s
- Second 10: ~3333 msg/s
- Second 20: ~6667 msg/s
- Second 30: 10000 msg/s
- Benchmark begins at full rate

## Code Changes

### 1. `internal/producer/producer.go`

**Add `SetRate(r int)` to `Worker`:**

```go
// SetRate updates the worker's rate limit. If the limiter is nil and r > 0,
// a new limiter is created. If r <= 0, the limiter is set to unlimited.
func (w *Worker) SetRate(r int) {
    if r <= 0 {
        w.limiter = nil
        return
    }
    if w.limiter == nil {
        w.limiter = rate.NewLimiter(rate.Limit(r), r)
        return
    }
    w.limiter.SetLimit(rate.Limit(r))
    w.limiter.SetBurst(r)
}
```

**Add `NewPool` and `StartPool`:**

```go
// NewPool creates n Workers without starting them.
func NewPool(client *kgo.Client, rec *metrics.Recorder, n int, topic string, messageSize int, produceRate int) []*Worker {
    workers := make([]*Worker, n)
    for i := range workers {
        workers[i] = NewWorker(client, rec, topic, messageSize, produceRate)
    }
    return workers
}

// StartPool runs all workers concurrently and blocks until all have stopped.
func StartPool(ctx context.Context, workers []*Worker) {
    done := make(chan struct{}, len(workers))
    for _, w := range workers {
        w := w
        go func() {
            w.Run(ctx)
            done <- struct{}{}
        }()
    }
    for range workers {
        <-done
    }
}
```

**Update `RunPool` to use `NewPool` + `StartPool`** (no breaking change to signature):

```go
func RunPool(ctx context.Context, client *kgo.Client, rec *metrics.Recorder, n int, topic string, messageSize int, produceRate int) {
    StartPool(ctx, NewPool(client, rec, n, topic, messageSize, produceRate))
}
```

### 2. `internal/bench/bench.go`

**Update warmup block** to ramp rate when `cfg.ProduceRate > 0`:

```go
if cfg.WarmupDuration > 0 {
    fmt.Printf("----- Warming up for %s -----\n", cfg.WarmupDuration)
    wCtx, wCancel := context.WithTimeout(ctx, cfg.WarmupDuration)

    if cfg.ProduceRate > 0 {
        // Linear ramp: create workers at rate 1, ramp to target over warmup duration.
        workers := producer.NewPool(producerClient, rec, cfg.Producers, cfg.Topic, cfg.MessageSize, 1)
        totalSteps := int(cfg.WarmupDuration.Seconds())
        if totalSteps < 1 {
            totalSteps = 1
        }
        go func() {
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

        // Start consumers normally (no ramp needed).
        if cfg.Consumers > 0 {
            go consumer.RunPool(wCtx, consumerClient, rec, cfg.Consumers)
        }
        producer.StartPool(wCtx, workers)
    } else {
        // No rate limit — run workers at full speed during warmup.
        runWorkers(wCtx, cfg, producerClient, consumerClient, rec)
    }

    wCancel()
    rec = metrics.NewRecorder(expectedIntervalMicros)
}
```

### 3. No changes to `config.go` or `cmd/go-mb/main.go`

No new flags. Warmup ramp is automatic when `--rate > 0` and `--warmup > 0`.

## Tests

### `internal/producer/producer_test.go`

1. **TestSetRate_updatesLimiter** — call `SetRate` on a worker created with rate 0; verify limiter is created and `Wait` no longer blocks
2. **TestSetRate_updatesExistingLimiter** — call `SetRate(100)` on a worker created with rate 50; verify `limiter.Limit()` returns 100
3. **TestSetRate_zeroDisablesLimiter** — call `SetRate(0)` on a rate-limited worker; verify limiter is nil
4. **TestNewPool_createsNWorkers** — verify `NewPool` returns slice of length n with non-nil workers
5. **TestStartPool_runsAllWorkers** — verify all workers run and stop when context is cancelled

### `internal/bench/bench_test.go` (integration)

No new integration tests — existing integration test covers full run including warmup path.

## Invariants

- Warmup metrics are always discarded regardless of ramp (reset after warmup block)
- Rate ramp only affects producers; consumers start immediately at full speed
- `RunPool` signature is unchanged — no breaking change to callers
- If warmup duration < 1 second, `totalSteps = 1` and workers start at the target rate immediately
