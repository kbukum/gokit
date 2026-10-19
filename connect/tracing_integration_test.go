//go:build integration

package connect_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/emptypb"

	kitconnect "github.com/kbukum/gokit/connect"
	"github.com/kbukum/gokit/connect/client"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/testutil"
)

type verifiedTracePeer struct{}

func traceProvider(t *testing.T) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			t.Errorf("provider cleanup: %v", err)
		}
	})
	return provider, exporter
}

func traceTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	server.StartTLS()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := testutil.CloseHTTPServer(ctx, server); err != nil {
			t.Errorf("server cleanup exceeded budget: %v", err)
		}
	})
	return server
}

func TestTracingTLSPropagation(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"success", "refused", "unauthenticated", "canceled", "peer-timeout"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			gateway, gatewayExport := traceProvider(t)
			control, controlExport := traceProvider(t)
			var logs bytes.Buffer
			log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "control", logging.WithWriter(&logs))
			if err != nil {
				t.Fatal(err)
			}
			serverTrace, err := kitconnect.TracingInterceptor(kitconnect.TracingConfig{
				Tracer: control.Tracer("control"),
				TrustParent: func(ctx context.Context) bool {
					verified, _ := ctx.Value(verifiedTracePeer{}).(bool)
					return verified
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			started, completed := make(chan struct{}), make(chan struct{})
			const procedure = "/test.Service/Call"
			handler := connectrpc.NewUnaryHandler(procedure,
				func(ctx context.Context, _ *connectrpc.Request[emptypb.Empty]) (*connectrpc.Response[emptypb.Empty], error) {
					close(started)
					if baggage.FromContext(ctx).Len() != 0 {
						t.Error("peer baggage reached handler")
					}
					switch outcome {
					case "unauthenticated":
						return nil, apperrors.Unauthorized("peer required").WithReason("PEER_UNTRUSTED")
					case "refused":
						return nil, apperrors.Forbidden("denied").WithReason("POLICY_DENIED").WithCause(errors.New("private-cause-sentinel"))
					case "canceled", "peer-timeout":
						<-ctx.Done()
						return nil, ctx.Err()
					default:
						return connectrpc.NewResponse(&emptypb.Empty{}), nil
					}
				}, connectrpc.WithInterceptors(serverTrace, kitconnect.LoggingInterceptor(log), kitconnect.NormalizingInterceptor(log)))
			server := traceTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(completed)
				if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 {
					t.Error("expected TLS 1.3")
				}
				// The kit accepts an authenticated-context hook; consumers prove their real authenticator separately.
				if outcome != "unauthenticated" {
					r = r.WithContext(context.WithValue(r.Context(), verifiedTracePeer{}, true))
				}
				handler.ServeHTTP(w, r)
			}))
			httpClient := server.Client()
			t.Cleanup(httpClient.CloseIdleConnections)
			clientTrace, err := kitconnect.TracingInterceptor(kitconnect.TracingConfig{Tracer: gateway.Tracer("gateway")})
			if err != nil {
				t.Fatal(err)
			}
			peerBudget := time.Second
			if outcome == "peer-timeout" {
				peerBudget = 100 * time.Millisecond
			}
			timeout, err := client.UnaryTimeoutInterceptor(peerBudget)
			if err != nil {
				t.Fatal(err)
			}
			rpc := connectrpc.NewClient[emptypb.Empty, emptypb.Empty](httpClient, server.URL+procedure,
				connectrpc.WithInterceptors(clientTrace, timeout))
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			callCtx, cancelCall := context.WithCancel(ctx)
			defer cancelCall()
			callCtx, root := gateway.Tracer("gateway").Start(callCtx, "public.request", trace.WithNewRoot())
			req := connectrpc.NewRequest(&emptypb.Empty{})
			req.Header().Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
			req.Header().Set("Baggage", "secret=private-baggage-sentinel")
			req.Header().Set("Authorization", "private-credential-sentinel")
			result := make(chan error, 1)
			callDone := make(chan struct{})
			go func() {
				defer close(callDone)
				_, callErr := rpc.CallUnary(callCtx, req)
				result <- callErr
			}()
			t.Cleanup(func() {
				cancelCall()
				cleanup, stop := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
				defer stop()
				select {
				case <-callDone:
				case <-cleanup.Done():
					t.Error("client call did not join within cleanup budget")
				}
			})
			if outcome == "canceled" {
				select {
				case <-started:
					cancelCall()
				case <-ctx.Done():
					t.Fatal("handler did not start within budget")
				}
			}
			select {
			case err = <-result:
			case <-ctx.Done():
				t.Fatal("RPC exceeded fixture budget")
			}
			root.End()
			select {
			case <-completed:
			case <-ctx.Done():
				t.Fatal("server did not finish within budget")
			}
			want := map[string]connectrpc.Code{
				"refused": connectrpc.CodePermissionDenied, "unauthenticated": connectrpc.CodeUnauthenticated,
				"canceled": connectrpc.CodeCanceled, "peer-timeout": connectrpc.CodeDeadlineExceeded,
			}
			if outcome == "success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if connectrpc.CodeOf(err) != want[outcome] {
				t.Fatalf("code = %v, want %v", connectrpc.CodeOf(err), want[outcome])
			}
			if outcome == "peer-timeout" && !errors.Is(err, client.ErrUnaryTimeout) {
				t.Fatal("peer timeout lost its cause")
			}
			local, remote := gatewayExport.GetSpans(), controlExport.GetSpans()
			if len(local) != 2 || len(remote) != 1 {
				t.Fatalf("gateway spans = %d, control spans = %d", len(local), len(remote))
			}
			clientSpan, serverSpan := local[0], remote[0]
			if clientSpan.Name != procedure || serverSpan.Name != procedure {
				t.Fatal("span names are not canonical procedures")
			}
			if clientSpan.Parent.SpanID() != root.SpanContext().SpanID() {
				t.Fatal("client span detached from gateway")
			}
			if outcome == "unauthenticated" {
				if serverSpan.Parent.IsValid() || serverSpan.SpanContext.TraceID() == root.SpanContext().TraceID() {
					t.Fatal("unverified peer selected server correlation")
				}
			} else if serverSpan.Parent.SpanID() != clientSpan.SpanContext.SpanID() ||
				serverSpan.SpanContext.TraceID() != root.SpanContext().TraceID() {
				t.Fatal("trusted distributed trace is disconnected")
			}
			if (clientSpan.Status.Code == codes.Error) != (outcome != "success") ||
				(serverSpan.Status.Code == codes.Error) != (outcome != "success") {
				t.Fatal("span status does not reflect RPC outcome")
			}
			if outcome == "refused" || outcome == "unauthenticated" {
				decoded, decodeErr := kitconnect.DecodeError(err)
				if decodeErr != nil || decoded.TraceID != serverSpan.SpanContext.TraceID().String() || decoded.Reason == "" {
					t.Fatal("wire error lost correlation or refusal reason")
				}
			}
			text := fmt.Sprint(local, remote, logs.String())
			if strings.Contains(text, "private-") || !strings.Contains(logs.String(), serverSpan.SpanContext.TraceID().String()) {
				t.Fatal("telemetry leaked input or lacked log correlation")
			}
		})
	}
}
