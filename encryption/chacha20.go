package encryption

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/pbkdf2"
)

// ChaCha20Service handles encryption/decryption using ChaCha20-Poly1305.
// This is a modern AEAD cipher that performs well on CPUs without AES hardware
// acceleration (e.g., ARM devices, older processors).
type ChaCha20Service struct {
	passphrase []byte
}

// NewChaCha20 creates a new ChaCha20-Poly1305 encryption service.
// The key is derived using PBKDF2-SHA256 with a random salt per operation.
func NewChaCha20(key string) (*ChaCha20Service, error) {
	return &ChaCha20Service{passphrase: []byte(key)}, nil
}

func (s *ChaCha20Service) deriveKey(salt []byte) []byte {
	return pbkdf2.Key(s.passphrase, salt, pbkdf2Iter, pbkdf2KeyLen, sha256.New)
}

// Encrypt encrypts plaintext and returns a base64-encoded result.
// Format: base64(salt[16] || nonce[12] || ciphertext)
func (s *ChaCha20Service) Encrypt(plaintext string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	keyBytes := s.deriveKey(salt)

	aead, err := chacha20poly1305.New(keyBytes)
	if err != nil {
		return "", fmt.Errorf("create chacha20: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, []byte(plaintext), nil)

	// Format: salt || nonce || ciphertext
	result := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return base64.StdEncoding.EncodeToString(result), nil
}

// Decrypt decrypts a base64-encoded ciphertext.
// Expects format: base64(salt[16] || nonce[12] || ciphertext)
func (s *ChaCha20Service) Decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	if len(data) < saltSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	salt := data[:saltSize]
	remaining := data[saltSize:]

	keyBytes := s.deriveKey(salt)

	aead, err := chacha20poly1305.New(keyBytes)
	if err != nil {
		return "", fmt.Errorf("create chacha20: %w", err)
	}

	nonceSize := aead.NonceSize()
	if len(remaining) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextData := remaining[:nonceSize], remaining[nonceSize:]
	plainBytes, err := aead.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plainBytes), nil
}
