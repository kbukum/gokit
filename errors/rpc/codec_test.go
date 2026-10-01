package rpc

import (
	"slices"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestRetryOverridesAreOrderIndependent(t *testing.T) {
	t.Parallel()
	for _, original := range []*apperrors.AppError{
		apperrors.ServiceUnavailable("db").WithRetryable(false),
		apperrors.Internal(nil).WithRetryAfter(time.Second),
	} {
		wire, err := Encode(original, "test.Service")
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			got, decodeErr := Decode(wire, nil)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if got.Retryable != original.Retryable || got.RetryAfter != original.RetryAfter {
				t.Fatalf("retry changed: %#v", got)
			}
			slices.Reverse(wire.Details)
		}
	}
}

func TestForeignIdentityCannotReplaceStatus(t *testing.T) {
	t.Parallel()
	detail, err := anypb.New(&errdetails.ErrorInfo{
		Domain: "foreign.example", Reason: "NOT_FOUND",
		Metadata: map[string]string{"code": "NOT_FOUND", "retryable": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(&statuspb.Status{Code: 13, Message: "remote", Details: []*anypb.Any{detail}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.RPCCode != apperrors.RPCCodeInternal || got.Code != apperrors.ErrCodeInternal || got.Retryable {
		t.Fatalf("foreign identity replaced status: %#v", got)
	}
	if normalized := apperrors.Normalize(got); normalized.Message == "remote" {
		t.Fatal("decoded remote error was trusted for public serialization")
	}
}

func TestExplicitFalseWinsOverRetryInfo(t *testing.T) {
	t.Parallel()
	wire, err := Encode(apperrors.ServiceUnavailable("db").WithRetryable(false), "")
	if err != nil {
		t.Fatal(err)
	}
	hint, err := anypb.New(&errdetails.RetryInfo{RetryDelay: durationpb.New(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	wire.Details = append(wire.Details, hint)
	got, err := Decode(wire, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Retryable || got.RetryAfter != 0 {
		t.Fatalf("false override ignored: %#v", got)
	}
}
