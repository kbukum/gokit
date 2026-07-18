package encryption

import (
	"crypto/cipher"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// ChaCha20Service handles encryption/decryption using ChaCha20-Poly1305. This is a modern AEAD cipher that performs well on CPUs without AES hardware acceleration (e.g., ARM devices, older processors).
type ChaCha20Service struct {
	passphrase []byte
}

// NewChaCha20 creates a new ChaCha20-Poly1305 encryption service. The passphrase is stretched with PBKDF2-SHA256 using a random 16-byte salt per encryption.
func NewChaCha20(key string) (*ChaCha20Service, error) {
	return &ChaCha20Service{passphrase: []byte(key)}, nil
}

func newChaCha20Poly1305(key []byte) (cipher.AEAD, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("create chacha20: %w", err)
	}

	return aead, nil
}

// Encrypt encrypts plaintext and returns base64(salt || nonce || ciphertext).
func (s *ChaCha20Service) Encrypt(plaintext string) (string, error) {
	return encryptWithAEAD(s.passphrase, newChaCha20Poly1305, plaintext)
}

// Decrypt decrypts a base64-encoded ciphertext.
func (s *ChaCha20Service) Decrypt(ciphertext string) (string, error) {
	return decryptWithAEAD(s.passphrase, newChaCha20Poly1305, ciphertext)
}
