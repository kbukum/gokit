package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

func encryptWithAEAD(passphrase []byte, factory aeadFactory, plaintext string) (string, error) {
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

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	payload = append(payload, salt...)
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)

	return base64.StdEncoding.EncodeToString(payload), nil
}

func decryptWithAEAD(passphrase []byte, factory aeadFactory, ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	if len(data) < saltSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	salt := data[:saltSize]
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}

	aead, err := factory(key)
	if err != nil {
		return "", err
	}

	nonceSize := aead.NonceSize()
	if len(data) < saltSize+nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextData := data[saltSize:saltSize+nonceSize], data[saltSize+nonceSize:]
	plaintext, err := aead.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// Encrypt encrypts plaintext and returns base64(salt || nonce || ciphertext).
func (s *Service) Encrypt(plaintext string) (string, error) {
	return encryptWithAEAD(s.passphrase, newAESGCM, plaintext)
}

// Decrypt decrypts a base64-encoded ciphertext.
func (s *Service) Decrypt(ciphertext string) (string, error) {
	return decryptWithAEAD(s.passphrase, newAESGCM, ciphertext)
}
