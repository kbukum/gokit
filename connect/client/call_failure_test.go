package client

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"connectrpc.com/connect"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestMapCallFailureSeparatesCallerFromPeer(t *testing.T) {
	down := connect.NewError(connect.CodeUnavailable, &transportError{cause: errors.New("down")})
	live := context.Background()
	canceled, cancel := context.WithCancel(live)
	cancel()
	expired, stop := context.WithDeadline(live, time.Unix(0, 0))
	defer stop()

	got, ok := MapCallFailure(live, "control", down)
	if !ok || got.Code != apperrors.ErrCodeServiceUnavailable || got.Reason != ReasonUnavailable || !got.Retryable || !errors.Is(got, down) {
		t.Fatalf("peer outage = %+v, %v", got, ok)
	}
	for name, tc := range map[string]struct {
		ctx  context.Context
		code apperrors.ErrorCode
	}{"deadline": {expired, apperrors.ErrCodeTimeout}, "cancel": {canceled, apperrors.ErrCodeCanceled}} {
		got, ok := MapCallFailure(tc.ctx, "control", connect.NewError(connect.CodeCanceled, context.Canceled))
		if !ok || got.Code != tc.code || !errors.Is(got, context.Cause(tc.ctx)) {
			t.Fatalf("%s = %+v, %v; want %s preserving the caller's cause", name, got, ok, tc.code)
		}
	}
	if got, ok := MapCallFailure(live, "control", connect.NewError(connect.CodePermissionDenied, errors.New("no"))); ok || got != nil {
		t.Fatalf("served error = %+v, %v", got, ok)
	}
	if got, ok := MapCallFailure(live, "control", nil); ok || got != nil {
		t.Fatalf("nil = %+v, %v", got, ok)
	}
	for _, ctx := range []context.Context{live, canceled, expired} {
		if got, ok := MapCallFailure(ctx, "control", io.EOF); ok || got != nil {
			t.Fatalf("clean end = %+v, %v", got, ok)
		}
	}
	transportEOF := &transportError{cause: io.EOF}
	if got, ok := MapCallFailure(live, "control", transportEOF); !ok || got.Reason != ReasonUnavailable || !errors.Is(got, transportEOF) {
		t.Fatalf("transport EOF = %+v, %v", got, ok)
	}
}

// expiringContext reaches its deadline right after the first time its error is read.
type expiringContext struct {
	context.Context
	reads int
}

func (c *expiringContext) Err() error {
	c.reads++
	if c.reads == 1 {
		return nil
	}
	return context.DeadlineExceeded
}

func TestMapCallFailureReadsTheContextOnce(t *testing.T) {
	ctx := &expiringContext{Context: context.Background()}
	got, ok := MapCallFailure(ctx, "control", connect.NewError(connect.CodeUnavailable, &transportError{cause: errors.New("down")}))
	if !ok || got.Code == apperrors.ErrCodeCanceled || ctx.reads != 1 {
		t.Fatalf("got %+v, %v after %d reads; want one consistent read", got, ok, ctx.reads)
	}
}
