package util

import "testing"

func TestSecretKeyMatcherDefaults(t *testing.T) {
	t.Parallel()
	m := DefaultSecretKeyMatcher()
	for _, key := range []string{"--token", "--auth-token", "db_password", "CLIENT_SECRET"} {
		if !m.IsSecretKey(key) {
			t.Errorf("expected %q to match", key)
		}
	}
}

func TestSecretKeyMatcherNonSecrets(t *testing.T) {
	t.Parallel()
	m := DefaultSecretKeyMatcher()
	for _, key := range []string{"--author", "tokenizer", "account"} {
		if m.IsSecretKey(key) {
			t.Errorf("did not expect %q to match", key)
		}
	}
}

func TestSecretKeyMatcherCustomNames(t *testing.T) {
	t.Parallel()
	m := DefaultSecretKeyMatcher().WithName("license-key")
	if !m.IsSecretKey("--license-key") || !m.IsSecretKey("vendor_license_key") {
		t.Error("custom name should extend matching")
	}
}

func TestSecretKeyMatcherEmptyKey(t *testing.T) {
	t.Parallel()
	if DefaultSecretKeyMatcher().IsSecretKey("---") {
		t.Error("all-dash key should not match")
	}
}
