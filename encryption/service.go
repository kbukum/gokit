package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltSize     = 16
	pbkdf2Iter   = 600_000
	pbkdf2KeyLen = 32
)

// Service handles encryption/decryption of sensitive data using AES-256-GCM.
type Service struct {
	passphrase []byte
}

// NewService creates a new encryption service with the given key.
// The key is derived using PBKDF2-SHA256 with a random salt per operation.
func NewService(key string) (*Service, error) {
	return &Service{passphrase: []byte(key)}, nil
}

func (s *Service) deriveKey(salt []byte) []byte {
	return pbkdf2.Key(s.passphrase, salt, pbkdf2Iter, pbkdf2KeyLen, sha256.New)
}

// Encrypt encrypts plaintext and returns a base64-encoded result.
// Format: base64(salt[16] || nonce[12] || ciphertext)
func (s *Service) Encrypt(plaintext string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	keyBytes := s.deriveKey(salt)

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Format: salt || nonce || ciphertext
	result := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return base64.StdEncoding.EncodeToString(result), nil
}

// Decrypt decrypts a base64-encoded ciphertext.
// Expects format: base64(salt[16] || nonce[12] || ciphertext)
func (s *Service) Decrypt(ciphertext string) (string, error) {
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

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(remaining) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextData := remaining[:nonceSize], remaining[nonceSize:]
	plainBytes, err := gcm.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plainBytes), nil
}
