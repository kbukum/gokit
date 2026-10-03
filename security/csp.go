package security

import (
	"crypto/rand"
	"slices"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
)

const defaultNonceCSP = "default-src 'self'; script-src 'nonce-{nonce}' 'strict-dynamic'; style-src 'self' 'nonce-{nonce}'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// NonceCSP issues a fresh nonce and a matching strict policy for each HTML response. Templates use the literal {nonce} marker.
type NonceCSP struct{ policy string }

// NewNonceCSP validates a policy template. Empty selects secure same-origin defaults.
func NewNonceCSP(policy string) (*NonceCSP, error) {
	if policy == "" {
		policy = defaultNonceCSP
	}
	invalid := func() (*NonceCSP, error) {
		return nil, apperrors.InvalidInput("csp", "CSP requires nonce-only scripts, restrictive base/object/frame directives, and no unsafe sources")
	}
	if strings.ContainsAny(policy, "\r\n") || strings.Contains(strings.ToLower(policy), "unsafe-") || strings.Contains(policy, "*") {
		return invalid()
	}
	for _, char := range policy {
		if char != '\t' && (char < ' ' || char > '~') {
			return invalid()
		}
	}
	directives := make(map[string][]string)
	for _, directive := range strings.Split(policy, ";") {
		fields := strings.Fields(directive)
		if len(fields) == 0 {
			continue
		}
		fields[0] = strings.ToLower(fields[0])
		if _, duplicate := directives[fields[0]]; duplicate {
			return invalid()
		}
		directives[fields[0]] = fields[1:]
	}
	for _, name := range []string{"object-src", "frame-ancestors"} {
		if !slices.Equal(directives[name], []string{"'none'"}) {
			return invalid()
		}
	}
	if base := directives["base-uri"]; !slices.Equal(base, []string{"'self'"}) && !slices.Equal(base, []string{"'none'"}) {
		return invalid()
	}
	scripts := directives["script-src"]
	if !slices.Contains(scripts, "'nonce-{nonce}'") || !slices.Contains(scripts, "'strict-dynamic'") {
		return invalid()
	}
	for _, source := range scripts {
		if source != "'nonce-{nonce}'" && source != "'strict-dynamic'" {
			return invalid()
		}
	}
	for _, name := range []string{"script-src-elem", "script-src-attr"} {
		if sources, exists := directives[name]; exists && !slices.Equal(sources, []string{"'none'"}) {
			return invalid()
		}
	}
	return &NonceCSP{policy: policy}, nil
}

// Issue uses the system cryptographic random source, never a shared or cached nonce.
func (c *NonceCSP) Issue() (nonce, header string) {
	nonce = rand.Text()
	return nonce, strings.ReplaceAll(c.policy, "{nonce}", nonce)
}
