package oidc_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/oidc"
)

func TestConfig_ApplyDefaults(t *testing.T) {
	t.Parallel()
	c := oidc.Config{}
	c.ApplyDefaults()
	if want := []string{"openid", "email", "profile"}; !equalStrings(c.Scopes, want) {
		t.Errorf("Scopes default: got %v want %v", c.Scopes, want)
	}
	if want := []string{"RS256", "ES256", "EdDSA"}; !equalStrings(c.SupportedSigningAlgs, want) {
		t.Errorf("SupportedSigningAlgs default: got %v want %v", c.SupportedSigningAlgs, want)
	}
	if c.JWKSCacheDuration != time.Hour {
		t.Errorf("JWKSCacheDuration default: got %v want 1h", c.JWKSCacheDuration)
	}
	if c.HTTPTimeout != 10*time.Second {
		t.Errorf("HTTPTimeout default: got %v want 10s", c.HTTPTimeout)
	}
}

func TestConfig_ApplyDefaults_DoesNotOverrideExplicit(t *testing.T) {
	t.Parallel()
	c := oidc.Config{
		Scopes:               []string{"x"},
		SupportedSigningAlgs: []string{"ES256"},
		JWKSCacheDuration:    5 * time.Minute,
		HTTPTimeout:          1 * time.Second,
	}
	c.ApplyDefaults()
	if !equalStrings(c.Scopes, []string{"x"}) {
		t.Errorf("Scopes overridden: %v", c.Scopes)
	}
	if !equalStrings(c.SupportedSigningAlgs, []string{"ES256"}) {
		t.Errorf("Algs overridden: %v", c.SupportedSigningAlgs)
	}
	if c.JWKSCacheDuration != 5*time.Minute {
		t.Errorf("Cache duration overridden")
	}
	if c.HTTPTimeout != 1*time.Second {
		t.Errorf("HTTP timeout overridden")
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		c    oidc.Config
		ok   bool
		msg  string
	}{
		{"disabled-no-issuer-ok", oidc.Config{Enabled: false}, true, ""},
		{"missing-issuer", oidc.Config{Enabled: true, ClientID: "c"}, false, "issuer"},
		{"missing-client-id", oidc.Config{Enabled: true, Issuer: "https://x"}, false, "client_id"},
		{"valid", oidc.Config{Enabled: true, Issuer: "https://x", ClientID: "c"}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.c.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.msg)
				}
				if !strings.Contains(err.Error(), tc.msg) {
					t.Errorf("error %v does not contain %q", err, tc.msg)
				}
			}
		})
	}
}

func TestConfig_ToVerifierConfig(t *testing.T) {
	t.Parallel()
	c := oidc.Config{
		ClientID:             "cid",
		SupportedSigningAlgs: []string{"RS256", "ES256"},
		JWKSCacheDuration:    7 * time.Minute,
		SkipIssuerCheck:      true,
	}
	v := c.ToVerifierConfig()
	if v.ClientID != "cid" {
		t.Errorf("ClientID: got %q", v.ClientID)
	}
	if !equalStrings(v.SupportedSigningAlgs, []string{"RS256", "ES256"}) {
		t.Errorf("Algs: got %v", v.SupportedSigningAlgs)
	}
	if v.JWKSCacheDuration != 7*time.Minute {
		t.Errorf("Cache: got %v", v.JWKSCacheDuration)
	}
	if !v.SkipIssuerCheck {
		t.Errorf("SkipIssuerCheck not propagated")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// state.go: GenerateState / NewPKCE / GenerateNonce
// ─────────────────────────────────────────────────────────────────────────────

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
