package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestUnansweredSeparatesCallerFromPeer(t *testing.T) {
	down := connect.NewError(connect.CodeUnavailable, errors.New("down"))
	live := context.Background()
	canceled, cancel := context.WithCancel(live)
	cancel()
	expired, stop := context.WithDeadline(live, time.Unix(0, 0))
	defer stop()

	got, ok := Unanswered(live, "control", down)
	if !ok || got.Code != apperrors.ErrCodeServiceUnavailable || got.Reason != ReasonUnavailable || !got.Retryable || !errors.Is(got, down) {
		t.Fatalf("peer outage = %+v, %v", got, ok)
	}
	for name, tc := range map[string]struct {
		ctx  context.Context
		code apperrors.ErrorCode
	}{"deadline": {expired, apperrors.ErrCodeTimeout}, "cancel": {canceled, apperrors.ErrCodeCanceled}} {
		got, ok := Unanswered(tc.ctx, "control", connect.NewError(connect.CodeCanceled, context.Canceled))
		if !ok || got.Code != tc.code || got.Cause != nil {
			t.Fatalf("%s = %+v, %v; want %s without a cause", name, got, ok, tc.code)
		}
	}
	if got, ok := Unanswered(live, "control", connect.NewError(connect.CodePermissionDenied, errors.New("no"))); ok || got != nil {
		t.Fatalf("served error = %+v, %v", got, ok)
	}
	if got, ok := Unanswered(live, "control", nil); ok || got != nil {
		t.Fatalf("nil = %+v, %v", got, ok)
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

func TestUnansweredReadsTheContextOnce(t *testing.T) {
	ctx := &expiringContext{Context: context.Background()}
	got, ok := Unanswered(ctx, "control", connect.NewError(connect.CodeUnavailable, errors.New("down")))
	if !ok || got.Code == apperrors.ErrCodeCanceled || ctx.reads != 1 {
		t.Fatalf("got %+v, %v after %d reads; want one consistent read", got, ok, ctx.reads)
	}
}
