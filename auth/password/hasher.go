package password

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	apperrors "github.com/kbukum/gokit/errors"
)

// Error reasons. Mismatch is UNAUTHORIZED, a corrupt stored hash is INTERNAL and a length violation is
// INVALID_INPUT, so callers never confuse bad credentials with bad data.
const (
	ReasonMismatch        = "PASSWORD_MISMATCH"
	ReasonCorruptHash     = "PASSWORD_HASH_INVALID"
	ReasonTooShort        = "PASSWORD_TOO_SHORT"
	ReasonTooLong         = "PASSWORD_TOO_LONG"
	ReasonUnsupportedCost = "PASSWORD_COST_UNSUPPORTED"
)

const (
	argon2KeyLen  = 32
	argon2SaltLen = 16
	minEncodedLen = 16
	maxEncodedLen = 64
)

// Hasher hashes and verifies passwords. Use [NewHasher] to construct one, and [Pool] to bound concurrent work.
type Hasher interface {
	// Hash enforces the configured length policy and returns an encoded, salted hash.
	Hash(password string) (string, error)

	// Verify returns nil when password matches hash, an UNAUTHORIZED error with ReasonMismatch when it does not,
	// and an INTERNAL error with ReasonCorruptHash when hash cannot be decoded.
	Verify(password, hash string) error
}

func mismatch() error {
	return apperrors.New(apperrors.ErrCodeUnauthorized, "Invalid password").WithReason(ReasonMismatch)
}

func corrupt(cause error) error {
	return apperrors.Internal(cause).WithReason(ReasonCorruptHash)
}

type lengthPolicy struct{ min, max int }

// admit checks a new password's length in characters.
func (p lengthPolicy) admit(password string) error {
	n := utf8.RuneCountInString(password)
	switch {
	case n < p.min:
		return apperrors.InvalidInput("password", fmt.Sprintf("Password must be at least %d characters", p.min)).WithReason(ReasonTooShort)
	case n > p.max:
		return apperrors.InvalidInput("password", fmt.Sprintf("Password must be at most %d characters", p.max)).WithReason(ReasonTooLong)
	}
	return nil
}

// oversized reports a presented password that can never match, so it is rejected without hashing.
func (p lengthPolicy) oversized(password string) bool {
	return len(password) > utf8.UTFMax*p.max || utf8.RuneCountInString(password) > p.max
}

type bcryptHasher struct {
	lengthPolicy
	cost   int
	limits VerifyLimits
}

func (h *bcryptHasher) Hash(password string) (string, error) {
	if err := h.admit(password); err != nil {
		return "", err
	}
	if len(password) > bcryptMaxBytes {
		return "", apperrors.InvalidInput("password", "Password must be at most 72 bytes for bcrypt").WithReason(ReasonTooLong)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", apperrors.Internal(err)
	}
	return string(hash), nil
}

func (h *bcryptHasher) Verify(password, hash string) error {
	if err := validateBcryptEncoding(hash, h.limits.EncodedBytes); err != nil {
		return corrupt(err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		return corrupt(err)
	}
	if cost > h.limits.BcryptCost {
		return unsupportedCost()
	}
	if h.oversized(password) || len(password) > bcryptMaxBytes {
		return mismatch()
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return mismatch()
		}
		return corrupt(err)
	}
	return nil
}

type argon2Hasher struct {
	lengthPolicy
	time    uint32
	memory  uint32
	threads uint8
	limits  VerifyLimits
}

func (h *argon2Hasher) Hash(password string) (string, error) {
	if err := h.admit(password); err != nil {
		return "", err
	}
	salt, err := generateRandomBytes(argon2SaltLen)
	if err != nil {
		return "", apperrors.Internal(err)
	}
	key := argon2.IDKey([]byte(password), salt, h.time, h.memory, h.threads, argon2KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$%s$%s$%s", argon2.Version, argon2Params(h.memory, h.time, h.threads),
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func argon2Params(memory, time uint32, threads uint8) string {
	return fmt.Sprintf("m=%d,t=%d,p=%d", memory, time, threads)
}

func (h *argon2Hasher) Verify(password, encoded string) error {
	if len(encoded) > h.limits.EncodedBytes {
		return corrupt(fmt.Errorf("password: encoded hash too long"))
	}
	parsed, err := parseArgon2(encoded)
	if err != nil {
		return corrupt(err)
	}
	if parsed.memory > h.limits.Argon2Memory || parsed.time > h.limits.Argon2Time || parsed.threads > h.limits.Argon2Threads {
		return unsupportedCost()
	}
	if h.oversized(password) {
		return mismatch()
	}
	key := argon2.IDKey([]byte(password), parsed.salt, parsed.time, parsed.memory, parsed.threads, uint32(len(parsed.key)))
	if subtle.ConstantTimeCompare(key, parsed.key) != 1 {
		return mismatch()
	}
	return nil
}

type argon2Hash struct {
	memory, time uint32
	threads      uint8
	salt, key    []byte
}

// parseArgon2 decodes a canonical PHC-format hash and bounds its cost, so a corrupt or hostile stored hash cannot
// demand unbounded memory or time.
func parseArgon2(encoded string) (argon2Hash, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return argon2Hash{}, fmt.Errorf("password: not an argon2id v%d hash", argon2.Version)
	}
	var h argon2Hash
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &h.memory, &h.time, &h.threads); err != nil || argon2Params(h.memory, h.time, h.threads) != parts[3] {
		return argon2Hash{}, fmt.Errorf("password: malformed argon2id parameters")
	}
	if h.memory < 1 || h.time < 1 || h.threads < 1 {
		return argon2Hash{}, fmt.Errorf("password: argon2id parameters out of bounds")
	}
	var err error
	if h.salt, err = decodeField(parts[4]); err != nil {
		return argon2Hash{}, fmt.Errorf("password: salt: %w", err)
	}
	if h.key, err = decodeField(parts[5]); err != nil {
		return argon2Hash{}, fmt.Errorf("password: key: %w", err)
	}
	return h, nil
}

func decodeField(field string) ([]byte, error) {
	if len(field) > base64.RawStdEncoding.EncodedLen(maxEncodedLen) {
		return nil, fmt.Errorf("too long")
	}
	value, err := base64.RawStdEncoding.Strict().DecodeString(field)
	if err != nil {
		return nil, err
	}
	if len(value) < minEncodedLen {
		return nil, fmt.Errorf("too short")
	}
	return value, nil
}
