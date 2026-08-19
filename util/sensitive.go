package util

import "strings"

// DefaultSecretKeyNames are the key names SecretKeyMatcher treats as secret-bearing
// by default. They cover the common credential and token spellings seen in config
// keys and CLI flags.
var DefaultSecretKeyNames = []string{
	"password",
	"passwd",
	"pwd",
	"token",
	"secret",
	"credential",
	"credentials",
	"apikey",
	"api_key",
	"auth",
	"authorization",
	"auth_token",
	"access_token",
	"refresh_token",
	"client_secret",
}

// SecretKeyMatcher decides whether a config key or flag name commonly carries a
// secret value, so callers can redact it. Names are normalized (leading dashes
// trimmed, '-' folded to '_', lowercased) and a key matches when it equals a
// configured name or ends with "_<name>" — so "db_password" matches while "author"
// does not.
type SecretKeyMatcher struct {
	names []string
}

// NewSecretKeyMatcher builds a matcher from the given secret-bearing names, applying
// the same normalization used at match time and dropping empties.
func NewSecretKeyMatcher(names []string) SecretKeyMatcher {
	m := SecretKeyMatcher{}
	for _, name := range names {
		if normalized, ok := normalizeSecretName(name); ok {
			m.names = appendUniqueString(m.names, normalized)
		}
	}
	return m
}

// DefaultSecretKeyMatcher returns a matcher seeded with DefaultSecretKeyNames.
func DefaultSecretKeyMatcher() SecretKeyMatcher {
	return NewSecretKeyMatcher(DefaultSecretKeyNames)
}

// WithName returns a matcher extended with one more secret-bearing name.
func (m SecretKeyMatcher) WithName(name string) SecretKeyMatcher {
	if normalized, ok := normalizeSecretName(name); ok {
		out := make([]string, len(m.names))
		copy(out, m.names)
		return SecretKeyMatcher{names: appendUniqueString(out, normalized)}
	}
	return m
}

// WithNames returns a matcher extended with several secret-bearing names.
func (m SecretKeyMatcher) WithNames(names []string) SecretKeyMatcher {
	out := m
	for _, name := range names {
		out = out.WithName(name)
	}
	return out
}

// IsSecretKey reports whether name should be treated as secret-bearing.
func (m SecretKeyMatcher) IsSecretKey(name string) bool {
	normalized, ok := normalizeSecretName(name)
	if !ok {
		return false
	}
	for _, secret := range m.names {
		if normalized == secret || hasSecretSuffix(normalized, secret) {
			return true
		}
	}
	return false
}

func hasSecretSuffix(normalized, secret string) bool {
	prefix, found := strings.CutSuffix(normalized, secret)
	return found && strings.HasSuffix(prefix, "_")
}

func normalizeSecretName(name string) (string, bool) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimLeft(name, "-"), "-", "_"))
	if normalized == "" {
		return "", false
	}
	return normalized, true
}

func appendUniqueString(existing []string, value string) []string {
	for _, e := range existing {
		if e == value {
			return existing
		}
	}
	return append(existing, value)
}
