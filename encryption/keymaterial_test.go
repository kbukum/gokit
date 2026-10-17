package encryption

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

// Vectors computed independently with HKDF-SHA256 and the AEAD from Python's cryptography package.
// rskit-encryption asserts the same vectors, so both kits read each other's key-material envelopes.
var keyMaterialVectors = []struct {
	algorithm Algorithm
	binding   string
	envelope  string
}{
	{AlgorithmAESGCM, "", "AgEgISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7D0XZ4BESJRUjDeZ3hOvn9ERBGZ6pjHLIBfihpTxn3rI="},
	{AlgorithmAESGCM, "tenant/record/1", "AgEgISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7D0XZ4BESJRUjDeZ3hOvn9DyOVS7NETpan9tPrx2XWOE="},
	{AlgorithmChaCha20, "", "AgIgISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7woDWa+sQYBAxRPgz15Yz9B00vxuHxjes+ii1NOSwKcI="},
	{AlgorithmChaCha20, "tenant/record/1", "AgIgISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7woDWa+sQYBAxRPgz15Yz9CFWOjxVfc4t56bhewHIJZk="},
}

func sequence(from byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = from + byte(i)
	}
	return out
}

func TestKeyMaterialMatchesCrossKitVectors(t *testing.T) {
	t.Parallel()
	key, salt, nonce := sequence(0, 32), sequence(0x20, saltSize), sequence(0x30, nonceSize)
	for _, v := range keyMaterialVectors {
		t.Run(string(v.algorithm)+"/"+v.binding, func(t *testing.T) {
			t.Parallel()
			factory := newAESGCM
			if v.algorithm == AlgorithmChaCha20 {
				factory = newChaCha20Poly1305
			}
			got, err := sealWith(key, deriveKeyMaterial, v.algorithm, factory, "kit key material", []byte(v.binding), salt, nonce)
			if err != nil {
				t.Fatal(err)
			}
			if got != v.envelope {
				t.Fatalf("envelope = %s, want %s", got, v.envelope)
			}
			svc, err := New(string(key), WithKeyMaterial(), WithAlgorithm(v.algorithm), WithAssociatedData([]byte(v.binding)))
			if err != nil {
				t.Fatal(err)
			}
			if plaintext, err := svc.Decrypt(v.envelope); err != nil || plaintext != "kit key material" {
				t.Fatalf("Decrypt = %q, %v", plaintext, err)
			}
		})
	}
}

func TestKeyMaterialRoundTripsAndBindsContext(t *testing.T) {
	t.Parallel()
	key := strings.Repeat("k", 32)
	for _, alg := range []Algorithm{AlgorithmAESGCM, AlgorithmChaCha20} {
		t.Run(string(alg), func(t *testing.T) {
			t.Parallel()
			svc, err := New(key, WithKeyMaterial(), WithAlgorithm(alg), WithAssociatedData([]byte("record-1")))
			if err != nil {
				t.Fatal(err)
			}
			ciphertext, err := svc.Encrypt("provider secret")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := base64.StdEncoding.DecodeString(ciphertext)
			if err != nil || raw[0] != byte(deriveKeyMaterial) {
				t.Fatalf("envelope version = %d, %v; want %d", raw[0], err, deriveKeyMaterial)
			}
			if plaintext, err := svc.Decrypt(ciphertext); err != nil || plaintext != "provider secret" {
				t.Fatalf("Decrypt = %q, %v", plaintext, err)
			}
			other, err := New(key, WithKeyMaterial(), WithAlgorithm(alg), WithAssociatedData([]byte("record-2")))
			if err != nil {
				t.Fatal(err)
			}
			if plaintext, err := other.Decrypt(ciphertext); err == nil || plaintext != "" {
				t.Fatal("ciphertext opened under a different binding")
			}
		})
	}
}

func TestKeyMaterialRefusesShortKeys(t *testing.T) {
	t.Parallel()
	_, err := New(strings.Repeat("k", 31), WithKeyMaterial())
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("New with a 31-byte key = %v, want INVALID_INPUT", err)
	}
}

func TestKeyDerivationModesDoNotOpenEachOther(t *testing.T) {
	t.Parallel()
	key := strings.Repeat("k", 32)
	passphrase, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	material, err := New(key, WithKeyMaterial())
	if err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2]Encryptor{"passphrase to key material": {passphrase, material}, "key material to passphrase": {material, passphrase}} {
		ciphertext, err := pair[0].Encrypt("secret")
		if err != nil {
			t.Fatal(err)
		}
		plaintext, err := pair[1].Decrypt(ciphertext)
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidFormat || plaintext != "" {
			t.Errorf("%s: Decrypt = %q, %v; want INVALID_FORMAT", name, plaintext, err)
		}
	}
}
