// Package tlsconfig builds *tls.Config values for use with franz-go clients.
// Named tlsconfig (not tls) to avoid import-name collision with crypto/tls.
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
	pemBytes, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert %q: %w", caCertPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("no valid PEM certificates found in %q", caCertPath)
	}
	return &tls.Config{RootCAs: pool}, nil
}
