package apikey

import (
	"context"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestUnknownDigestIsTerminalAuthenticationFailure(t *testing.T) {
	m := NewManager(newMemStore(), testHasher(t))
	_, err := m.ValidateKey(context.Background(), "key.unknown")
	if err == nil || apperrors.Normalize(err).Code != apperrors.ErrCodeUnauthorized {
		t.Fatal("missing digest was not an authentication failure", err)
	}
}
