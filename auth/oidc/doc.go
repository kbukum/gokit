// Package oidc provides OpenID Connect building blocks for authentication.
//
// It defines a Provider interface that projects implement per-provider,
// an OIDC token Verifier backed by auto-discovery and JWKS key rotation,
// and utilities for OAuth2 state management and PKCE.
//
// This package wraps coreos/go-oidc and golang.org/x/oauth2 to provide
// a standardized, reusable layer without enforcing specific providers,
// callback patterns, or account linking logic.
//
// Usage:
//
//	// Create a verifier for any OIDC-compliant issuer
//	v, err := oidc.NewVerifier(ctx, "https://accounts.google.com", oidc.VerifierConfig{
//	    ClientID: "my-client-id",
//	})
//	idToken, err := v.Verify(ctx, rawIDToken)
//
//	// Generate secure state + PKCE for OAuth2 flows
//	state, err := oidc.GenerateState()
//	pkce, err := oidc.NewPKCE()
package oidc
