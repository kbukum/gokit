package testutil

import (
	"net/http"
	"sync/atomic"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/sse"
)

var _ sse.Authenticator[string] = (*FakeAuthenticator[string])(nil)

// FakeAuthenticator is a deterministic [sse.Authenticator] for tests. It returns
// a fixed identity or a fixed rejection error and counts how many times it ran,
// so tests can assert the gate was exercised. It is safe for concurrent use.
type FakeAuthenticator[T any] struct {
	// Identity is returned on success (when Err is nil).
	Identity T
	// Err, when non-nil, rejects every connection. Use an [apperrors.Forbidden]
	// error to exercise the 403 path; any other error maps to 401.
	Err error

	calls atomic.Int64
}

// Authenticate implements [sse.Authenticator].
func (f *FakeAuthenticator[T]) Authenticate(_ *http.Request) (T, error) {
	f.calls.Add(1)
	if f.Err != nil {
		var zero T
		return zero, f.Err
	}
	return f.Identity, nil
}

// Calls reports how many times Authenticate was invoked.
func (f *FakeAuthenticator[T]) Calls() int { return int(f.calls.Load()) }

// AllowAuthenticator returns a [FakeAuthenticator] that admits every connection
// with the given identity.
func AllowAuthenticator[T any](identity T) *FakeAuthenticator[T] {
	return &FakeAuthenticator[T]{Identity: identity}
}

// RejectUnauthorized returns a [FakeAuthenticator] that rejects every connection
// with a 401 (missing/invalid credential).
func RejectUnauthorized[T any](reason string) *FakeAuthenticator[T] {
	return &FakeAuthenticator[T]{Err: apperrors.Unauthorized(reason)}
}

// RejectForbidden returns a [FakeAuthenticator] that rejects every connection
// with a 403 (authenticated but not permitted).
func RejectForbidden[T any](reason string) *FakeAuthenticator[T] {
	return &FakeAuthenticator[T]{Err: apperrors.Forbidden(reason)}
}
