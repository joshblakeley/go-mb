# go-mb

A lightweight Kafka/Redpanda benchmarking tool written in Go. Measures publish latency, end-to-end latency, and throughput under controlled load — with HDR histogram accuracy and coordinated omission correction.

## Features

- Concurrent producers and consumers with configurable rate limiting
- Linear warmup rate ramp (0 → target over warmup duration, matching OMB behaviour)
- Coordinated omission correction (accurate latency under backpressure)
- Interactive HTML reports with time-series charts
- TLS (system CAs or custom CA cert) and SASL (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)
- Producer tuning: acks, compression, linger, batch size
- Automatic topic create/delete

## Quick Start

```bash
# Start a local Redpanda broker
make broker-up

# Run a benchmark
go run ./cmd/go-mb run

# With HTML report
go run ./cmd/go-mb run --duration 60s --output report.html
```

## Installation

```bash
go install github.com/joshblakeley/go-mb/cmd/go-mb@latest
```

Or build from source:

```bash
git clone https://github.com/joshblakeley/go-mb
cd go-mb
make build
./go-mb run
```

## Usage

```
go-mb run [flags]
```

### Connection

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--brokers` | `-b` | `localhost:9092` | Comma-separated broker addresses |

### Topic

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--topic` | `-t` | `benchmark` | Topic name |
| `--partitions` | `-p` | `1` | Number of partitions |
| `--replication-factor` | `-r` | `3` | Replication factor |
| `--create-topic` | | `true` | Create topic before benchmark |
| `--delete-topic` | | `true` | Delete topic after benchmark |

### Workload

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--producers` | `-P` | `1` | Concurrent producer goroutines |
| `--consumers` | `-C` | `1` | Concurrent consumer goroutines |
| `--message-size` | `-s` | `1024` | Message size in bytes (min 8) |
| `--rate` | `-R` | `0` | Target produce rate msg/s per producer (0 = unlimited) |
| `--consumer-group` | | `benchmark-group` | Consumer group ID |

### Duration

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--duration` | `-d` | `1m` | Benchmark duration |
| `--warmup` | `-w` | `0` | Warmup period (excluded from results); linearly ramps rate 0→target when `--rate` is set |
| `--report-interval` | | `10s` | Periodic stats interval |

### Producer Tuning

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--acks` | | `all` | Acks: `0` (none), `1` (leader), `all` (all ISR) |
| `--compression` | | `none` | Codec: `none`, `gzip`, `snappy`, `lz4`, `zstd` |
| `--linger-ms` | | `0` | Linger in ms before flushing batch (0 = immediate) |
| `--batch-max-bytes` | | `0` | Max batch size in bytes (0 = franz-go default ~1MB) |

### Security

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--tls` | | `false` | Enable TLS using system root CAs |
| `--tls-ca-cert` | | `` | Path to CA certificate PEM (implies `--tls`) |
| `--sasl-mechanism` | | `` | `plain`, `scram-sha-256`, or `scram-sha-512` |
| `--sasl-username` | | `` | SASL username |
| `--sasl-password` | | `` | SASL password |

### Output

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `` | Path to write HTML report (e.g. `report.html`) |

## Examples

```bash
# High-throughput: 4 producers, no rate limit, zstd compression
go-mb run --producers 4 --consumers 4 --partitions 4 \
  --compression zstd --acks 1 --duration 2m

# Latency-focused: rate-limited with warmup ramp, HTML report
go-mb run --rate 1000 --acks all --warmup 30s --duration 60s --output report.html

# Against a TLS+SASL cluster
go-mb run --brokers broker:9092 \
  --tls --tls-ca-cert /path/to/ca.pem \
  --sasl-mechanism scram-sha-256 \
  --sasl-username user --sasl-password secret
```

## HTML Report

Pass `--output report.html` to generate a self-contained HTML report with:

- Publish and consume rate over time (msg/s)
- Throughput over time (MB/s)
- Publish latency percentiles over time
- End-to-end latency percentiles over time
- Final aggregated latency summary (p50/p99/p99.9/max)

## Coordinated Omission Correction

When `--rate` is set, go-mb uses HDR Histogram's `RecordCorrectedValue` to account for coordinated omission — the systematic underreporting of latency that occurs when slow responses cause a benchmark to skip scheduled sends. This matches the behaviour of [OpenMessaging Benchmark](https://openmessaging.cloud/docs/benchmarks/).

## Development

```bash
make test              # Unit tests
make broker-up         # Start Redpanda via Docker Compose
make test-integration  # Integration tests (broker required)
make e2e               # Full end-to-end: build → broker → test → teardown
make broker-down       # Stop broker
```

## Dependencies

- [franz-go](https://github.com/twmb/franz-go) — pure Go Kafka client
- [hdrhistogram-go](https://github.com/HdrHistogram/hdrhistogram-go) — HDR histogram with coordinated omission
- [cobra](https://github.com/spf13/cobra) — CLI framework

## License

MIT
