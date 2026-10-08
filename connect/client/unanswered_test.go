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
