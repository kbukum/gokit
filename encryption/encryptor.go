package encryption

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
}

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

// New creates an Encryptor with the given key and options. Default algorithm is AES-256-GCM.
// Use WithAlgorithm to select ChaCha20-Poly1305.
//
// The passphrase is stretched with PBKDF2-SHA256 and each ciphertext is a
// base64-encoded versioned envelope: version || algorithm || salt || nonce || ciphertext,
// with the header authenticated as AEAD associated data (wire-compatible with the shared encryption envelope format).
// WithAssociatedData additionally authenticates caller-supplied context without including it in the envelope.
func New(key string, opts ...Option) (Encryptor, error) {
	o := &options{algorithm: AlgorithmAESGCM}
	for _, opt := range opts {
		opt(o)
	}

	switch o.algorithm {
	case AlgorithmChaCha20:
		service, err := NewChaCha20(key)
		if err != nil {
			return nil, err
		}
		service.associatedData = o.associatedData
		return service, nil
	default:
		service, err := NewService(key)
		if err != nil {
			return nil, err
		}
		service.associatedData = o.associatedData
		return service, nil
	}
}
