package config

import "encoding/json"

const secretMask = "***"

// SecretString wraps a string value and masks it in String(), MarshalJSON(), and MarshalText() to prevent accidental exposure in logs, configs, or serialized output. Use Value() to retrieve the actual plaintext.
type SecretString struct {
	value string
}

// NewSecretString creates a SecretString from a plaintext value.
func NewSecretString(value string) SecretString {
	return SecretString{value: value}
}

// Value returns the unmasked plaintext value.
func (s SecretString) Value() string {
	return s.value
}

// IsEmpty returns true if the underlying value is empty.
func (s SecretString) IsEmpty() bool {
	return s.value == ""
}

// String returns a masked representation, safe for logging.
func (s SecretString) String() string {
	if s.value == "" {
		return ""
	}
	return secretMask
}

// GoString returns a masked representation for %#v formatting.
func (s SecretString) GoString() string {
	return "SecretString{***}"
}

// MarshalJSON serializes the secret as a masked string.
func (s SecretString) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON deserializes a JSON string into the secret value.
func (s *SecretString) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	s.value = v
	return nil
}

// MarshalText serializes the secret as a masked string for text-based formats.
func (s SecretString) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText deserializes a text value into the secret.
func (s *SecretString) UnmarshalText(data []byte) error {
	s.value = string(data)
	return nil
}
