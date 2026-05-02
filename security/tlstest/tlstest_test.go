package tlstest

import (
	"crypto/x509"
	"os"
	"testing"
)

func TestGenerateTLSCerts(t *testing.T) {
	t.Parallel()

	certs := GenerateTLSCerts(t)
	if certs.CAFile == "" || certs.CertFile == "" || certs.KeyFile == "" {
		t.Fatal("expected generated file paths")
	}
	if _, err := os.Stat(certs.CAFile); err != nil {
		t.Fatalf("expected CA file: %v", err)
	}
	if certs.CACert == nil || certs.CAKey == nil || certs.CertPool == nil {
		t.Fatal("expected parsed cert material")
	}
	if certs.ServerTLS.Certificate == nil {
		t.Fatal("expected loaded tls.Certificate")
	}
}

func TestWriteInvalidPEM(t *testing.T) {
	t.Parallel()

	path := WriteInvalidPEM(t, "invalid.pem")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if _, err := x509.ParseCertificate(data); err == nil {
		t.Fatal("expected raw PEM bytes not to parse directly as DER")
	}
}
