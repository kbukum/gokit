package rpc

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	apperrors "github.com/kbukum/gokit/errors"
)

func pack(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	result, err := anypb.New(msg)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDecodeRejectsMalformedContract(t *testing.T) {
	t.Parallel()
	info := pack(t, &errdetails.ErrorInfo{Domain: Domain, Reason: "INTERNAL_ERROR", Metadata: map[string]string{"retryable": "false"}})
	retry := pack(t, &errdetails.RetryInfo{})
	bad := pack(t, &errdetails.BadRequest{})
	cases := map[string]*statuspb.Status{
		"success is not a failure": {Code: 0},
		"unknown status":           {Code: 99},
		"nil detail":               {Code: 13, Details: []*anypb.Any{nil}},
		"invalid bytes":            {Code: 13, Details: []*anypb.Any{{TypeUrl: info.TypeUrl, Value: []byte{255}}}},
		"duplicate identity":       {Code: 13, Details: []*anypb.Any{info, info}},
		"duplicate retry":          {Code: 13, Details: []*anypb.Any{retry, retry}},
		"duplicate violations":     {Code: 13, Details: []*anypb.Any{bad, bad}},
		"contradictory category":   {Code: 5, Details: []*anypb.Any{info}},
		"missing retry verdict":    {Code: 13, Details: []*anypb.Any{pack(t, &errdetails.ErrorInfo{Domain: Domain, Reason: "INTERNAL_ERROR"})}},
		"invalid reason":           {Code: 13, Details: []*anypb.Any{pack(t, &errdetails.ErrorInfo{Domain: Domain, Reason: "INTERNAL_ERROR", Metadata: map[string]string{"retryable": "false", "reason": "lowercase"}})}},
		"invalid violation reason": {Code: 3, Details: []*anypb.Any{pack(t, &errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{{Reason: "string.min_len"}}})}},
		"negative delay":           {Code: 14, Details: []*anypb.Any{pack(t, &errdetails.RetryInfo{RetryDelay: durationpb.New(-time.Second)})}},
		"invalid proto delay":      {Code: 14, Details: []*anypb.Any{pack(t, &errdetails.RetryInfo{RetryDelay: &durationpb.Duration{Seconds: 1, Nanos: -1}})}},
		"overflow seconds":         {Code: 14, Details: []*anypb.Any{pack(t, &errdetails.RetryInfo{RetryDelay: &durationpb.Duration{Seconds: 10000000000}})}},
		"overflow nanos":           {Code: 14, Details: []*anypb.Any{pack(t, &errdetails.RetryInfo{RetryDelay: &durationpb.Duration{Seconds: 9223372036, Nanos: 854775808}})}},
	}
	for name, wire := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := Decode(wire, nil)
			var malformed *DecodeError
			if got != nil || !errors.As(err, &malformed) || err.Error() == "" {
				t.Fatalf("expected explicit decode error, got %#v, %v", got, err)
			}
		})
	}
}

func TestUnknownIdentityAndDetailsPreserveProtocol(t *testing.T) {
	t.Parallel()
	wire := &statuspb.Status{Code: 10, Message: "transaction aborted", Details: []*anypb.Any{
		pack(t, &emptypb.Empty{}),
		pack(t, &errdetails.ErrorInfo{Domain: Domain, Reason: "FUTURE_CODE", Metadata: map[string]string{"retryable": "false"}}),
		pack(t, &errdetails.RetryInfo{}),
		pack(t, &errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: "full_name", Description: "invalid"}}}),
	}}
	cause := errors.New("original")
	got, err := Decode(wire, cause)
	if err != nil {
		t.Fatal(err)
	}
	if got.RPCCode != apperrors.RPCCodeAborted || !got.Retryable || got.RetryAfter != 0 ||
		got.Violations[0].Reason != apperrors.ViolationInvalidValue || !errors.Is(got, cause) || got.HTTPStatus() != 500 {
		t.Fatalf("unexpected fallback: %#v", got)
	}
	if _, err := Decode(nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Encode(nil, ""); err != nil {
		t.Fatal(err)
	}
}

func TestEncodingRejectsInvalidVocabulary(t *testing.T) {
	t.Parallel()
	for _, err := range []*apperrors.AppError{
		apperrors.New("UNKNOWN_CODE", "unknown"),
		apperrors.Internal(nil).WithReason("lowercase"),
		apperrors.Internal(nil).WithRetryAfter(-time.Second),
		apperrors.Validation("invalid").WithViolations(apperrors.Violation{Reason: "string.min_len"}),
		apperrors.New(apperrors.ErrCodeInternal, string([]byte{255})),
	} {
		if encoded, encodeErr := Encode(err, ""); encoded != nil || encodeErr == nil {
			t.Fatalf("invalid vocabulary encoded: %#v", err)
		}
	}
	if _, err := Encode(apperrors.Internal(nil), string([]byte{255})); err == nil {
		t.Fatal("invalid metadata text was accepted")
	}
}
