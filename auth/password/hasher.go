// Package password provides password hashing and verification utilities.
//
// It defines a Hasher interface with multiple implementations:
//   - BcryptHasher: industry-standard bcrypt hashing
//   - Argon2Hasher: modern argon2id hashing (recommended for new projects)
//
// Usage:
//
//	hasher := password.NewBcryptHasher()
//	hash, err := hasher.Hash("my-password")
//	err = hasher.Verify("my-password", hash)
package password

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Hasher defines the interface for password hashing and verification.
// Projects choose which implementation to use based on their requirements.
type Hasher interface {
	// Hash returns a hashed representation of the password.
	Hash(password string) (string, error)

	// Verify checks if a password matches the given hash.
	// Returns nil if they match, an error otherwise.
	Verify(password, hash string) error
}

// --- Bcrypt Implementation ---

// BcryptHasher implements Hasher using bcrypt.
type BcryptHasher struct {
	cost int
}

// BcryptOption configures the bcrypt hasher.
type BcryptOption func(*BcryptHasher)

// WithCost sets the bcrypt cost parameter (default: 12, range: 4-31).
func WithCost(cost int) BcryptOption {
	return func(h *BcryptHasher) {
		if cost >= bcrypt.MinCost && cost <= bcrypt.MaxCost {
			h.cost = cost
		}
	}
}

// NewBcryptHasher creates a bcrypt-based password hasher.
func NewBcryptHasher(opts ...BcryptOption) *BcryptHasher {
	h := &BcryptHasher{cost: 12}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *BcryptHasher) Hash(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password: minimum length is 8 characters")
	}
	if len(password) > 72 {
		return "", errors.New("password: maximum length is 72 characters (bcrypt limit)")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("password: hash: %w", err)
	}
	return string(hash), nil
}

func (h *BcryptHasher) Verify(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return errors.New("password: invalid password")
	}
	return nil
}

// --- Argon2id Implementation ---

// Argon2Hasher implements Hasher using argon2id.
type Argon2Hasher struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
	saltLen int
}

// Argon2Option configures the argon2id hasher.
type Argon2Option func(*Argon2Hasher)

// WithArgon2Time sets the number of iterations (default: 1).
func WithArgon2Time(t uint32) Argon2Option {
	return func(h *Argon2Hasher) { h.time = t }
}

// WithArgon2Memory sets the memory usage in KiB (default: 64*1024 = 64MB).
func WithArgon2Memory(m uint32) Argon2Option {
	return func(h *Argon2Hasher) { h.memory = m }
}

// WithArgon2Threads sets the parallelism (default: 4).
func WithArgon2Threads(t uint8) Argon2Option {
	return func(h *Argon2Hasher) { h.threads = t }
}

// NewArgon2Hasher creates an argon2id-based password hasher.
// Defaults follow OWASP recommendations: time=1, memory=64MB, threads=4.
func NewArgon2Hasher(opts ...Argon2Option) *Argon2Hasher {
	h := &Argon2Hasher{
		time:    1,
		memory:  64 * 1024,
		threads: 4,
		keyLen:  32,
		saltLen: 16,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Argon2Hasher) Hash(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password: minimum length is 8 characters")
	}

	salt, err := generateRandomBytes(h.saltLen)
	if err != nil {
		return "", fmt.Errorf("password: generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, h.time, h.memory, h.threads, h.keyLen)

	// Encode as: $argon2id$v=19$m=MEMORY,t=TIME,p=THREADS$SALT$HASH
	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.memory, h.time, h.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

func (h *Argon2Hasher) Verify(password, encodedHash string) error {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return errors.New("password: invalid argon2id hash format")
	}

	var memory, time uint32
	var threads uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads)
	if err != nil {
		return fmt.Errorf("password: parse argon2id params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return fmt.Errorf("password: decode salt: %w", err)
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return fmt.Errorf("password: decode hash: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))

	if subtle.ConstantTimeCompare(hash, expectedHash) != 1 {
		return errors.New("password: invalid password")
	}
	return nil
}
