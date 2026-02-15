package encryption

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

// ChaCha20Service handles encryption/decryption using ChaCha20-Poly1305.
// This is a modern AEAD cipher that performs well on CPUs without AES hardware
// acceleration (e.g., ARM devices, older processors).
type ChaCha20Service struct {
	aead interface {
		Seal(dst, nonce, plaintext, additionalData []byte) []byte
		Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
		NonceSize() int
	}
}

// NewChaCha20 creates a new ChaCha20-Poly1305 encryption service.
// The key is hashed with SHA-256 to produce a consistent 32-byte key.
func NewChaCha20(key string) (*ChaCha20Service, error) {
	hasher := sha256.New()
	hasher.Write([]byte(key))
	keyBytes := hasher.Sum(nil)

	aead, err := chacha20poly1305.New(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("create chacha20: %w", err)
	}

	return &ChaCha20Service{aead: aead}, nil
}

// Encrypt encrypts plaintext and returns a base64-encoded result.
func (s *ChaCha20Service) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := s.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded ciphertext.
func (s *ChaCha20Service) Decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	nonceSize := s.aead.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextData := data[:nonceSize], data[nonceSize:]
	plaintext, err := s.aead.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
