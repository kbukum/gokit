//go:build integration

package connect_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/protobuf/types/known/emptypb"

	kitconnect "github.com/kbukum/gokit/connect"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestTracingTLSStream(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"complete", "refused", "canceled", "closed"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			local, localExport := traceProvider(t)
			remote, remoteExport := traceProvider(t)
			serverTrace, err := kitconnect.TracingInterceptor(kitconnect.TracingConfig{
				Tracer: remote.Tracer("receiver"),
				TrustParent: func(ctx context.Context) bool {
					verified, _ := ctx.Value(verifiedTracePeer{}).(bool)
					return verified
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			const procedure = "/test.Service/Watch"
			completed := make(chan struct{})
			handler := connectrpc.NewServerStreamHandler(procedure,
				func(ctx context.Context, _ *connectrpc.Request[emptypb.Empty], stream *connectrpc.ServerStream[emptypb.Empty]) error {
					if baggage.FromContext(ctx).Len() != 0 {
						t.Error("stream forwarded incoming baggage")
					}
					if err := stream.Send(&emptypb.Empty{}); err != nil {
						return err
					}
					switch outcome {
					case "refused":
						return apperrors.Forbidden("denied").WithReason("STREAM_DENIED")
					case "canceled", "closed":
						<-ctx.Done()
						return ctx.Err()
					default:
						return nil
					}
				}, connectrpc.WithInterceptors(serverTrace, kitconnect.NormalizingInterceptor(nil)))
			server := traceTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(completed)
				handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), verifiedTracePeer{}, true)))
			}))
			httpClient := server.Client()
			t.Cleanup(httpClient.CloseIdleConnections)
			clientTrace, err := kitconnect.TracingInterceptor(kitconnect.TracingConfig{Tracer: local.Tracer("caller")})
			if err != nil {
				t.Fatal(err)
			}
			rpc := connectrpc.NewClient[emptypb.Empty, emptypb.Empty](httpClient, server.URL+procedure,
				connectrpc.WithInterceptors(clientTrace))
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			callCtx, cancelCall := context.WithCancel(ctx)
			defer cancelCall()
			request := connectrpc.NewRequest(&emptypb.Empty{})
			request.Header().Set("Baggage", "secret=private-stream-sentinel")
			stream, err := rpc.CallServerStream(callCtx, request)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cancelCall()
				if err := stream.Close(); err != nil && connectrpc.CodeOf(err) != connectrpc.CodeCanceled {
					t.Errorf("stream cleanup: %v", err)
				}
			})
			if !stream.Receive() {
				t.Fatal("missing first message", stream.Err())
			}
			if len(localExport.GetSpans()) != 0 {
				t.Fatal("client span ended before stream completion")
			}
			switch outcome {
			case "closed":
				if err := stream.Close(); err != nil {
					t.Fatal(err)
				}
			default:
				if outcome == "canceled" {
					cancelCall()
				}
				if stream.Receive() {
					t.Fatal("unexpected second message")
				}
				err := stream.Err()
				switch outcome {
				case "complete":
					if err != nil {
						t.Fatal(err)
					}
				case "refused":
					if connectrpc.CodeOf(err) != connectrpc.CodePermissionDenied {
						t.Fatal("stream refusal changed", err)
					}
				case "canceled":
					if connectrpc.CodeOf(err) != connectrpc.CodeCanceled {
						t.Fatal("stream cancellation changed", err)
					}
				}
			}
			select {
			case <-completed:
			case <-ctx.Done():
				t.Fatal("stream server did not join within budget")
			}
			clientSpans, serverSpans := localExport.GetSpans(), remoteExport.GetSpans()
			if len(clientSpans) != 1 || len(serverSpans) != 1 {
				t.Fatalf("stream spans = %d client, %d server", len(clientSpans), len(serverSpans))
			}
			c, s := clientSpans[0], serverSpans[0]
			if c.Name != procedure || s.Name != procedure || s.Parent.SpanID() != c.SpanContext.SpanID() ||
				s.SpanContext.TraceID() != c.SpanContext.TraceID() {
				t.Fatal("stream trace is disconnected or uses the wrong name")
			}
			if (c.Status.Code == codes.Error) != (outcome != "complete") ||
				(s.Status.Code == codes.Error) != (outcome != "complete") {
				t.Fatal("stream span status does not match completion")
			}
		})
	}
}
