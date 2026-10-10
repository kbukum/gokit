package connect

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"

	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
	"github.com/kbukum/gokit/logging"
)

func TestRetryDelayPreservesMinimumAndDecodeFailures(t *testing.T) {
	t.Parallel()
	wire, err := ToConnectError(apperrors.ServiceUnavailable("peer").WithRetryAfter(5*time.Second), "test.Service")
	if err != nil {
		t.Fatal(err)
	}
	if delay, err := RetryDelay(wire); err != nil || delay != 5*time.Second {
		t.Fatalf("minimum delay = %v, %v", delay, err)
	}
	if delay, err := RetryDelay(nil); err != nil || delay != 0 {
		t.Fatalf("absent error = %v, %v", delay, err)
	}
	cause := errors.New("not an RPC error")
	delay, err := RetryDelay(cause)
	var decodeErr *errorrpc.DecodeError
	if delay != 0 || !errors.As(err, &decodeErr) || !errors.Is(err, cause) {
		t.Fatalf("invalid error = %v, %v", delay, err)
	}
}

type normalizationStream struct {
	connectrpc.StreamingHandlerConn
}

func (normalizationStream) Spec() connectrpc.Spec {
	return connectrpc.Spec{Procedure: "/test.Service/Stream"}
}

func TestNormalizingInterceptorStreamingContracts(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "error", Format: "json"}, "rpc-test", logging.WithWriter(&logs))
	if err != nil {
		t.Fatal(err)
	}
	interceptor := NormalizingInterceptor(log)
	failure := apperrors.Internal(errors.New("private backend failure")).WithTraceID("trace-123")
	call := interceptor.WrapStreamingHandler(func(context.Context, connectrpc.StreamingHandlerConn) error { return failure })
	got := call(t.Context(), normalizationStream{})
	if connectrpc.CodeOf(got) != connectrpc.CodeInternal || strings.Contains(got.Error(), "private") {
		t.Fatalf("stream normalization = %v", got)
	}
	if !strings.Contains(logs.String(), "private backend failure") || !strings.Contains(logs.String(), "trace-123") || !strings.Contains(logs.String(), "/test.Service/Stream") {
		t.Fatalf("private diagnostic missing from injected logger: %s", &logs)
	}
	nextCalled := false
	client := interceptor.WrapStreamingClient(func(context.Context, connectrpc.Spec) connectrpc.StreamingClientConn {
		nextCalled = true
		return nil
	})
	if client(t.Context(), connectrpc.Spec{}) != nil || !nextCalled {
		t.Fatal("server normalization changed the client path")
	}
	call = interceptor.WrapStreamingHandler(func(context.Context, connectrpc.StreamingHandlerConn) error {
		return apperrors.New("INVALID_CODE", "private invalid failure")
	})
	if got := call(t.Context(), normalizationStream{}); connectrpc.CodeOf(got) != connectrpc.CodeInternal || strings.Contains(got.Error(), "private") {
		t.Fatalf("invalid application error = %v", got)
	}
}

// safeFirst mirrors a storage boundary: a cause-free classification unwraps before a private cause.
type safeFirst struct{ private error }

func (safeFirst) Error() string { return "DATABASE_FAILURE: Database operation failed" }
func (e safeFirst) Unwrap() []error {
	return []error{apperrors.New(apperrors.ErrCodeInternal, "Database operation failed").WithReason("DATABASE_FAILURE"), e.private}
}

func TestNormalizingInterceptorHonorsSafeBoundaryClassification(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "rpc-test", logging.WithWriter(&logs))
	if err != nil {
		t.Fatal(err)
	}
	private := apperrors.InvalidInput("dsn", "private credential detail").WithCause(errors.New("private driver text"))
	got := NormalizingInterceptor(log).WrapUnary(unaryHandler(nil, safeFirst{private}))
	_, err = got(t.Context(), newProtoRequest())
	if connectrpc.CodeOf(err) != connectrpc.CodeInternal || strings.Contains(err.Error(), "private") || strings.Contains(logs.String(), "private") {
		t.Fatalf("private classification crossed the boundary: %v; logs %s", err, &logs)
	}
}
