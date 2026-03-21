# Producer Tuning Flags Design

## Goal

Expose four producer-side Kafka tuning parameters as CLI flags — `--acks`, `--compression`, `--linger-ms`, and `--batch-max-bytes` — so go-bench can match the producer configuration expressible in OMB's driver YAML files and benchmark against real Redpanda deployments with non-default durability and throughput settings.

## Background

OMB's Kafka driver config exposes `producerConfig` as raw Java Kafka client properties:

```yaml
producerConfig: |
  acks=all
  linger.ms=1
  batch.size=131072
  compression.type=lz4
```

go-bench currently hard-codes the franz-go defaults for all of these, making it impossible to reproduce OMB workloads that tune these parameters.

## Flags

| Flag | Default | Valid values |
|---|---|---|
| `--acks` | `all` | `0`, `1`, `all` |
| `--compression` | `none` | `none`, `gzip`, `snappy`, `lz4`, `zstd` |
| `--linger-ms` | `0` | `>= 0` |
| `--batch-max-bytes` | `0` | `0` to `2147483647` (0 = use franz-go default ~1MB) |

`--acks all` mirrors OMB's recommended Redpanda default. `--compression none` preserves current behaviour. `0` for both int flags means "don't override the franz-go default".

## Architecture

Four files change (three source files + config test file); no new packages.

### `internal/config/config.go`

Add four fields to `Config` after `DeleteTopic`:

```go
Acks          string // "0", "1", "all"
Compression   string // "none", "gzip", "snappy", "lz4", "zstd"
LingerMs      int    // milliseconds; 0 = franz-go default (send immediately)
BatchMaxBytes int    // bytes; 0 = franz-go default (~1MB)
```

`Default()` returns `Acks: "all"`, `Compression: "none"`, `LingerMs: 0`, `BatchMaxBytes: 0`.

`Validate()` additions:
- `Acks` must be `"0"`, `"1"`, or `"all"` (error: `"acks must be 0, 1, or all"`)
- `Compression` must be one of `"none"`, `"gzip"`, `"snappy"`, `"lz4"`, `"zstd"` (error: `"compression must be none, gzip, snappy, lz4, or zstd"`)
- `LingerMs >= 0` (error: `"linger-ms must be >= 0"`)
- `BatchMaxBytes >= 0 && BatchMaxBytes <= math.MaxInt32` (error: `"batch-max-bytes must be between 0 and 2147483647"`)

### `internal/config/config_test.go`

New unit tests (TDD — write failing tests before implementation):
- `TestValidateAcks`: invalid value `"2"` returns error; valid values `"0"`, `"1"`, `"all"` pass
- `TestValidateCompression`: invalid value `"brotli"` returns error; all 5 valid values pass
- `TestValidateLingerMs`: `-1` returns error; `0` and positive pass
- `TestValidateBatchMaxBytes`: `-1` returns error; `math.MaxInt32+1` returns error; `0` and `131072` pass

### `cmd/bench/main.go`

Four new flags registered in `newRunCommand()` alongside the existing flags (pflag does not guarantee help-output ordering; no ordering constraint applies):

```go
f.StringVar(&cfg.Acks, "acks", cfg.Acks, `producer acks: "0" (none), "1" (leader), "all" (all ISR)`)
f.StringVar(&cfg.Compression, "compression", cfg.Compression, `compression: none, gzip, snappy, lz4, zstd`)
f.IntVar(&cfg.LingerMs, "linger-ms", cfg.LingerMs, "producer linger in milliseconds (0 = send immediately)")
f.IntVar(&cfg.BatchMaxBytes, "batch-max-bytes", cfg.BatchMaxBytes, "max producer batch size in bytes (0 = franz-go default ~1MB)")
```

### `internal/bench/bench.go`

New unexported helper function:

```go
// producerOpts translates producer tuning config into franz-go client options.
// The acks option is always set explicitly — even for "all" (the default) — to
// make the producer's durability contract visible in code rather than relying on
// the franz-go default. The int options are omitted when zero so franz-go's own
// defaults apply, consistent with their "0 = use default" semantics.
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

`producerOpts(cfg)` is appended to the producer `kgo.NewClient()` call only. The consumer client is unchanged.

The `int32` cast in `kgo.ProducerBatchMaxBytes(int32(cfg.BatchMaxBytes))` is safe because `Validate()` caps `BatchMaxBytes` at `math.MaxInt32`.

## HTML Report

The four new fields (`Acks`, `Compression`, `LingerMs`, `BatchMaxBytes`) are intentionally excluded from `results.RunMeta` and the HTML report header. The report already captures the workload parameters most relevant to result interpretation (producers, consumers, message size, duration). Producer tuning flags are operational configuration; they can be recovered from the CLI invocation logged to stdout. `results.go` does not change.

## Out of Scope

- Consumer-side tuning flags (fetch size, fetch wait)
- TLS / SASL (covered in the next spec)
