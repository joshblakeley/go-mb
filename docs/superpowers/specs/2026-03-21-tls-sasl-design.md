# TLS and SASL Authentication Design

## Goal

Enable go-bench to connect to TLS-encrypted Kafka/Redpanda clusters and authenticate via SASL (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512), matching the security configuration expressible in OMB's driver YAML and enabling benchmarks against Redpanda Cloud and other production clusters.

## Background

OMB's Kafka driver supports security via Java Kafka client properties:

```yaml
commonConfig: |
  security.protocol=SASL_SSL
  sasl.mechanism=SCRAM-SHA-512
  sasl.jaas.config=org.apache.kafka.clients.sasl.scram.ScramLoginModule required \
    username="user" password="pass";
```

go-bench currently has no security support, making it impossible to benchmark any cluster that requires TLS or authentication.

## Scope

- **TLS:** Server-side TLS only. `--tls` enables with system root CAs; `--tls-ca-cert` loads a custom CA PEM. No mTLS (client certificates) in this iteration.
- **SASL:** PLAIN, SCRAM-SHA-256, SCRAM-SHA-512. Both TLS and SASL options apply to **both** producer and consumer clients.

## Flags

| Flag | Default | Notes |
|---|---|---|
| `--tls` | `false` | Enable TLS using system root CAs |
| `--tls-ca-cert` | `""` | Path to custom CA cert PEM file; implies `--tls` |
| `--sasl-mechanism` | `""` | `plain`, `scram-sha-256`, `scram-sha-512` |
| `--sasl-username` | `""` | Required when `--sasl-mechanism` is set |
| `--sasl-password` | `""` | Required when `--sasl-mechanism` is set |

## Architecture

Six files change; one new package.

### New package: `internal/tlsconfig`

Named `tlsconfig` (not `tls`) to avoid collision with stdlib `crypto/tls` in importers.

**`internal/tlsconfig/tlsconfig.go`**

```go
package tlsconfig

import (
    "crypto/tls"
    "crypto/x509"
    "fmt"
    "os"
)

// Build returns a *tls.Config for use with franz-go's kgo.DialTLSConfig option.
//
//   - enabled=false, caCertPath="" → nil, nil  (TLS disabled; caller omits the option)
//   - enabled=true,  caCertPath="" → &tls.Config{} using system root CAs
//   - caCertPath non-empty         → &tls.Config{RootCAs: pool} loaded from the PEM file;
//                                    implies enabled regardless of the enabled flag
//
// Returns a non-nil error if caCertPath is set but the file cannot be read or
// contains no valid PEM certificates.
func Build(enabled bool, caCertPath string) (*tls.Config, error) {
    if !enabled && caCertPath == "" {
        return nil, nil
    }
    if caCertPath == "" {
        return &tls.Config{}, nil
    }
    pem, err := os.ReadFile(caCertPath)
    if err != nil {
        return nil, fmt.Errorf("read CA cert %q: %w", caCertPath, err)
    }
    pool := x509.NewCertPool()
    if !pool.AppendCertsFromPEM(pem) {
        return nil, fmt.Errorf("no valid PEM certificates found in %q", caCertPath)
    }
    return &tls.Config{RootCAs: pool}, nil
}
```

**`internal/tlsconfig/tlsconfig_test.go`** — four tests (TDD):

- `TestBuildDisabled`: `Build(false, "")` → nil config, nil error
- `TestBuildSystemRoots`: `Build(true, "")` → non-nil config, nil error
- `TestBuildInvalidPath`: `Build(true, "/nonexistent/ca.pem")` → nil config, non-nil error
- `TestBuildValidPEM`: write a hardcoded self-signed CA cert to `t.TempDir()`, `Build(true, path)` → non-nil config with non-nil `RootCAs`, nil error

Hardcoded test CA cert (minimal self-signed, 2048-bit RSA, for test use only — not cryptographically strong):

```go
const testCACert = `-----BEGIN CERTIFICATE-----
MIICpDCCAYwCCQDU+pQ4pHgSpDANBgkqhkiG9w0BAQsFADAUMRIwEAYDVQQDDAl0
ZXN0LWNlcnQwHhcNMjMwMTAxMDAwMDAwWhcNMjQwMTAxMDAwMDAwWjAUMRIwEAYD
VQQDDAl0ZXN0LWNlcnQwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQC7
o4qne60TB3wolTBq5NMAO6UD1JPzNRqrJpFO0RVHAnPQfD6Gp2MRqRpWH7PpzHL
pPFqFWSmFjsPpKLfDEJ5yIbMV5kNSTHAqlkOvdlzRVLaEVKtgJnmxkx5wnpRn9Wd
0QoNnsFcJkQxuHivXR6lKiOVBPDJI3biHMDd6MwCZIjFnFh12r4smEF0IfnbAAEA
AQABAoIBAC5RgZ+hBx7xHNaMpPgwGpnFa7DmEkMFQBqzZcGMEMiAFlEWiLpMhzaN
kKgDmNGMxCKKIlJ9bBGGfWMwLgxqWGVqNENTEFQwzRBTU05QRlRYNVVGN1BPVVNN
VFdVWThRSjhMSEdNTExLTjdQWFhMSk5SMjhJTTlOSzFCMgIDAQABMA0GCSqGSIb3
DQEBCwUAA4IBAQCxzFMKuBCa5PVMOStNuDLMUaRBHhZMiB7dEvQlnLVTBJBtQRo6
9q3K+UMSHGPnOGNCNVGMRXt1on+nH2GOZH2PlNjSLlXNi9wWsHjsN+gcFNzDN5er
aefZVEPvN/fVCFNFTDhIVzRZRVpZVVVNTlpXT0RPVFdIMFYzQ0NHSFNOVUZ6VFVB
-----END CERTIFICATE-----`
```

Note: this test certificate may not parse as valid x509 — if `pool.AppendCertsFromPEM` returns false, generate a real self-signed cert using `crypto/x509` and `crypto/rsa` in a `TestMain` or helper. The test must confirm that a valid PEM file produces a non-nil `RootCAs` pool.

### `internal/config/config.go`

Add five fields after `BatchMaxBytes`:

```go
TLS           bool   // enable TLS with system root CAs
TLSCACert     string // path to CA cert PEM file; implies TLS
SASLMechanism string // "plain", "scram-sha-256", "scram-sha-512"; "" = disabled
SASLUsername  string // required when SASLMechanism is set
SASLPassword  string // required when SASLMechanism is set
```

`Default()`: all zero/false/empty values — no changes to struct literal (zero values are the correct defaults).

`Validate()` additions (before final `return nil`):

```go
validMechanisms := map[string]bool{"plain": true, "scram-sha-256": true, "scram-sha-512": true}
if c.SASLMechanism != "" && !validMechanisms[c.SASLMechanism] {
    return errors.New("sasl-mechanism must be plain, scram-sha-256, or scram-sha-512")
}
if c.SASLMechanism != "" && c.SASLUsername == "" {
    return errors.New("sasl-username is required when sasl-mechanism is set")
}
if c.SASLMechanism != "" && c.SASLPassword == "" {
    return errors.New("sasl-password is required when sasl-mechanism is set")
}
if (c.SASLUsername != "" || c.SASLPassword != "") && c.SASLMechanism == "" {
    return errors.New("sasl-mechanism is required when sasl-username or sasl-password is set")
}
```

### `internal/config/config_test.go`

New unit tests:

- `TestValidateSASLMechanism`: invalid `"kerberos"` returns error; valid `"plain"`, `"scram-sha-256"`, `"scram-sha-512"` pass
- `TestValidateSASLRequiresCredentials`: mechanism set without username returns error; without password returns error
- `TestValidateSASLCredentialsRequireMechanism`: username set without mechanism returns error

### `cmd/bench/main.go`

Five new flags after `--batch-max-bytes`:

```go
f.BoolVar(&cfg.TLS, "tls", cfg.TLS, "enable TLS using system root CAs")
f.StringVar(&cfg.TLSCACert, "tls-ca-cert", cfg.TLSCACert, "path to CA certificate PEM file (implies --tls)")
f.StringVar(&cfg.SASLMechanism, "sasl-mechanism", cfg.SASLMechanism, "SASL mechanism: plain, scram-sha-256, scram-sha-512")
f.StringVar(&cfg.SASLUsername, "sasl-username", cfg.SASLUsername, "SASL username")
f.StringVar(&cfg.SASLPassword, "sasl-password", cfg.SASLPassword, "SASL password")
```

### `internal/bench/bench.go`

Two additions:

**1. New imports:**
```go
"github.com/redpanda-data/go-bench/internal/tlsconfig"
"github.com/twmb/franz-go/pkg/sasl/plain"
"github.com/twmb/franz-go/pkg/sasl/scram"
```

**2. Shared options built before client construction:**

```go
// Build shared TLS and SASL options applied to both producer and consumer clients.
tlsCfg, err := tlsconfig.Build(cfg.TLS || cfg.TLSCACert != "", cfg.TLSCACert)
if err != nil {
    return fmt.Errorf("build TLS config: %w", err)
}
var sharedOpts []kgo.Opt
if tlsCfg != nil {
    sharedOpts = append(sharedOpts, kgo.DialTLSConfig(tlsCfg))
}
sharedOpts = append(sharedOpts, saslOpts(cfg)...)
```

Producer client updated to include `sharedOpts`:
```go
producerClient, err := kgo.NewClient(
    append(
        append([]kgo.Opt{
            kgo.SeedBrokers(cfg.Brokers...),
            kgo.DefaultProduceTopic(cfg.Topic),
        }, producerOpts(cfg)...),
        sharedOpts...,
    )...,
)
```

Consumer client updated to include `sharedOpts`:
```go
consumerClient, err := kgo.NewClient(
    append([]kgo.Opt{
        kgo.SeedBrokers(cfg.Brokers...),
        kgo.ConsumeTopics(cfg.Topic),
        kgo.ConsumerGroup(cfg.ConsumerGroup),
    }, sharedOpts...)...,
)
```

**3. New `saslOpts` helper (after `producerOpts`):**

```go
// saslOpts returns the franz-go SASL option for the configured mechanism.
// Returns an empty slice when SASLMechanism is empty (no authentication).
func saslOpts(cfg *config.Config) []kgo.Opt {
    switch cfg.SASLMechanism {
    case "plain":
        return []kgo.Opt{kgo.SASL(plain.Auth{
            User: cfg.SASLUsername,
            Pass: cfg.SASLPassword,
        }.AsMechanism())}
    case "scram-sha-256":
        return []kgo.Opt{kgo.SASL(scram.Auth{
            User: cfg.SASLUsername,
            Pass: cfg.SASLPassword,
        }.AsMechanism(scram.SHA256))}
    case "scram-sha-512":
        return []kgo.Opt{kgo.SASL(scram.Auth{
            User: cfg.SASLUsername,
            Pass: cfg.SASLPassword,
        }.AsMechanism(scram.SHA512))}
    }
    return nil
}
```

## Testing

Unit tests cover all new logic that can be tested without a broker:
- `internal/tlsconfig/tlsconfig_test.go`: 4 tests (disabled, system roots, invalid path, valid PEM)
- `internal/config/config_test.go`: 3 new tests for SASL validation rules

The existing `TestIntegrationSmoke` exercises the full `bench.Run` path and will compile and exercise the new shared-opts code path (with TLS/SASL disabled, the default).

## go.mod

`github.com/twmb/franz-go/pkg/sasl/plain` and `github.com/twmb/franz-go/pkg/sasl/scram` are sub-packages of `github.com/twmb/franz-go` which is already in `go.mod`. No new module dependencies required.

## Out of Scope

- mTLS (client certificate + key)
- SASL GSSAPI / Kerberos
- `--tls-skip-verify` (insecure mode)
- Credential injection via environment variables
