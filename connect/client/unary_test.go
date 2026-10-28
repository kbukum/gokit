package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	kitconnect "github.com/kbukum/gokit/connect"
	apperrors "github.com/kbukum/gokit/errors"
)

type stubValidator struct{ err error }

func (v stubValidator) Validate(proto.Message, ...protovalidate.ValidationOption) error { return v.err }

func answer(msg *wrapperspb.StringValue, err error) func(context.Context, *connect.Request[wrapperspb.StringValue]) (*connect.Response[wrapperspb.StringValue], error) {
	return func(context.Context, *connect.Request[wrapperspb.StringValue]) (*connect.Response[wrapperspb.StringValue], error) {
		if err != nil {
			return nil, err
		}
		if msg == nil {
			return nil, nil //nolint:nilnil // a peer that breaks the contract
		}
		return connect.NewResponse(msg), nil
	}
}

// served returns the wire form of a failure the peer answered with, as a client receives it.
func served(t *testing.T, appErr *apperrors.AppError) error {
	t.Helper()
	local, err := kitconnect.ToConnectError(appErr, "peer")
	if err != nil {
		t.Fatal(err)
	}
	wire := connect.NewWireError(local.Code(), errors.New(local.Message()))
	for _, d := range local.Details() {
		wire.AddDetail(d)
	}
	return wire
}

func TestCallFailureKeepsThePeersTypedFailure(t *testing.T) {
	violation := apperrors.Violation{Field: "name", Reason: "REQUIRED", Message: "Name is required"}
	wire := served(t, apperrors.New(apperrors.ErrCodeInvalidInput, "Some fields are invalid").
		WithReason("FIELDS_INVALID").WithViolations(violation).WithRetryable(true).WithRetryAfter(2*time.Second))

	var got *apperrors.AppError
	if !errors.As(CallFailure(context.Background(), "control", wire), &got) {
		t.Fatalf("not an AppError")
	}
	if got.Code != apperrors.ErrCodeInvalidInput || got.Reason != "FIELDS_INVALID" || !got.Retryable || got.RetryAfter != 2*time.Second ||
		len(got.Violations) != 1 || got.Violations[0].Field != "name" || got.Message != "Some fields are invalid" || !errors.Is(got, wire) {
		t.Fatalf("served failure = %+v", got)
	}
}

func TestCallFailureMapsOutagesAndMalformedDetails(t *testing.T) {
	down := connect.NewError(connect.CodeUnavailable, &transportError{cause: errors.New("down")})
	var got *apperrors.AppError
	if !errors.As(CallFailure(context.Background(), "control", down), &got) || got.Reason != ReasonUnavailable || !errors.Is(got, down) {
		t.Fatalf("outage = %+v", got)
	}
	if !errors.As(CallFailure(context.Background(), "control", errors.New("not connect")), &got) || got.Code != apperrors.ErrCodeInternal || got.Retryable {
		t.Fatalf("undecodable = %+v", got)
	}
}

func TestUnaryValidatesTheResponse(t *testing.T) {
	ctx := context.Background()
	out, err := Unary(ctx, "control", stubValidator{}, answer(wrapperspb.String("ok"), nil), &wrapperspb.StringValue{})
	if err != nil || out.GetValue() != "ok" {
		t.Fatalf("valid = %v, %v", out, err)
	}
	invalid := errors.New("value too long")
	for name, tc := range map[string]struct {
		msg *wrapperspb.StringValue
		v   protovalidate.Validator
	}{"missing": {nil, stubValidator{}}, "invalid": {wrapperspb.String("x"), stubValidator{err: invalid}}} {
		out, err := Unary(ctx, "control", tc.v, answer(tc.msg, nil), &wrapperspb.StringValue{})
		var appErr *apperrors.AppError
		if out != nil || !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInternal {
			t.Fatalf("%s = %v, %v; want an Internal contract failure", name, out, err)
		}
	}
	if _, err := Unary(ctx, "control", stubValidator{err: invalid}, answer(wrapperspb.String("x"), nil), &wrapperspb.StringValue{}); !errors.Is(err, invalid) {
		t.Fatalf("validation cause lost: %v", err)
	}
}

func TestUnaryMapsFailuresAndRefusesMissingInputs(t *testing.T) {
	ctx := context.Background()
	wire := served(t, apperrors.NotFound("setup", "s1").WithReason("SETUP_NOT_FOUND"))
	_, err := Unary(ctx, "control", stubValidator{}, answer(nil, wire), &wrapperspb.StringValue{})
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeNotFound || appErr.Reason != "SETUP_NOT_FOUND" {
		t.Fatalf("served = %v", err)
	}
	//nolint:staticcheck // a nil context is the misuse under test
	if _, err := Unary(nil, "control", stubValidator{}, answer(wrapperspb.String("ok"), nil), &wrapperspb.StringValue{}); !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("nil context = %v", err)
	}
	if _, err := Unary(ctx, "control", nil, answer(wrapperspb.String("ok"), nil), &wrapperspb.StringValue{}); !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("nil validator = %v", err)
	}
}
