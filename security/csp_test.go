package security

import (
	"strings"
	"testing"
)

func TestNonceCSP(t *testing.T) {
	t.Parallel()
	policy, err := NewNonceCSP("")
	if err != nil {
		t.Fatal(err)
	}
	first, header := policy.Issue()
	second, _ := policy.Issue()
	if first == second || len(first) < 26 || !strings.Contains(header, "'nonce-"+first+"'") || strings.Contains(header, "{nonce}") {
		t.Fatalf("invalid nonce policy: %q", header)
	}
	for _, invalid := range []string{
		"default-src *",
		"default-src 'self'; script-src 'unsafe-inline'",
		"default-src 'self'; script-src 'unsafe-eval'",
		"default-src 'none'\r\nInjected: true",
		defaultNonceCSP + "; SCRIPT-SRC-ELEM https:",
		defaultNonceCSP + "; ObJeCt-SrC https:",
		defaultNonceCSP + "; style-src-elem 'UNSAFE-INLINE'",
		strings.Replace(defaultNonceCSP, "script-src ", "script-src\u00a0", 1),
		strings.Replace(defaultNonceCSP, "script-src ", "script-src\v", 1),
	} {
		if _, err := NewNonceCSP(invalid); err == nil {
			t.Errorf("accepted unsafe CSP %q", invalid)
		}
	}
}
