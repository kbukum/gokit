package client

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/types/known/wrapperspb"

	kitconnect "github.com/kbukum/gokit/connect"
	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
)

func peerError(t *testing.T, failure error) error {
	t.Helper()
	const path = "/test.Service/Call"
	h := connect.NewUnaryHandler(path, func(context.Context, *connect.Request[wrapperspb.StringValue]) (*connect.Response[wrapperspb.StringValue], error) {
		return nil, failure
	})
	s := h2cServer(t, h.ServeHTTP)
	c := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](h2cClient(t), s.URL+path)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := c.CallUnary(ctx, connect.NewRequest(wrapperspb.String("request")))
	if !connect.IsWireError(err) {
		t.Fatalf("expected a wire error, got %v", err)
	}
	return err
}

func TestAvailabilityIgnoresLocalValidationWithoutTraffic(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{
		errors.New("local validation"),
		connect.NewError(connect.CodeInvalidArgument, errors.New("local validation")),
		connect.NewError(connect.CodeUnavailable, errors.New("local circuit rejection")),
		connect.NewError(connect.CodeDeadlineExceeded, errors.New("local budget rejection")),
		connect.NewError(connect.CodeCanceled, errors.New("local policy rejection")),
	} {
		var calls atomic.Int32
		a := NewAvailability("peer")
		a.Observe(t.Context(), &transportError{cause: errors.New("down")})
		validate := connect.UnaryInterceptorFunc(func(connect.UnaryFunc) connect.UnaryFunc {
			return func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
				return nil, failure
			}
		})
		next := func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
			calls.Add(1)
			return connect.NewResponse(wrapperspb.String("ok")), nil
		}
		_, err := a.WrapUnary(validate.WrapUnary(next))(t.Context(), connect.NewRequest(wrapperspb.String("request")))
		if !errors.Is(err, failure) || calls.Load() != 0 {
			t.Fatalf("local rejection = %v, calls = %d", err, calls.Load())
		}
		assertState(t, a, AvailabilityUnavailable)
		a.Observe(t.Context(), nil)
		if _, err := a.WrapUnary(validate.WrapUnary(next))(t.Context(), connect.NewRequest(wrapperspb.String("request"))); !errors.Is(err, failure) {
			t.Fatalf("local rejection changed: %v", err)
		}
		assertState(t, a, AvailabilityAvailable)
	}
}

func TestMapCallFailurePreservesPeerRetryPolicy(t *testing.T) {
	t.Parallel()
	for name, failure := range map[string]*apperrors.AppError{
		"minimum delay": apperrors.ServiceUnavailable("private").WithRetryAfter(5 * time.Second),
		"no retry":      apperrors.ServiceUnavailable("private").WithRetryable(false),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			wire, err := kitconnect.ToConnectError(failure, "peer")
			if err != nil {
				t.Fatal(err)
			}
			received := peerError(t, wire)
			got, ok := MapCallFailure(t.Context(), "peer", received)
			if !ok || got.Retryable != failure.Retryable || got.RetryAfter != failure.RetryAfter || !errors.Is(got, received) {
				t.Fatalf("mapped = %+v, %v; want retryable=%v delay=%v", got, ok, failure.Retryable, failure.RetryAfter)
			}
			if got.Message == failure.Message {
				t.Fatal("remote message became a public application message")
			}
		})
	}
}

func TestMapCallFailureRejectsMalformedRetryMetadata(t *testing.T) {
	t.Parallel()
	wire := connect.NewError(connect.CodeUnavailable, errors.New("down"))
	detail, err := connect.NewErrorDetail(&errdetails.ErrorInfo{Domain: errorrpc.Domain, Reason: string(apperrors.ErrCodeServiceUnavailable), Metadata: map[string]string{"retryable": "invalid"}})
	if err != nil {
		t.Fatal(err)
	}
	wire.AddDetail(detail)
	received := peerError(t, wire)
	got, ok := MapCallFailure(t.Context(), "peer", received)
	var decodeErr *errorrpc.DecodeError
	if !ok || got.Code != apperrors.ErrCodeInternal || got.Retryable || !errors.As(got, &decodeErr) || !errors.Is(got, received) {
		t.Fatalf("malformed peer failure = %+v, %v", got, ok)
	}
}

func TestMapCallFailureTreatsADueDeadlineAsTheCallers(t *testing.T) {
	t.Parallel()
	enforced := peerError(t, connect.NewError(connect.CodeDeadlineExceeded, errors.New("propagated deadline")))
	ctx, cancel := context.WithTimeout(context.Background(), deadlinePrecision/2)
	defer cancel()
	mapped, ok := MapCallFailure(ctx, "echo", enforced)
	if !ok || mapped.Code != apperrors.ErrCodeTimeout || !errors.Is(mapped, context.DeadlineExceeded) {
		t.Fatalf("mapped = %v, %v; want the caller's timeout", mapped, ok)
	}
	distant, cancelDistant := context.WithTimeout(context.Background(), time.Minute)
	defer cancelDistant()
	if mapped, ok := MapCallFailure(distant, "echo", enforced); !ok || mapped.Reason != ReasonUnavailable {
		t.Fatalf("mapped = %v, %v; want a peer outage", mapped, ok)
	}
}
