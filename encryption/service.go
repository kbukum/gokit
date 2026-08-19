package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

const (
	keySize  = 32
	saltSize = 16
)

// pbkdf2Iterations is a var (not const) so tests can reduce it for speed.
var pbkdf2Iterations = 600_000

type aeadFactory func([]byte) (cipher.AEAD, error)

// Service handles encryption/decryption of sensitive data using AES-256-GCM.
type Service struct {
	passphrase []byte
}

// NewService creates a new encryption service with the given key.
// The passphrase is stretched with PBKDF2-SHA256 using a random 16-byte salt per encryption.
func NewService(key string) (*Service, error) {
	return &Service{passphrase: []byte(key)}, nil
}

func deriveKey(passphrase, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, string(passphrase), salt, pbkdf2Iterations, keySize)
}

func newAESGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return gcm, nil
}

// sealEnvelope derives a per-message key, seals the plaintext with the header as
// AEAD associated data, and returns the base64-encoded versioned envelope.
func sealEnvelope(passphrase []byte, alg Algorithm, factory aeadFactory, plaintext string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}

	aead, err := factory(key)
	if err != nil {
		return "", err
	}
	if aead.NonceSize() != nonceSize {
		return "", fmt.Errorf("unexpected nonce size %d", aead.NonceSize())
	}

	nonce := make([]byte, nonceSize)
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	header, err := envelopeHeader(alg, salt, nonce)
	if err != nil {
		return "", err
	}

	ciphertext := aead.Seal(nil, nonce, []byte(plaintext), header)
	return encodeEnvelope(header, ciphertext), nil
}

// openEnvelope decodes the versioned envelope, verifies it was sealed by the
// expected algorithm, and authenticates the header before decrypting.
func openEnvelope(passphrase []byte, expected Algorithm, factory aeadFactory, ciphertext string) (string, error) {
	env, err := decodeEnvelope(ciphertext)
	if err != nil {
		return "", err
	}
	if env.algorithm() != expected {
		return "", invalidEnvelope("ciphertext algorithm does not match encryptor")
	}

	key, err := deriveKey(passphrase, env.salt())
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}

	aead, err := factory(key)
	if err != nil {
		return "", err
	}

	plaintext, err := aead.Open(nil, env.nonce(), env.ciphertext(), env.associatedData())
	if err != nil {
		return "", invalidEnvelope("ciphertext authentication failed").WithCause(err)
	}

	return string(plaintext), nil
}

// Encrypt encrypts plaintext and returns a base64-encoded versioned envelope.
func (s *Service) Encrypt(plaintext string) (string, error) {
	return sealEnvelope(s.passphrase, AlgorithmAESGCM, newAESGCM, plaintext)
}

// Decrypt decrypts a base64-encoded ciphertext envelope.
func (s *Service) Decrypt(ciphertext string) (string, error) {
	return openEnvelope(s.passphrase, AlgorithmAESGCM, newAESGCM, ciphertext)
}
