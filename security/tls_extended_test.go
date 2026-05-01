package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/security/tlstest"
)

// ---------------------------------------------------------------------------
// MinVersion enforcement
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_DefaultMinVersionIsTLS12(t *testing.T) {
	cfg := &TLSConfig{SkipVerify: true}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MinVersion != tls.VersionTLS12 {
		t.Errorf("default MinVersion = %#x, want %#x (TLS 1.2)", result.MinVersion, tls.VersionTLS12)
	}
}

func TestTLSConfig_Build_MinVersionTLS13Accepted(t *testing.T) {
	cfg := &TLSConfig{SkipVerify: true, MinVersion: tls.VersionTLS13}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %#x, want %#x (TLS 1.3)", result.MinVersion, tls.VersionTLS13)
	}
}

func TestTLSConfig_Build_MinVersionTLS12Accepted(t *testing.T) {
	cfg := &TLSConfig{SkipVerify: true, MinVersion: tls.VersionTLS12}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want %#x (TLS 1.2)", result.MinVersion, tls.VersionTLS12)
	}
}

func TestTLSConfig_Build_ZeroMinVersionDefaultsToTLS12(t *testing.T) {
	cfg := &TLSConfig{SkipVerify: true, MinVersion: 0}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion 0 should default to TLS 1.2 (%#x), got %#x", tls.VersionTLS12, result.MinVersion)
	}
}

// ---------------------------------------------------------------------------
// Certificate chain validation: intermediate + root CA
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_CertificateChainWithIntermediateCA(t *testing.T) {
	dir := t.TempDir()

	// Generate root CA
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate root key: %v", err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Test Root CA"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create root cert: %v", err)
	}
	rootCert, _ := x509.ParseCertificate(rootDER)

	// Generate intermediate CA signed by root
	interKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate intermediate key: %v", err)
	}
	interTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{Organization: []string{"Test Intermediate CA"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	interDER, err := x509.CreateCertificate(rand.Reader, interTemplate, rootCert, &interKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create intermediate cert: %v", err)
	}

	// Write a CA bundle (root + intermediate) to a single PEM file
	caBundle := filepath.Join(dir, "ca-bundle.pem")
	f, err := os.Create(caBundle)
	if err != nil {
		t.Fatalf("create ca-bundle: %v", err)
	}
	_ = pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	_ = pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: interDER})
	f.Close()

	cfg := &TLSConfig{CAFile: caBundle}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if result.RootCAs == nil {
		t.Fatal("expected RootCAs to be set with CA bundle")
	}

	// Verify the pool contains multiple certs by checking the intermediate is trusted
	interCert, _ := x509.ParseCertificate(interDER)
	_, err = interCert.Verify(x509.VerifyOptions{Roots: result.RootCAs})
	if err != nil {
		t.Errorf("intermediate cert should be verifiable against root: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Expired certificate loading → error
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_ExpiredCertificateLoadsButExpired(t *testing.T) {
	dir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Certificate that expired yesterday
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired.test"},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     time.Now().Add(-24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create expired cert: %v", err)
	}

	certFile := filepath.Join(dir, "expired-cert.pem")
	writePEMFile(t, certFile, "CERTIFICATE", certDER)

	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyFile := filepath.Join(dir, "expired-key.pem")
	writePEMFile(t, keyFile, "EC PRIVATE KEY", keyDER)

	// Build succeeds — TLS config loads the cert, expiry is checked at handshake time
	cfg := &TLSConfig{CertFile: certFile, KeyFile: keyFile}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("Build() should load expired cert (expiry checked at handshake): %v", err)
	}
	if len(result.Certificates) != 1 {
		t.Errorf("expected 1 certificate loaded, got %d", len(result.Certificates))
	}
}

// ---------------------------------------------------------------------------
// Self-signed certificate as CA
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_SelfSignedCertAsCA(t *testing.T) {
	dir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "self-signed-ca.test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create self-signed cert: %v", err)
	}

	caFile := filepath.Join(dir, "self-signed-ca.pem")
	writePEMFile(t, caFile, "CERTIFICATE", certDER)

	cfg := &TLSConfig{CAFile: caFile}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("Build() error with self-signed CA: %v", err)
	}
	if result.RootCAs == nil {
		t.Error("expected RootCAs to be set for self-signed CA")
	}
}

// ---------------------------------------------------------------------------
// mTLS: both client and server certs configured correctly
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_mTLSFullConfiguration(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := &TLSConfig{
		CAFile:     certs.CAFile,
		CertFile:   certs.CertFile,
		KeyFile:    certs.KeyFile,
		ServerName: "localhost",
	}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("mTLS Build() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil tls.Config for mTLS")
	}
	if result.RootCAs == nil {
		t.Error("mTLS: expected RootCAs for server verification")
	}
	if len(result.Certificates) != 1 {
		t.Errorf("mTLS: expected 1 client certificate, got %d", len(result.Certificates))
	}
	if result.ServerName != "localhost" {
		t.Errorf("mTLS: ServerName = %q, want %q", result.ServerName, "localhost")
	}
	if result.MinVersion != tls.VersionTLS12 {
		t.Errorf("mTLS: MinVersion = %#x, want TLS 1.2", result.MinVersion)
	}
}

// ---------------------------------------------------------------------------
// mTLS: missing client cert → error (only CertFile, no KeyFile)
// ---------------------------------------------------------------------------

func TestTLSConfig_Validate_mTLSMissingKeyFile(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := &TLSConfig{
		CAFile:   certs.CAFile,
		CertFile: certs.CertFile,
		// KeyFile intentionally missing
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error when CertFile set without KeyFile")
	}
	if !strings.Contains(err.Error(), "cert_file and key_file must be provided together") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestTLSConfig_Validate_mTLSMissingCertFile(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := &TLSConfig{
		CAFile:  certs.CAFile,
		KeyFile: certs.KeyFile,
		// CertFile intentionally missing
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error when KeyFile set without CertFile")
	}
}

// ---------------------------------------------------------------------------
// Invalid cert/key file paths → clear error messages
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_NonexistentCertFileClearError(t *testing.T) {
	cfg := &TLSConfig{
		CertFile: "/nonexistent/path/cert.pem",
		KeyFile:  "/nonexistent/path/key.pem",
	}
	_, err := cfg.Build()
	if err == nil {
		t.Fatal("expected error for nonexistent cert files")
	}
	if !strings.Contains(err.Error(), "security/tls:") {
		t.Errorf("error should have security/tls prefix: %v", err)
	}
	if !strings.Contains(err.Error(), "failed to load client certificate") {
		t.Errorf("error should mention client certificate loading: %v", err)
	}
}

func TestTLSConfig_Build_NonexistentCAFileClearError(t *testing.T) {
	cfg := &TLSConfig{CAFile: "/nonexistent/path/ca.pem"}
	_, err := cfg.Build()
	if err == nil {
		t.Fatal("expected error for nonexistent CA file")
	}
	if !strings.Contains(err.Error(), "security/tls:") {
		t.Errorf("error should have security/tls prefix: %v", err)
	}
	if !strings.Contains(err.Error(), "failed to read CA file") {
		t.Errorf("error should mention CA file reading: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Empty string paths vs nil paths
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_EmptyStringPathsProduceNilConfig(t *testing.T) {
	cfg := &TLSConfig{
		CAFile:   "",
		CertFile: "",
		KeyFile:  "",
	}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatal("empty string paths (no settings) should produce nil config")
	}
}

func TestTLSConfig_Build_EmptyPathsWithSkipVerify(t *testing.T) {
	cfg := &TLSConfig{
		SkipVerify: true,
		CAFile:     "",
		CertFile:   "",
		KeyFile:    "",
	}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil config when SkipVerify is set")
	}
	if result.RootCAs != nil {
		t.Error("expected nil RootCAs with empty CAFile")
	}
	if len(result.Certificates) != 0 {
		t.Errorf("expected no client certs, got %d", len(result.Certificates))
	}
}

// ---------------------------------------------------------------------------
// Cert/key mismatch (wrong key for cert)
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_CertKeyMismatchError(t *testing.T) {
	dir := t.TempDir()

	// Generate two separate key pairs
	key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// Create cert signed with key1
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key1.PublicKey, key1)
	certFile := filepath.Join(dir, "cert.pem")
	writePEMFile(t, certFile, "CERTIFICATE", certDER)

	// Write key2 (mismatched key)
	keyDER, _ := x509.MarshalECPrivateKey(key2)
	keyFile := filepath.Join(dir, "wrong-key.pem")
	writePEMFile(t, keyFile, "EC PRIVATE KEY", keyDER)

	cfg := &TLSConfig{CertFile: certFile, KeyFile: keyFile}
	_, err := cfg.Build()
	if err == nil {
		t.Fatal("expected error for cert/key mismatch")
	}
	if !strings.Contains(err.Error(), "security/tls:") {
		t.Errorf("error should have security/tls prefix: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Permission-denied cert file
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_PermissionDeniedCAFile(t *testing.T) {
	dir := t.TempDir()
	caFile := filepath.Join(dir, "no-read-ca.pem")
	if err := os.WriteFile(caFile, []byte("data"), 0o000); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(caFile, 0o600) })

	cfg := &TLSConfig{CAFile: caFile}
	_, err := cfg.Build()
	if err == nil {
		t.Fatal("expected error for permission-denied CA file")
	}
	if !strings.Contains(err.Error(), "failed to read CA file") && !strings.Contains(err.Error(), "failed to parse CA certificate") {
		t.Errorf("error should mention CA file read or parse failure: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Build() produces valid tls.Config with correct MinVersion
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_ProducesValidConfig(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := &TLSConfig{
		CAFile:     certs.CAFile,
		CertFile:   certs.CertFile,
		KeyFile:    certs.KeyFile,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS13,
	}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// Verify all fields are properly set
	if result.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false")
	}
	if result.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %#x, want TLS 1.3", result.MinVersion)
	}
	if result.ServerName != "localhost" {
		t.Errorf("ServerName = %q, want %q", result.ServerName, "localhost")
	}
	if result.RootCAs == nil {
		t.Error("RootCAs should not be nil")
	}
	if len(result.Certificates) != 1 {
		t.Errorf("Certificates length = %d, want 1", len(result.Certificates))
	}
}

// ---------------------------------------------------------------------------
// IsEnabled() returns correct boolean for edge cases
// ---------------------------------------------------------------------------

func TestTLSConfig_IsEnabled_KeyFileOnlyNotEnabled(t *testing.T) {
	cfg := &TLSConfig{KeyFile: "key.pem"}
	if cfg.IsEnabled() {
		t.Error("KeyFile alone should not enable TLS (CertFile is the trigger)")
	}
}

func TestTLSConfig_IsEnabled_MinVersionOnlyEnabled(t *testing.T) {
	cfg := &TLSConfig{MinVersion: tls.VersionTLS13}
	if !cfg.IsEnabled() {
		t.Error("MinVersion alone should enable TLS")
	}
}

func TestTLSConfig_IsEnabled_MultipleFieldsCombined(t *testing.T) {
	cfg := &TLSConfig{
		SkipVerify: true,
		CAFile:     "ca.pem",
		CertFile:   "cert.pem",
		ServerName: "example.com",
	}
	if !cfg.IsEnabled() {
		t.Error("multiple fields set should be enabled")
	}
}

// ---------------------------------------------------------------------------
// Validate() catches all invalid combinations
// ---------------------------------------------------------------------------

func TestTLSConfig_Validate_ZeroValueValid(t *testing.T) {
	cfg := &TLSConfig{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero-value config should be valid: %v", err)
	}
}

func TestTLSConfig_Validate_AllFieldsSetValid(t *testing.T) {
	cfg := &TLSConfig{
		SkipVerify: true,
		CAFile:     "ca.pem",
		CertFile:   "cert.pem",
		KeyFile:    "key.pem",
		ServerName: "example.com",
		MinVersion: tls.VersionTLS13,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("all fields set with both cert+key should be valid: %v", err)
	}
}

func TestTLSConfig_Validate_ErrorMessageDoesNotLeakPaths(t *testing.T) {
	cfg := &TLSConfig{CertFile: "/secret/path/cert.pem"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	// Error should be generic, not leak the actual file path
	if strings.Contains(err.Error(), "/secret/path") {
		t.Error("validation error should not contain file paths")
	}
}

// ---------------------------------------------------------------------------
// Invalid PEM content in cert/key files
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_InvalidPEMCertFile(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "bad-cert.pem")
	keyFile := filepath.Join(dir, "bad-key.pem")
	_ = os.WriteFile(certFile, []byte("not a PEM file"), 0o600)
	_ = os.WriteFile(keyFile, []byte("not a PEM file"), 0o600)

	cfg := &TLSConfig{CertFile: certFile, KeyFile: keyFile}
	_, err := cfg.Build()
	if err == nil {
		t.Fatal("expected error for invalid PEM cert/key")
	}
	if !strings.Contains(err.Error(), "security/tls:") {
		t.Errorf("error should have security/tls prefix: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Large certificate file (not a real cert, just large)
// ---------------------------------------------------------------------------

func TestTLSConfig_Build_LargeCAFile(t *testing.T) {
	dir := t.TempDir()
	caFile := filepath.Join(dir, "large-ca.pem")

	// Write a large file that is valid-ish PEM but huge
	largeData := make([]byte, 1024*1024) // 1MB
	for i := range largeData {
		largeData[i] = 'A'
	}
	content := append([]byte("-----BEGIN CERTIFICATE-----\n"), largeData...)
	content = append(content, []byte("\n-----END CERTIFICATE-----\n")...)
	_ = os.WriteFile(caFile, content, 0o600)

	cfg := &TLSConfig{CAFile: caFile}
	_, err := cfg.Build()
	// Should fail to parse (not a valid cert)
	if err == nil {
		t.Fatal("expected error for large invalid CA file")
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func writePEMFile(t *testing.T, path, blockType string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		t.Fatalf("encode PEM %s: %v", path, err)
	}
}
