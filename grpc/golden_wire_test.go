package grpc_test

import (
	"bytes"
	"testing"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/kbukum/gokit/contracttest/wire"
	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
	grpcwire "github.com/kbukum/gokit/grpc"
)

const goldenDomain = "gokit.test.v1.TestService"

// TestGoldenFixtures_GRPCRoundTrip proves every published golden fixture survives
// AppError → gRPC status → AppError over the gRPC wire, and that the numeric gRPC
// code matches the one the fixture publishes.
func TestGoldenFixtures_GRPCRoundTrip(t *testing.T) {
	t.Parallel()

	fixtures, err := wire.LoadFixtures()
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}

	for _, c := range wire.WireCases() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			f := fixtures[c.Name]

			st, encodeErr := grpcwire.AppErrorToStatus(c.Err, goldenDomain)
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			if st == nil {
				t.Fatal("AppErrorToStatus returned nil")
			}
			actual, marshalErr := proto.MarshalOptions{Deterministic: true}.Marshal(st.Proto())
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if !bytes.Equal(actual, f.GRPCStatus) {
				t.Fatal("gRPC status bytes differ from published fixture")
			}
			published := &statuspb.Status{}
			if unmarshalErr := proto.Unmarshal(f.GRPCStatus, published); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			if _, decodeErr := errorrpc.Decode(published, nil); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if uint32(st.Code()) != f.RPCCode {
				t.Errorf("grpc code = %d, want published rpcCode %d", st.Code(), f.RPCCode)
			}
			if st.Code() != codes.Code(apperrors.RPCCodeFor(c.Err.Code)) {
				t.Errorf("grpc code = %v, not the table code", st.Code())
			}

			got, decodeErr := grpcwire.DecodeError(st.Err())
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			assertVocabulary(t, got, c.Err)
		})
	}
}

func assertVocabulary(t *testing.T, got *errorrpc.Error, want *apperrors.AppError) {
	t.Helper()
	if got == nil {
		t.Fatal("decoded error is nil")
	}
	if got.Code != want.Code {
		t.Errorf("code = %q, want %q", got.Code, want.Code)
	}
	if got.Message != want.Message {
		t.Errorf("message = %q, want %q", got.Message, want.Message)
	}
	if got.Reason != want.Reason {
		t.Errorf("reason = %q, want %q", got.Reason, want.Reason)
	}
	if got.TraceID != want.TraceID {
		t.Errorf("traceId = %q, want %q", got.TraceID, want.TraceID)
	}
	if got.Retryable != want.Retryable {
		t.Errorf("retryable = %v, want %v", got.Retryable, want.Retryable)
	}
	if got.RetryAfter != want.RetryAfter {
		t.Errorf("retryAfter = %v, want %v", got.RetryAfter, want.RetryAfter)
	}
	if len(got.Violations) != len(want.Violations) {
		t.Fatalf("violations len = %d, want %d", len(got.Violations), len(want.Violations))
	}
	for i := range want.Violations {
		if got.Violations[i] != want.Violations[i] {
			t.Errorf("violation[%d] = %+v, want %+v", i, got.Violations[i], want.Violations[i])
		}
	}
}
