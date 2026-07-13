package encryption

import "testing"

// ─── helpers ───────────────────────────────────────────────────────────

// algorithmCase bundles a human-readable name, the Algorithm constant, and a
// constructor so every test can be driven for both AES-GCM and ChaCha20.
type algorithmCase struct {
	name string
	alg  Algorithm
}

var algorithms = []algorithmCase{
	{"AES-GCM", AlgorithmAESGCM},
	{"ChaCha20", AlgorithmChaCha20},
}

func newEncryptor(t *testing.T, key string, alg Algorithm) Encryptor {
	t.Helper()
	enc, err := New(key, WithAlgorithm(alg))
	if err != nil {
		t.Fatalf("New(%s) failed: %v", alg, err)
	}
	return enc
}

func TestFactory_UnknownAlgorithmFallsToDefault(t *testing.T) {
	// Unknown algorithm falls through to the default switch case (AES-GCM)
	enc, err := New("my-key", WithAlgorithm(Algorithm("unknown-alg")))
	if err != nil {
		t.Fatalf("New with unknown algorithm should fall back to default: %v", err)
	}
	if _, ok := enc.(*Service); !ok {
		t.Errorf("expected *Service (default fallback), got %T", enc)
	}
}

func TestFactory_MultipleOptions(t *testing.T) {
	// Last option wins when multiple WithAlgorithm are passed
	enc, err := New("my-key",
		WithAlgorithm(AlgorithmAESGCM),
		WithAlgorithm(AlgorithmChaCha20),
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if _, ok := enc.(*ChaCha20Service); !ok {
		t.Errorf("last option should win: expected *ChaCha20Service, got %T", enc)
	}
}

func TestFactory_EncryptorInterface(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc, err := New("iface-key", WithAlgorithm(ac.alg))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			// Verify the returned value satisfies the Encryptor interface
			_ = enc

			ct, err := enc.Encrypt("interface test")
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			pt, err := enc.Decrypt(ct)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if pt != "interface test" {
				t.Errorf("got %q", pt)
			}
		})
	}
}

// ─── 6. Security tests ────────────────────────────────────────────────
