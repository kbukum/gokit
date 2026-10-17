package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
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

// keyDerivation selects how each message key is derived from the configured key.
// Its value is the envelope version, so an envelope records how its key was derived.
type keyDerivation byte

const (
	// derivePassphrase stretches a human-chosen passphrase with PBKDF2-SHA256.
	derivePassphrase keyDerivation = 1
	// deriveKeyMaterial expands uniformly random key material with HKDF-SHA256.
	deriveKeyMaterial keyDerivation = 2
)

// pbkdf2Iterations is a var (not const) so tests can reduce it for speed.
var pbkdf2Iterations = 600_000

type aeadFactory func([]byte) (cipher.AEAD, error)

// Service handles encryption/decryption of sensitive data using AES-256-GCM.
type Service struct {
	key            []byte
	derivation     keyDerivation
	associatedData []byte
}

// NewService creates a new encryption service with the given key.
// The passphrase is stretched with PBKDF2-SHA256 using a random 16-byte salt per encryption.
func NewService(key string) (*Service, error) {
	return &Service{key: []byte(key), derivation: derivePassphrase}, nil
}

func deriveKey(passphrase, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, string(passphrase), salt, pbkdf2Iterations, keySize)
}

// messageKey derives the per-message key. Key material uses an algorithm-specific HKDF info,
// so one key never yields the same message key for two algorithms.
func messageKey(key []byte, derivation keyDerivation, alg Algorithm, salt []byte) ([]byte, error) {
	switch derivation {
	case derivePassphrase:
		return deriveKey(key, salt)
	case deriveKeyMaterial:
		return hkdf.Key(sha256.New, key, salt, "kit.encryption.v2/"+string(alg), keySize)
	default:
		return nil, fmt.Errorf("unsupported key derivation %d", derivation)
	}
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

// sealEnvelope seals the plaintext with a random salt and nonce and returns the base64-encoded versioned envelope.
func sealEnvelope(key []byte, derivation keyDerivation, alg Algorithm, factory aeadFactory, plaintext string, binding []byte) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return sealWith(key, derivation, alg, factory, plaintext, binding, salt, nonce)
}

// sealWith derives the message key from salt and seals the plaintext with the header as AEAD associated data.
func sealWith(key []byte, derivation keyDerivation, alg Algorithm, factory aeadFactory, plaintext string, binding, salt, nonce []byte) (string, error) {
	derived, err := messageKey(key, derivation, alg, salt)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}

	aead, err := factory(derived)
	if err != nil {
		return "", err
	}
	if aead.NonceSize() != nonceSize {
		return "", fmt.Errorf("unexpected nonce size %d", aead.NonceSize())
	}

	header, err := envelopeHeader(derivation, alg, salt, nonce)
	if err != nil {
		return "", err
	}

	ciphertext := aead.Seal(nil, nonce, []byte(plaintext), append(header, binding...))
	return encodeEnvelope(header, ciphertext), nil
}

// openEnvelope decodes the versioned envelope, verifies it was sealed with the
// expected key derivation and algorithm, and authenticates the header before decrypting.
func openEnvelope(key []byte, derivation keyDerivation, expected Algorithm, factory aeadFactory, ciphertext string, binding []byte) (string, error) {
	env, err := decodeEnvelope(ciphertext)
	if err != nil {
		return "", err
	}
	if env.derivation() != derivation {
		return "", invalidEnvelope("ciphertext key derivation does not match encryptor")
	}
	if env.algorithm() != expected {
		return "", invalidEnvelope("ciphertext algorithm does not match encryptor")
	}

	derived, err := messageKey(key, derivation, expected, env.salt())
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}

	aead, err := factory(derived)
	if err != nil {
		return "", err
	}

	associated := append([]byte(nil), env.associatedData()...)
	plaintext, err := aead.Open(nil, env.nonce(), env.ciphertext(), append(associated, binding...))
	if err != nil {
		return "", invalidEnvelope("ciphertext authentication failed").WithCause(err)
	}

	return string(plaintext), nil
}

// Encrypt encrypts plaintext and returns a base64-encoded versioned envelope.
func (s *Service) Encrypt(plaintext string) (string, error) {
	return sealEnvelope(s.key, s.derivation, AlgorithmAESGCM, newAESGCM, plaintext, s.associatedData)
}

// Decrypt decrypts a base64-encoded ciphertext envelope.
func (s *Service) Decrypt(ciphertext string) (string, error) {
	return openEnvelope(s.key, s.derivation, AlgorithmAESGCM, newAESGCM, ciphertext, s.associatedData)
}
