// Package tlstest provides TLS certificate generation for testing.
// All certificates are created using Go's crypto stdlib — no external tools needed.
// Generated files auto-clean via t.TempDir().
//
// This package lives in the root gokit module so it can be imported by both
// root module tests (e.g. security/tls_test.go) and sub-module tests
// (e.g. httpclient, grpc) without circular dependencies.
//
// Usage:
//
//	func TestWithTLS(t *testing.T) {
//	    certs := tlstest.GenerateTLSCerts(t)
//	    // certs.CAFile, certs.CertFile, certs.KeyFile are valid PEM files
//	}
package tlstest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TLSCerts holds paths to generated TLS certificate files and parsed objects.
type TLSCerts struct {
	// CAFile is the path to the CA certificate PEM file.
	CAFile string
	// CertFile is the path to the server/client certificate PEM file.
	CertFile string
	// KeyFile is the path to the server/client private key PEM file.
	KeyFile string

	// CACert is the parsed CA certificate.
	CACert *x509.Certificate
	// CAKey is the CA private key (for signing additional certs).
	CAKey *ecdsa.PrivateKey
	// ServerTLS is a ready-to-use tls.Certificate for server/client use.
	ServerTLS tls.Certificate
	// CertPool contains the CA certificate for client-side verification.
	CertPool *x509.CertPool
}

// GenerateTLSCerts creates a self-signed CA and a server certificate for testing.
// The server cert is valid for localhost, 127.0.0.1, and [::1].
// Files are written to t.TempDir() and auto-cleaned on test completion.
func GenerateTLSCerts(t testing.TB) *TLSCerts {
	t.Helper()
	dir := t.TempDir()

	// --- Generate CA ---
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("tlstest: generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"GoKit Test CA"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("tlstest: create CA cert: %v", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("tlstest: parse CA cert: %v", err)
	}

	caFile := filepath.Join(dir, "ca.pem")
	writePEM(t, caFile, "CERTIFICATE", caDER)

	// --- Generate server/client certificate ---
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("tlstest: generate server key: %v", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"GoKit Test"},
			CommonName:   "localhost",
		},
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("tlstest: create server cert: %v", err)
	}

	certFile := filepath.Join(dir, "cert.pem")
	writePEM(t, certFile, "CERTIFICATE", serverDER)

	keyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatalf("tlstest: marshal server key: %v", err)
	}
	keyFile := filepath.Join(dir, "key.pem")
	writePEM(t, keyFile, "EC PRIVATE KEY", keyDER)

	// Build tls.Certificate for programmatic use
	serverTLS, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("tlstest: load key pair: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	return &TLSCerts{
		CAFile:    caFile,
		CertFile:  certFile,
		KeyFile:   keyFile,
		CACert:    caCert,
		CAKey:     caKey,
		ServerTLS: serverTLS,
		CertPool:  pool,
	}
}

// WriteInvalidPEM writes a file with content that looks like PEM but isn't a valid certificate.
// Useful for testing error paths.
func WriteInvalidPEM(t testing.TB, filename string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filename)
	content := []byte("-----BEGIN CERTIFICATE-----\nnot-valid-base64-data\n-----END CERTIFICATE-----\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("tlstest: write invalid PEM: %v", err)
	}
	return path
}

func writePEM(t testing.TB, path, blockType string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("tlstest: create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		t.Fatalf("tlstest: encode PEM %s: %v", path, err)
	}
}
