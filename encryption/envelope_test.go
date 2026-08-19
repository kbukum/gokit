package encryption

import (
	"encoding/base64"
	"errors"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestEnvelopeLayout(t *testing.T) {
	svc, err := NewService("layout-key")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ct, err := svc.Encrypt("payload")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := raw[0]; got != envelopeVersion {
		t.Errorf("version byte = %d, want %d", got, envelopeVersion)
	}
	id, _ := AlgorithmAESGCM.id()
	if got := raw[1]; got != id {
		t.Errorf("algorithm byte = %d, want %d", got, id)
	}
	if len(raw) < envelopeHeaderSize+aeadTagSize {
		t.Errorf("envelope too short: %d bytes", len(raw))
	}
}

func TestDecryptRejectsWrongAlgorithm(t *testing.T) {
	aes, err := NewService("shared-key")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	cc, err := NewChaCha20("shared-key")
	if err != nil {
		t.Fatalf("NewChaCha20: %v", err)
	}
	ct, err := cc.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = aes.Decrypt(ct)
	if err == nil {
		t.Fatal("expected algorithm mismatch error")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidFormat {
		t.Errorf("expected INVALID_FORMAT AppError, got %v", err)
	}
}

func TestDecryptRejectsUnsupportedVersion(t *testing.T) {
	svc, err := NewService("ver-key")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ct, err := svc.Encrypt("payload")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(ct)
	raw[0] = 0xFF
	_, err = svc.Decrypt(base64.StdEncoding.EncodeToString(raw))
	if err == nil {
		t.Fatal("expected unsupported version error")
	}
}

func TestDecryptRejectsUnsupportedAlgorithmID(t *testing.T) {
	svc, err := NewService("alg-key")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ct, err := svc.Encrypt("payload")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(ct)
	raw[1] = 0x7F
	_, err = svc.Decrypt(base64.StdEncoding.EncodeToString(raw))
	if err == nil {
		t.Fatal("expected unsupported algorithm error")
	}
}
