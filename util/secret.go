package util

import (
	"crypto/subtle"
	"encoding/json"
)

const secretMask = "***"

// SecretString wraps a string so it does not leak through logging, formatting, or
// JSON serialization. The plaintext is reachable only through Expose, marshals as
// the mask "***", and unmarshals from plaintext so a SecretString can be populated
// from config. Go provides no guaranteed in-memory zeroization, so unlike the rskit
// counterpart the backing bytes are not scrubbed on drop; treat process memory as a
// separate trust boundary.
type SecretString struct {
	value string
}

// NewSecretString wraps plaintext in a SecretString.
func NewSecretString(plaintext string) SecretString {
	return SecretString{value: plaintext}
}

// Expose returns the plaintext value. Call it only where the secret is genuinely needed.
func (s SecretString) Expose() string { return s.value }

// IsEmpty reports whether the underlying value is empty.
func (s SecretString) IsEmpty() bool { return s.value == "" }

// Len returns the length of the underlying value in bytes.
func (s SecretString) Len() int { return len(s.value) }

// Equal compares two secrets in constant time, avoiding a timing side channel.
func (s SecretString) Equal(other SecretString) bool {
	return ConstantTimeEqual([]byte(s.value), []byte(other.value))
}

// ConstantTimeEqual reports whether two byte slices are equal, comparing in constant
// time to avoid a timing side channel. Length is not secret: a length mismatch returns
// false quickly, so prefer fixed-length encodings for sensitive tokens.
func ConstantTimeEqual(left, right []byte) bool {
	return subtle.ConstantTimeCompare(left, right) == 1
}

// String renders the mask for a non-empty secret and an empty string otherwise, so
// a SecretString is safe to interpolate into logs.
func (s SecretString) String() string {
	if s.value == "" {
		return ""
	}
	return secretMask
}

// GoString masks the value in %#v / Go-syntax formatting.
func (s SecretString) GoString() string { return "SecretString(***)" }

// MarshalJSON emits the mask for a non-empty secret and an empty string otherwise,
// so the plaintext never reaches a serialized config dump.
func (s SecretString) MarshalJSON() ([]byte, error) {
	if s.value == "" {
		return json.Marshal("")
	}
	return json.Marshal(secretMask)
}

// UnmarshalJSON reads a plaintext string into the secret.
func (s *SecretString) UnmarshalJSON(data []byte) error {
	var plaintext string
	if err := json.Unmarshal(data, &plaintext); err != nil {
		return err
	}
	s.value = plaintext
	return nil
}
