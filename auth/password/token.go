package password

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// GenerateToken creates a cryptographically secure random token of the
// specified byte length, returned as a hex-encoded string.
// Common usage: session tokens, API keys, email verification tokens.
func GenerateToken(length int) (string, error) {
	bytes, err := generateRandomBytes(length)
	if err != nil {
		return "", fmt.Errorf("password: generate token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// HashSHA256 returns the SHA-256 hex digest of the input string.
// Useful for hashing tokens before storing them in a database
// (store the hash, compare hashes — never store raw tokens).
func HashSHA256(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

// generateRandomBytes returns cryptographically secure random bytes.
func generateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}
