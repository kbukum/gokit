package connect

import (
	"errors"
	"fmt"
	"testing"

	connectrpc "connectrpc.com/connect"
)

func TestIsTransientError_Nil(t *testing.T) {
	if IsTransientError(nil) {
		t.Fatal("nil error should not be transient")
	}
}

func TestIsTransientError_TransientCodes(t *testing.T) {
	codes := []connectrpc.Code{
		connectrpc.CodeUnavailable,
		connectrpc.CodeResourceExhausted,
		connectrpc.CodeAborted,
	}
	for _, code := range codes {
		err := connectrpc.NewError(code, errors.New("test"))
		if !IsTransientError(err) {
			t.Errorf("expected code %v to be transient", code)
		}
	}
}

func TestIsTransientError_NonTransientCodes(t *testing.T) {
	codes := []connectrpc.Code{
		connectrpc.CodeNotFound,
		connectrpc.CodeInvalidArgument,
		connectrpc.CodePermissionDenied,
		connectrpc.CodeInternal,
	}
	for _, code := range codes {
		err := connectrpc.NewError(code, errors.New("test"))
		if IsTransientError(err) {
			t.Errorf("expected code %v to not be transient", code)
		}
	}
}

func TestIsTransientError_ConnectionErrors(t *testing.T) {
	msgs := []string{
		"connection refused",
		"no such host",
		"i/o timeout",
		"service unavailable",
	}
	for _, msg := range msgs {
		err := errors.New(msg)
		if !IsTransientError(err) {
			t.Errorf("expected %q to be transient", msg)
		}
	}
}

func TestIsTransientError_WrappedConnectError(t *testing.T) {
	inner := connectrpc.NewError(connectrpc.CodeUnavailable, errors.New("down"))
	wrapped := fmt.Errorf("call failed: %w", inner)
	if !IsTransientError(wrapped) {
		t.Fatal("wrapped transient connect error should be transient")
	}
}

func TestIsTransientError_NonTransientPlainError(t *testing.T) {
	err := errors.New("some random error")
	if IsTransientError(err) {
		t.Fatal("random error should not be transient")
	}
}
