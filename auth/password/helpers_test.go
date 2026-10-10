package password

import (
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func newHasher(t *testing.T, cfg Config) Hasher {
	t.Helper()
	h, err := NewHasher(cfg)
	if err != nil {
		t.Fatalf("NewHasher: %v", err)
	}
	return h
}

func requireError(t *testing.T, err error, code apperrors.ErrorCode, reason string) {
	t.Helper()
	app, ok := apperrors.AsAppError(err)
	if !ok || app.Code != code || app.Reason != reason {
		t.Fatalf("error = %v, want %s/%s", err, code, reason)
	}
}
