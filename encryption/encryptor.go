package encryption

import apperrors "github.com/kbukum/gokit/errors"

// Encryptor defines the interface for symmetric encryption and decryption.
// Projects choose which implementation to use based on their requirements.
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// Algorithm represents supported encryption algorithms.
type Algorithm string

const (
	// AlgorithmAESGCM is AES-256-GCM (default, widely supported).
	AlgorithmAESGCM Algorithm = "aes-256-gcm"

	// AlgorithmChaCha20 is ChaCha20-Poly1305 (modern, fast on CPUs without AES-NI).
	AlgorithmChaCha20 Algorithm = "chacha20-poly1305"
)

// Option configures the encryption service.
type Option func(*options)

type options struct {
	algorithm      Algorithm
	associatedData []byte
	keyMaterial    bool
}

// minKeyMaterial is the shortest key accepted by WithKeyMaterial.
const minKeyMaterial = 32

// WithAlgorithm selects the encryption algorithm (default: AES-256-GCM).
func WithAlgorithm(alg Algorithm) Option {
	return func(o *options) { o.algorithm = alg }
}

// WithAssociatedData binds ciphertext to caller-owned context, which must match exactly at decryption.
// The data is copied, not stored in the ciphertext, and must use an unambiguous canonical encoding.
func WithAssociatedData(data []byte) Option {
	copied := append([]byte(nil), data...)
	return func(o *options) { o.associatedData = append([]byte(nil), copied...) }
}

// WithKeyMaterial declares the key to be uniformly random secret material of at least 32 bytes,
// such as a mounted key file, not a human-chosen passphrase. Each message key is then derived with
// HKDF-SHA256 instead of PBKDF2, which makes every operation cheap. Its envelopes use version 2;
// a key-material encryptor and a passphrase encryptor never open each other's ciphertext.
func WithKeyMaterial() Option {
	return func(o *options) { o.keyMaterial = true }
}

// New creates an Encryptor with the given key and options. Default algorithm is AES-256-GCM.
// Use WithAlgorithm to select ChaCha20-Poly1305.
//
// By default the key is a passphrase stretched with PBKDF2-SHA256; WithKeyMaterial selects HKDF-SHA256
// for random key material. Each ciphertext is a
// base64-encoded versioned envelope: version || algorithm || salt || nonce || ciphertext,
// with the header authenticated as AEAD associated data (wire-compatible with the shared encryption envelope format).
// WithAssociatedData additionally authenticates caller-supplied context without including it in the envelope.
func New(key string, opts ...Option) (Encryptor, error) {
	o := &options{algorithm: AlgorithmAESGCM}
	for _, opt := range opts {
		opt(o)
	}
	derivation := derivePassphrase
	if o.keyMaterial {
		if len(key) < minKeyMaterial {
			return nil, apperrors.InvalidInput("key", "Key material must be at least 32 bytes")
		}
		derivation = deriveKeyMaterial
	}

	switch o.algorithm {
	case AlgorithmChaCha20:
		service, err := NewChaCha20(key)
		if err != nil {
			return nil, err
		}
		service.derivation = derivation
		service.associatedData = o.associatedData
		return service, nil
	default:
		service, err := NewService(key)
		if err != nil {
			return nil, err
		}
		service.derivation = derivation
		service.associatedData = o.associatedData
		return service, nil
	}
}
