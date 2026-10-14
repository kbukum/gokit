package encryption_test

import (
	"errors"
	"testing"

	"github.com/kbukum/gokit/encryption"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestAssociatedDataBinding(t *testing.T) {
	t.Parallel()
	for _, algorithm := range []encryption.Algorithm{encryption.AlgorithmAESGCM, encryption.AlgorithmChaCha20} {
		t.Run(string(algorithm), func(t *testing.T) {
			t.Parallel()
			newBound := func(binding []byte) encryption.Encryptor {
				t.Helper()
				enc, err := encryption.New("fake-test-key", encryption.WithAlgorithm(algorithm), encryption.WithAssociatedData(binding))
				if err != nil {
					t.Fatal(err)
				}
				return enc
			}
			binding := []byte("organization/test/credential/one/version/1")
			enc := newBound(binding)
			ciphertext, err := enc.Encrypt("fake-secret-sentinel")
			if err != nil {
				t.Fatal(err)
			}
			got, err := newBound(binding).Decrypt(ciphertext)
			if err != nil || got != "fake-secret-sentinel" {
				t.Fatalf("matching binding did not decrypt: %v", err)
			}
			for _, other := range [][]byte{nil, []byte("organization/other/credential/one/version/1"), []byte("organization/test/credential/one/version/2")} {
				got, err := newBound(other).Decrypt(ciphertext)
				var appErr *apperrors.AppError
				if got != "" || !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidFormat {
					t.Fatalf("mismatched binding returned plaintext or lacked authentication error: %v", err)
				}
			}
			unbound := newBound(nil)
			unboundCiphertext, err := unbound.Encrypt("fake-secret-sentinel")
			if err != nil {
				t.Fatal(err)
			}
			if got, err := enc.Decrypt(unboundCiphertext); got != "" || err == nil {
				t.Fatal("bound reader accepted unbound ciphertext")
			}
		})
	}
}

func TestAssociatedDataCopiesCallerMemory(t *testing.T) {
	t.Parallel()
	binding := []byte("original")
	option := encryption.WithAssociatedData(binding)
	binding[0] = 'X'
	enc, err := encryption.New("fake-test-key", option)
	if err != nil {
		t.Fatal(err)
	}

	ciphertext, err := enc.Encrypt("fake-secret-sentinel")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := encryption.New("fake-test-key", encryption.WithAssociatedData([]byte("original")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Decrypt(ciphertext); err != nil {
		t.Fatalf("caller mutation changed binding: %v", err)
	}
}

func TestAssociatedDataEmptyMatchesUnbound(t *testing.T) {
	t.Parallel()
	for _, algorithm := range []encryption.Algorithm{encryption.AlgorithmAESGCM, encryption.AlgorithmChaCha20} {
		t.Run(string(algorithm), func(t *testing.T) {
			t.Parallel()
			unbound, err := encryption.New("fake-test-key", encryption.WithAlgorithm(algorithm))
			if err != nil {
				t.Fatal(err)
			}
			empty, err := encryption.New("fake-test-key", encryption.WithAlgorithm(algorithm), encryption.WithAssociatedData([]byte{}))
			if err != nil {
				t.Fatal(err)
			}
			ciphertext, err := unbound.Encrypt("fake-secret-sentinel")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := empty.Decrypt(ciphertext); err != nil {
				t.Fatalf("empty binding changed unbound envelope: %v", err)
			}
		})
	}
}
