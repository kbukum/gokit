//go:build integration

package forward_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kbukum/gokit/connect/client"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/testutil"
)

func TestUnaryTLSRoundTrip(t *testing.T) {
	t.Parallel()
	e := entry(t, descriptorpb.MethodOptions_NO_SIDE_EFFECTS)
	handler, err := e.Handler(echo)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 {
			t.Error("expected trusted TLS 1.3")
		}
		handler.ServeHTTP(w, r)
	}))
	server.EnableHTTP2 = true
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	server.StartTLS()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := testutil.CloseHTTPServer(ctx, server); err != nil {
			t.Errorf("bounded shutdown: %v", err)
		}
	})
	httpClient := server.Client()
	t.Cleanup(httpClient.CloseIdleConnections)
	for _, tc := range []struct {
		name     string
		protocol string
		options  []connectrpc.ClientOption
	}{
		{"protobuf", client.ProtocolConnect, nil},
		{"json", client.ProtocolConnect, []connectrpc.ClientOption{connectrpc.WithProtoJSON()}},
		{"get", client.ProtocolConnect, []connectrpc.ClientOption{connectrpc.WithHTTPGet()}},
		{"grpc", client.ProtocolGRPC, nil},
		{"grpcweb", client.ProtocolGRPCWeb, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			send, err := e.Client(httpClient, client.Config{
				BaseURL: server.URL, Timeout: time.Second, Protocol: tc.protocol,
				TLS: &security.TLSConfig{MinVersion: tls.VersionTLS13},
			}, tc.options...)
			if err != nil {
				t.Fatal(err)
			}
			in := wrapperspb.String(tc.name)
			out, err := send(t.Context(), in, nil)
			if err != nil || !proto.Equal(in, out) {
				t.Fatalf("out = %v, error = %v", out, err)
			}
		})
	}
	t.Cleanup(func() {
		if got := calls.Load(); got != 5 {
			t.Errorf("calls = %d, want one per protocol", got)
		}
	})
}
