package client

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"

	kitconnect "github.com/kbukum/gokit/connect"
	apperrors "github.com/kbukum/gokit/errors"
)

func gatewayClient(t *testing.T, status int, contentType string) (*connect.Client[wrapperspb.StringValue, wrapperspb.StringValue], *Availability) {
	t.Helper()
	const path = "/test.Service/Call"
	s := h2cServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte("<html>upstream unavailable</html>"))
	})
	a := NewAvailability("peer")
	return connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](h2cClient(t), s.URL+path, connect.WithInterceptors(a)), a
}

func TestGatewayOutageStatusIsAnOutage(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		for _, contentType := range []string{"text/html", "", "application/json"} {
			t.Run(strconv.Itoa(status)+"/"+contentType, func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				c, a := gatewayClient(t, status, contentType)
				calls := map[string]error{}
				// A Connect unary JSON body is a protocol error that connect-go decodes itself; streams never carry one.
				if contentType != "application/json" {
					_, calls["unary"] = c.CallUnary(ctx, connect.NewRequest(wrapperspb.String("request")))
				}
				stream, err := c.CallServerStream(ctx, connect.NewRequest(wrapperspb.String("request")))
				if err != nil {
					t.Fatal(err)
				}
				for stream.Receive() {
					t.Fatal("gateway response delivered a message")
				}
				calls["stream"] = stream.Err()
				if closeErr := stream.Close(); closeErr != nil {
					t.Fatal(closeErr)
				}
				for name, callErr := range calls {
					if !IsTransportFailure(callErr) || !IsUnavailable(callErr) || connect.CodeOf(callErr) != connect.CodeUnavailable {
						t.Fatalf("%s err = %v (code %v), want a marked unavailable transport failure", name, callErr, connect.CodeOf(callErr))
					}
					mapped, ok := MapCallFailure(ctx, "peer", callErr)
					if !ok || mapped.Code != apperrors.ErrCodeServiceUnavailable || !mapped.Retryable || mapped.Reason != ReasonUnavailable {
						t.Fatalf("%s mapped = %v, %v", name, mapped, ok)
					}
				}
				assertState(t, a, AvailabilityUnavailable)
			})
		}
	}
}

func TestGatewayStatusOutsideOutagesStaysAnswer(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	c, _ := gatewayClient(t, http.StatusForbidden, "text/html")
	_, err := c.CallUnary(ctx, connect.NewRequest(wrapperspb.String("request")))
	if IsTransportFailure(err) || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("err = %v, want connect's unmarked permission_denied", err)
	}
}

func TestConnectErrorWithOutageStatusKeepsPeerDetails(t *testing.T) {
	t.Parallel()
	wire, err := kitconnect.ToConnectError(apperrors.ServiceUnavailable("private").WithRetryAfter(7*time.Second), "peer")
	if err != nil {
		t.Fatal(err)
	}
	callErr := peerError(t, wire)
	if IsTransportFailure(callErr) {
		t.Fatalf("Connect JSON error was marked as transport failure: %v", callErr)
	}
	mapped, ok := MapCallFailure(t.Context(), "peer", callErr)
	if !ok || mapped.RetryAfter != 7*time.Second || !errors.Is(mapped, callErr) {
		t.Fatalf("mapped = %v, %v; want the peer's retry delay preserved", mapped, ok)
	}
}
