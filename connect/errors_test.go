package connect

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"
	"go.opentelemetry.io/otel/trace"

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
	if strings.Contains(logs.String(), "private backend failure") || !strings.Contains(logs.String(), "trace-123") || !strings.Contains(logs.String(), "/test.Service/Stream") || !strings.Contains(logs.String(), "INTERNAL_ERROR") {
		t.Fatalf("safe diagnostic or privacy contract failed: %s", &logs)
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

func TestToConnectErrorContextAttachesTheActiveTraceID(t *testing.T) {
	t.Parallel()
	traceID := trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}, TraceFlags: trace.FlagsSampled,
	}))
	wire, err := ToConnectErrorContext(ctx, errors.New("private cause"), "test.Service")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeError(wire)
	if err != nil || decoded.TraceID != traceID.String() || decoded.Code != apperrors.ErrCodeInternal || strings.Contains(wire.Message(), "private cause") {
		t.Fatalf("decoded = %+v, %v", decoded, err)
	}
	untraced, err := ToConnectErrorContext(context.Background(), apperrors.Unauthorized("Missing key"), "test.Service")
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeError(untraced); err != nil || decoded.TraceID != "" || decoded.Code != apperrors.ErrCodeUnauthorized {
		t.Fatalf("untraced = %+v, %v", decoded, err)
	}
	if wire, err := ToConnectErrorContext(ctx, nil, "test.Service"); err != nil || wire.Code() != connectrpc.CodeInternal {
		t.Fatalf("nil error = %v, %v", wire, err)
	}
}
