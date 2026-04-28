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
	algorithm Algorithm
}

// WithAlgorithm selects the encryption algorithm (default: AES-256-GCM).
func WithAlgorithm(alg Algorithm) Option {
	return func(o *options) { o.algorithm = alg }
}

// New creates an Encryptor with the given key and options.
// Default algorithm is AES-256-GCM. Use WithAlgorithm to select ChaCha20-Poly1305.
//
// The key is hashed to the required length for the chosen algorithm.
func New(key string, opts ...Option) (Encryptor, error) {
	o := &options{algorithm: AlgorithmAESGCM}
	for _, opt := range opts {
		opt(o)
	}

	switch o.algorithm {
	case AlgorithmChaCha20:
		return NewChaCha20(key)
	default:
		return NewService(key)
	}
}
