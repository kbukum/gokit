package connect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connectrpc "connectrpc.com/connect"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/kbukum/gokit/connect"
	"github.com/kbukum/gokit/contracttest/golden"
	"github.com/kbukum/gokit/contracttest/wire"
	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
)

const goldenDomain = "gokit.test.v1.TestService"

func TestPublishedConnectJSONMatchesActualHandler(t *testing.T) {
	t.Parallel()
	fixtures, err := wire.LoadFixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range wire.WireCases() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			handler := connectrpc.NewUnaryHandler("/gokit.test.v1.TestService/Call",
				func(context.Context, *connectrpc.Request[emptypb.Empty]) (*connectrpc.Response[emptypb.Empty], error) {
					encoded, encodeErr := connect.ToConnectError(c.Err, goldenDomain)
					if encodeErr != nil {
						return nil, encodeErr
					}
					return nil, encoded
				})
			req := httptest.NewRequest(http.MethodPost, "/gokit.test.v1.TestService/Call", strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Connect-Protocol-Version", "1")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			golden.AssertJSON(t, response.Body.Bytes(), string(fixtures[c.Name].ConnectJSON))
		})
	}
}

// TestGoldenFixtures_ConnectRoundTrip proves every published golden fixture
// survives AppError → Connect → AppError over the Connect wire, and that the
// numeric Connect code matches the one the fixture publishes.
func TestGoldenFixtures_ConnectRoundTrip(t *testing.T) {
	t.Parallel()

	fixtures, err := wire.LoadFixtures()
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}

	for _, c := range wire.WireCases() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			f := fixtures[c.Name]

			cerr, encodeErr := connect.ToConnectError(c.Err, goldenDomain)
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			if cerr == nil {
				t.Fatal("ToConnectError returned nil")
			}
			if uint32(cerr.Code()) != f.RPCCode {
				t.Errorf("connect code = %d, want published rpcCode %d", cerr.Code(), f.RPCCode)
			}
			if cerr.Code() != connectrpc.Code(apperrors.RPCCodeFor(c.Err.Code)) {
				t.Errorf("connect code = %v, not the table code", cerr.Code())
			}

			got, decodeErr := connect.DecodeError(cerr)
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
