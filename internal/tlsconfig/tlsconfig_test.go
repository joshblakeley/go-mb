package tlsconfig_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/redpanda-data/go-bench/internal/tlsconfig"
)

func TestBuildDisabled(t *testing.T) {
	cfg, err := tlsconfig.Build(false, "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil tls.Config when TLS disabled")
	}
}

func TestBuildSystemRoots(t *testing.T) {
	cfg, err := tlsconfig.Build(true, "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg == nil {
		t.Error("expected non-nil tls.Config for system roots")
	}
}

func TestBuildInvalidPath(t *testing.T) {
	// enabled=true with missing file
	cfg, err := tlsconfig.Build(true, "/nonexistent/ca.pem")
	if err == nil {
		t.Error("expected error for nonexistent path with enabled=true")
	}
	if cfg != nil {
		t.Error("expected nil config on error")
	}

	// enabled=false with missing file — caCertPath non-empty still triggers read
	cfg2, err2 := tlsconfig.Build(false, "/nonexistent/ca.pem")
	if err2 == nil {
		t.Error("expected error for nonexistent path even with enabled=false")
	}
	if cfg2 != nil {
		t.Error("expected nil config on error")
	}
}

func TestBuildValidPEM(t *testing.T) {
	pemBytes := generateSelfSignedCACert(t)
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatalf("write test PEM: %v", err)
	}

	cfg, err := tlsconfig.Build(true, path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if cfg.RootCAs == nil {
		t.Error("expected non-nil RootCAs pool")
	}
}

// generateSelfSignedCACert generates a minimal self-signed CA certificate PEM
// for use in tests only.
func generateSelfSignedCACert(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
