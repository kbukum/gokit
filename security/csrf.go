package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"
	"sync"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// SignedCSRF signs nonce-bearing synchronizer tokens bound to a credential generation.
// Its secret must be independent from session credential protection.
type SignedCSRF struct {
	secret []byte
	random io.Reader
	mu     sync.Mutex
}

// NewSignedCSRF requires an injected cryptographic reader and at least 32 secret bytes.
func NewSignedCSRF(secret []byte, random io.Reader) (*SignedCSRF, error) {
	if len(secret) < 32 || util.IsNil(random) {
		return nil, apperrors.InvalidInput("csrf", "A strong CSRF secret and random source are required")
	}
	return &SignedCSRF{secret: append([]byte(nil), secret...), random: random}, nil
}

// Issue creates a new token bound to the protected generation reference.
func (s *SignedCSRF) Issue(generation string) (string, error) {
	if generation == "" {
		return "", csrfFailure()
	}
	var nonce [32]byte
	s.mu.Lock()
	_, err := io.ReadFull(s.random, nonce[:])
	s.mu.Unlock()
	if err != nil {
		return "", apperrors.Internal(err)
	}
	body := base64.RawURLEncoding.EncodeToString(nonce[:])
	return body + "." + s.signature(generation, body), nil
}

// Verify rejects noncanonical, oversized, altered or cross-generation tokens.
func (s *SignedCSRF) Verify(generation, token string) error {
	if generation == "" || len(token) > 256 {
		return csrfFailure()
	}
	body, signature, ok := strings.Cut(token, ".")
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(body)
	if !ok || err != nil || len(nonce) != 32 || len(signature) != 43 || len(body) != 43 {
		return csrfFailure()
	}
	if !hmac.Equal([]byte(signature), []byte(s.signature(generation, body))) {
		return csrfFailure()
	}
	return nil
}

func (s *SignedCSRF) signature(generation, nonce string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("gokit.csrf.v1\x00" + generation + "\x00" + nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func csrfFailure() error {
	return apperrors.New(apperrors.ErrCodeForbidden, "CSRF verification failed").WithReason("CSRF_INVALID")
}
