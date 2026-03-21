# Producer Tuning Flags Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--acks`, `--compression`, `--linger-ms`, and `--batch-max-bytes` CLI flags that wire into franz-go producer client options, letting go-bench reproduce OMB workload configurations.

**Architecture:** Four new `Config` fields with validation rules; a `producerOpts(cfg)` helper in `bench.go` translates them to `[]kgo.Opt` and appends them to the producer `kgo.NewClient()` call only. No new packages. Consumer client is unchanged.

**Tech Stack:** `github.com/twmb/franz-go/pkg/kgo` (RequiredAcks, ProducerBatchCompression, ProducerLinger, ProducerBatchMaxBytes), `github.com/spf13/cobra` / pflag for CLI flags, `math` stdlib for MaxInt32 guard.

---

## File Map

| File | Change |
|---|---|
| `internal/config/config.go` | Add 4 fields, update `Default()`, add 4 validation rules |
| `internal/config/config_test.go` | Add 4 new test functions |
| `cmd/bench/main.go` | Register 4 new flags |
| `internal/bench/bench.go` | Add `producerOpts` helper, wire into producer `kgo.NewClient()` |

---

## Task 1: Config fields and validation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

### Background

`Config` lives in `internal/config/config.go`. The struct has exported fields for every CLI parameter. `Default()` sets sensible values. `Validate()` returns the first error found.

Current last field is `OutputFile string`. New fields go after it. Existing test pattern: set one field to an invalid value on a valid `Default()` config, assert `Validate()` returns non-nil error.

`math.MaxInt32 = 2147483647` — import `"math"`.

- [ ] **Step 1: Write four failing tests**

Append to `internal/config/config_test.go` (add `"math"` to imports):

```go
func TestValidateAcks(t *testing.T) {
	cfg := config.Default()
	cfg.Acks = "2"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid acks value")
	}
	for _, valid := range []string{"0", "1", "all"} {
		cfg.Acks = valid
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected no error for acks=%q, got: %v", valid, err)
		}
	}
}

func TestValidateCompression(t *testing.T) {
	cfg := config.Default()
	cfg.Compression = "brotli"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid compression value")
	}
	for _, valid := range []string{"none", "gzip", "snappy", "lz4", "zstd"} {
		cfg.Compression = valid
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected no error for compression=%q, got: %v", valid, err)
		}
	}
}

func TestValidateLingerMs(t *testing.T) {
	cfg := config.Default()
	cfg.LingerMs = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative linger-ms")
	}
	cfg.LingerMs = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for linger-ms=0, got: %v", err)
	}
	cfg.LingerMs = 5
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for linger-ms=5, got: %v", err)
	}
}

func TestValidateBatchMaxBytes(t *testing.T) {
	cfg := config.Default()
	cfg.BatchMaxBytes = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative batch-max-bytes")
	}
	cfg.BatchMaxBytes = math.MaxInt32 + 1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for batch-max-bytes > MaxInt32")
	}
	cfg.BatchMaxBytes = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for batch-max-bytes=0, got: %v", err)
	}
	cfg.BatchMaxBytes = 131072
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for batch-max-bytes=131072, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests — expect compile errors (fields don't exist yet)**

```bash
cd /Users/josh/go-bench && go test ./internal/config/ -v -run 'TestValidateAcks|TestValidateCompression|TestValidateLingerMs|TestValidateBatchMaxBytes'
```

Expected: compile error — `cfg.Acks undefined` (or similar)

- [ ] **Step 3: Add fields, update Default(), add validation rules**

In `internal/config/config.go`:

**Add `"math"` to imports.**

**Add four fields after `OutputFile string`:**

```go
Acks          string // "0", "1", "all"
Compression   string // "none", "gzip", "snappy", "lz4", "zstd"
LingerMs      int    // milliseconds; 0 = franz-go default (send immediately)
BatchMaxBytes int    // bytes; 0 = franz-go default (~1MB)
```

**Update `Default()` — add inside the return struct literal:**

```go
Acks:         "all",
Compression:  "none",
LingerMs:     0,
BatchMaxBytes: 0,
```

**Add four rules at the end of `Validate()` (before the final `return nil`):**

```go
validAcks := map[string]bool{"0": true, "1": true, "all": true}
if !validAcks[c.Acks] {
    return errors.New("acks must be 0, 1, or all")
}
validCompression := map[string]bool{"none": true, "gzip": true, "snappy": true, "lz4": true, "zstd": true}
if !validCompression[c.Compression] {
    return errors.New("compression must be none, gzip, snappy, lz4, or zstd")
}
if c.LingerMs < 0 {
    return errors.New("linger-ms must be >= 0")
}
if c.BatchMaxBytes < 0 || c.BatchMaxBytes > math.MaxInt32 {
    return errors.New("batch-max-bytes must be between 0 and 2147483647")
}
```

- [ ] **Step 4: Run tests — expect 4 PASS**

```bash
cd /Users/josh/go-bench && go test ./internal/config/ -v
```

Expected: all tests PASS (12 total — 8 existing + 4 new)

- [ ] **Step 5: Commit**

```bash
cd /Users/josh/go-bench && git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add Acks, Compression, LingerMs, BatchMaxBytes fields with validation"
```

---

## Task 2: CLI flags and producerOpts wiring

**Files:**
- Modify: `cmd/bench/main.go`
- Modify: `internal/bench/bench.go`

### Background

`cmd/bench/main.go` registers flags using pflag via `cmd.Flags()`. Flags call `f.StringVar` / `f.IntVar` pointing at cfg fields. Flag ordering in help output is alphabetical by pflag — no ordering constraint.

`internal/bench/bench.go` constructs two `kgo.Client` instances. The producer client is created at roughly line 55:

```go
producerClient, err := kgo.NewClient(
    kgo.SeedBrokers(cfg.Brokers...),
    kgo.DefaultProduceTopic(cfg.Topic),
)
```

The new `producerOpts(cfg)` helper returns a `[]kgo.Opt` slice. Use `append` to merge it:

```go
producerClient, err := kgo.NewClient(
    append([]kgo.Opt{
        kgo.SeedBrokers(cfg.Brokers...),
        kgo.DefaultProduceTopic(cfg.Topic),
    }, producerOpts(cfg)...)...,
)
```

### No new tests needed for this task

`Validate()` already ensures all flag values are in the valid set before `Run` is called. The existing `TestIntegrationSmoke` (build tag `integration`) exercises the full `bench.Run` path including client construction, which will compile and execute `producerOpts`. There is no unit-testable behaviour in `producerOpts` beyond what `Validate()` already covers.

- [ ] **Step 1: Add four flags to `cmd/bench/main.go`**

After the `--output` flag line, add:

```go
f.StringVar(&cfg.Acks, "acks", cfg.Acks, `producer acks: "0" (none), "1" (leader), "all" (all ISR)`)
f.StringVar(&cfg.Compression, "compression", cfg.Compression, `compression codec: none, gzip, snappy, lz4, zstd`)
f.IntVar(&cfg.LingerMs, "linger-ms", cfg.LingerMs, "producer linger in milliseconds (0 = send immediately)")
f.IntVar(&cfg.BatchMaxBytes, "batch-max-bytes", cfg.BatchMaxBytes, "max producer batch size in bytes (0 = franz-go default ~1MB)")
```

- [ ] **Step 2: Add `producerOpts` helper to `internal/bench/bench.go`**

Add `"time"` is already imported. Ensure `"math"` is NOT needed here (validation already happened). Add the helper function anywhere in the file (conventional place: after `runWorkers`):

```go
// producerOpts translates producer tuning config into franz-go client options.
// The acks option is always set explicitly — even for "all" (the default) — to
// make the producer's durability contract visible in code rather than relying on
// the franz-go default. The int options are omitted when zero so franz-go's own
// defaults apply, consistent with their "0 = use default" semantics.
// BatchMaxBytes is safe to cast to int32: Validate() caps it at math.MaxInt32.
func producerOpts(cfg *config.Config) []kgo.Opt {
	var opts []kgo.Opt

	switch cfg.Acks {
	case "0":
		opts = append(opts, kgo.RequiredAcks(kgo.NoAck()))
	case "1":
		opts = append(opts, kgo.RequiredAcks(kgo.LeaderAck()))
	case "all":
		opts = append(opts, kgo.RequiredAcks(kgo.AllISRAcks()))
	}

	switch cfg.Compression {
	case "gzip":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.GzipCompression()))
	case "snappy":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.SnappyCompression()))
	case "lz4":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.Lz4Compression()))
	case "zstd":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.ZstdCompression()))
	// "none": omit — franz-go default is no compression
	}

	if cfg.LingerMs > 0 {
		opts = append(opts, kgo.ProducerLinger(time.Duration(cfg.LingerMs)*time.Millisecond))
	}
	if cfg.BatchMaxBytes > 0 {
		opts = append(opts, kgo.ProducerBatchMaxBytes(int32(cfg.BatchMaxBytes)))
	}

	return opts
}
```

- [ ] **Step 3: Wire `producerOpts` into the producer `kgo.NewClient()` call**

Find the producer client construction (around line 55 in bench.go):

```go
producerClient, err := kgo.NewClient(
    kgo.SeedBrokers(cfg.Brokers...),
    kgo.DefaultProduceTopic(cfg.Topic),
)
```

Replace with:

```go
producerClient, err := kgo.NewClient(
    append([]kgo.Opt{
        kgo.SeedBrokers(cfg.Brokers...),
        kgo.DefaultProduceTopic(cfg.Topic),
    }, producerOpts(cfg)...)...,
)
```

The consumer client (`consumerClient, err := kgo.NewClient(...)`) is NOT changed.

- [ ] **Step 4: Build and run full test suite**

```bash
cd /Users/josh/go-bench && go build ./... && go test ./...
```

Expected: clean build, all packages PASS

- [ ] **Step 5: Build binary and verify help output shows new flags**

```bash
cd /Users/josh/go-bench && go build -o bench ./cmd/bench && ./bench run --help
```

Expected: `--acks`, `--compression`, `--linger-ms`, `--batch-max-bytes` all appear in the flag list

- [ ] **Step 6: Commit**

```bash
cd /Users/josh/go-bench && git add cmd/bench/main.go internal/bench/bench.go
git commit -m "feat: add --acks, --compression, --linger-ms, --batch-max-bytes producer tuning flags"
```
