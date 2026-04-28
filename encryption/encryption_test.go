package encryption

import (
	"testing"
)

func TestNewService(t *testing.T) {
	svc, err := NewService("test-key-123")
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	svc, err := NewService("my-secret-key")
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"simple string", "hello world"},
		{"empty string", ""},
		{"special characters", "p@$$w0rd!#%^&*()"},
		{"unicode", "こんにちは世界"},
		{"long string", "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."},
		{"json", `{"key":"value","num":42}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encrypted, err := svc.Encrypt(tc.plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}
			if encrypted == tc.plaintext && tc.plaintext != "" {
				t.Error("encrypted should differ from plaintext")
			}

			decrypted, err := svc.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}
			if decrypted != tc.plaintext {
				t.Errorf("expected %q, got %q", tc.plaintext, decrypted)
			}
		})
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	svc, _ := NewService("my-key")
	plaintext := "same input"

	enc1, _ := svc.Encrypt(plaintext)
	enc2, _ := svc.Encrypt(plaintext)

	if enc1 == enc2 {
		t.Error("encrypting the same plaintext twice should produce different ciphertexts due to random nonce")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	svc1, _ := NewService("key-one")
	svc2, _ := NewService("key-two")

	encrypted, err := svc1.Encrypt("secret data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = svc2.Decrypt(encrypted)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	svc, _ := NewService("test-key")
	_, err := svc.Decrypt("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecryptTooShort(t *testing.T) {
	svc, _ := NewService("test-key")
	// Very short base64 that decodes to fewer bytes than nonce size
	_, err := svc.Decrypt("YQ==")
	if err == nil {
		t.Error("expected error for ciphertext too short")
	}
}

func TestDifferentKeysProduceDifferentCiphertexts(t *testing.T) {
	svc1, _ := NewService("key-alpha")
	svc2, _ := NewService("key-beta")

	plaintext := "test data"
	enc1, _ := svc1.Encrypt(plaintext)
	_, _ = svc2.Encrypt(plaintext)

	// While randomness means they'd almost certainly differ anyway,
	// the important thing is they can't decrypt each other's data
	dec1, err1 := svc1.Decrypt(enc1)
	if err1 != nil || dec1 != plaintext {
		t.Error("svc1 should decrypt its own ciphertext")
	}

	_, err2 := svc2.Decrypt(enc1)
	if err2 == nil {
		t.Error("svc2 should not decrypt svc1's ciphertext")
	}
}

func TestNewServiceDifferentKeysNotEqual(t *testing.T) {
	svc1, _ := NewService("key1")
	svc2, _ := NewService("key2")
	if svc1.gcm == svc2.gcm {
		t.Error("different keys should produce different GCM instances")
	}
}

// --- ChaCha20 tests ---

func TestNewChaCha20(t *testing.T) {
	svc, err := NewChaCha20("test-key-123")
	if err != nil {
		t.Fatalf("NewChaCha20 failed: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestChaCha20EncryptDecryptRoundTrip(t *testing.T) {
	svc, err := NewChaCha20("my-secret-key")
	if err != nil {
		t.Fatalf("NewChaCha20 failed: %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"simple string", "hello world"},
		{"empty string", ""},
		{"special characters", "p@$$w0rd!#%^&*()"},
		{"unicode", "こんにちは世界"},
		{"long string", "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."},
		{"json", `{"key":"value","num":42}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encrypted, err := svc.Encrypt(tc.plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}
			if encrypted == tc.plaintext && tc.plaintext != "" {
				t.Error("encrypted should differ from plaintext")
			}

			decrypted, err := svc.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}
			if decrypted != tc.plaintext {
				t.Errorf("expected %q, got %q", tc.plaintext, decrypted)
			}
		})
	}
}

func TestChaCha20ProducesDifferentCiphertexts(t *testing.T) {
	svc, _ := NewChaCha20("my-key")
	plaintext := "same input"

	enc1, _ := svc.Encrypt(plaintext)
	enc2, _ := svc.Encrypt(plaintext)

	if enc1 == enc2 {
		t.Error("encrypting the same plaintext twice should produce different ciphertexts due to random nonce")
	}
}

func TestChaCha20DecryptWithWrongKey(t *testing.T) {
	svc1, _ := NewChaCha20("key-one")
	svc2, _ := NewChaCha20("key-two")

	encrypted, err := svc1.Encrypt("secret data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = svc2.Decrypt(encrypted)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
}

func TestChaCha20DecryptInvalidBase64(t *testing.T) {
	svc, _ := NewChaCha20("test-key")
	_, err := svc.Decrypt("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestChaCha20DecryptTooShort(t *testing.T) {
	svc, _ := NewChaCha20("test-key")
	_, err := svc.Decrypt("YQ==")
	if err == nil {
		t.Error("expected error for ciphertext too short")
	}
}

// --- Factory / Option tests ---

func TestNewDefaultAlgorithm(t *testing.T) {
	enc, err := New("my-key")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if enc == nil {
		t.Fatal("expected non-nil encryptor")
	}
	// Default should be AES-GCM (Service type)
	if _, ok := enc.(*Service); !ok {
		t.Errorf("expected *Service (AES-GCM default), got %T", enc)
	}

	// Verify round-trip works
	encrypted, err := enc.Encrypt("test data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if decrypted != "test data" {
		t.Errorf("expected 'test data', got %q", decrypted)
	}
}

func TestNewWithAESGCMAlgorithm(t *testing.T) {
	enc, err := New("my-key", WithAlgorithm(AlgorithmAESGCM))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if _, ok := enc.(*Service); !ok {
		t.Errorf("expected *Service for AES-GCM, got %T", enc)
	}
}

func TestNewWithChaCha20Algorithm(t *testing.T) {
	enc, err := New("my-key", WithAlgorithm(AlgorithmChaCha20))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if _, ok := enc.(*ChaCha20Service); !ok {
		t.Errorf("expected *ChaCha20Service for ChaCha20, got %T", enc)
	}

	// Verify round-trip works
	encrypted, err := enc.Encrypt("chacha20 test")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if decrypted != "chacha20 test" {
		t.Errorf("expected 'chacha20 test', got %q", decrypted)
	}
}

func TestAlgorithmConstants(t *testing.T) {
	if AlgorithmAESGCM != "aes-256-gcm" {
		t.Errorf("expected 'aes-256-gcm', got %q", AlgorithmAESGCM)
	}
	if AlgorithmChaCha20 != "chacha20-poly1305" {
		t.Errorf("expected 'chacha20-poly1305', got %q", AlgorithmChaCha20)
	}
}

func TestCrossAlgorithmIncompatibility(t *testing.T) {
	aes, _ := New("shared-key", WithAlgorithm(AlgorithmAESGCM))
	chacha, _ := New("shared-key", WithAlgorithm(AlgorithmChaCha20))

	encrypted, _ := aes.Encrypt("secret")
	_, err := chacha.Decrypt(encrypted)
	if err == nil {
		t.Error("expected ChaCha20 to fail decrypting AES-GCM ciphertext")
	}

	encrypted2, _ := chacha.Encrypt("secret")
	_, err = aes.Decrypt(encrypted2)
	if err == nil {
		t.Error("expected AES-GCM to fail decrypting ChaCha20 ciphertext")
	}
}
