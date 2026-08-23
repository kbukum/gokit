package security

import "testing"

func TestAuthSchemeConstants(t *testing.T) {
	t.Parallel()
	if BasicAuthScheme != "Basic" {
		t.Errorf("BasicAuthScheme = %q, want Basic", BasicAuthScheme)
	}
	if BearerAuthScheme != "Bearer" {
		t.Errorf("BearerAuthScheme = %q, want Bearer", BearerAuthScheme)
	}
}
