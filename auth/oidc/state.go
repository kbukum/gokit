package oidc

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

// GenerateState creates a cryptographically secure random state string
// for CSRF protection in OAuth2 flows.
// Returns a 32-byte hex-encoded string (64 characters).
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// PKCE holds a PKCE (Proof Key for Code Exchange) challenge/verifier pair.
// Use NewPKCE to generate a pair, then:
//   - Send CodeChallenge + CodeChallengeMethod in the authorization URL
//   - Send CodeVerifier in the token exchange
type PKCE struct {
	// CodeVerifier is the random secret (kept by the client, sent during exchange).
	CodeVerifier string

	// CodeChallenge is the SHA-256 hash of the verifier (sent in the auth URL).
	CodeChallenge string

	// CodeChallengeMethod is always "S256".
	CodeChallengeMethod string
}

// NewPKCE generates a new PKCE challenge/verifier pair using S256 method.
// The verifier is a 32-byte random value, base64url-encoded (43 characters).
func NewPKCE() (*PKCE, error) {
	verifier := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, verifier); err != nil {
		return nil, err
	}

	verifierStr := base64.RawURLEncoding.EncodeToString(verifier)
	h := sha256.Sum256([]byte(verifierStr))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	return &PKCE{
		CodeVerifier:        verifierStr,
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
	}, nil
}

// GenerateNonce creates a cryptographically secure random nonce
// for OIDC replay protection.
// Returns a 16-byte hex-encoded string (32 characters).
func GenerateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ValidateState rejects missing or mismatched OAuth state values.
func ValidateState(expected, actual string) error {
	return validateSecretMatch("state", expected, actual)
}

// ValidateNonce rejects missing or mismatched OIDC nonce values.
func ValidateNonce(expected, actual string) error {
	return validateSecretMatch("nonce", expected, actual)
}

// ValidatePKCE rejects missing PKCE verifier/challenge data for public clients.
func ValidatePKCE(pkce *PKCE) error {
	if pkce == nil {
		return errors.New("oidc: PKCE is required")
	}
	if pkce.CodeVerifier == "" || pkce.CodeChallenge == "" || pkce.CodeChallengeMethod != "S256" {
		return errors.New("oidc: invalid PKCE parameters")
	}
	return nil
}

func validateSecretMatch(label, expected, actual string) error {
	if expected == "" || actual == "" {
		return errors.New("oidc: missing " + label)
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return errors.New("oidc: " + label + " mismatch")
	}
	return nil
}
