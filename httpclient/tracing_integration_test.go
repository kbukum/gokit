//go:build integration

package httpclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/kbukum/gokit/testutil"
)

func TestTracingTransportNativeUpgrade(t *testing.T) {
	t.Parallel()
	for _, traced := range []bool{false, true} {
		t.Run(fmt.Sprint(traced), func(t *testing.T) {
			t.Parallel()
			handled := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				defer close(handled)
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					t.Error("server cannot hijack")
					return
				}
				conn, rw, err := hijacker.Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
					t.Error(err)
					return
				}
				if _, err := rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n"); err != nil {
					t.Error(err)
					return
				}
				if err := rw.Flush(); err != nil {
					t.Error(err)
					return
				}
				var b [1]byte
				if _, err := io.ReadFull(rw, b[:]); err != nil {
					t.Error(err)
					return
				}
				if _, err := conn.Write(b[:]); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
				defer cancel()
				if err := testutil.CloseHTTPServer(ctx, server); err != nil {
					t.Error(err)
				}
				select {
				case <-handled:
				case <-ctx.Done():
					t.Error("upgrade handler did not finish within cleanup budget")
				}
			})
			base := &http.Transport{}
			t.Cleanup(base.CloseIdleConnections)
			var transport http.RoundTripper = base
			if traced {
				tracer, _ := httpTraceFixture(t)
				var err error
				transport, err = NewTracingTransport(base, TracingConfig{Tracer: tracer, Operation: "upgrade"})
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "test")
			response, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, ok := response.Body.(io.ReadWriteCloser)
			if response.StatusCode != http.StatusSwitchingProtocols || !ok {
				t.Fatalf("upgrade body contract lost: status=%d body=%T", response.StatusCode, response.Body)
			}
			if _, err := body.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
			var b [1]byte
			if _, err := io.ReadFull(body, b[:]); err != nil || b[0] != 'x' {
				t.Fatalf("upgrade echo failed: %v", err)
			}
		})
	}
}

func TestTracingTransportTLS(t *testing.T) {
	t.Parallel()
	tracer, exporter := httpTraceFixture(t)
	received := make(chan trace.SpanContext, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 {
			t.Error("expected TLS 1.3")
		}
		if r.Header.Get("Baggage") != "" {
			t.Error("baggage reached peer")
		}
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		received <- trace.SpanContextFromContext(ctx)
		if _, err := io.WriteString(w, "data"); err != nil {
			t.Error(err)
		}
	}))
	server.EnableHTTP2 = true
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	server.StartTLS()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := testutil.CloseHTTPServer(ctx, server); err != nil {
			t.Errorf("server cleanup: %v", err)
		}
	})
	client := server.Client()
	transport, err := NewTracingTransport(client.Transport, TracingConfig{Tracer: tracer, Operation: "events.subscribe"})
	if err != nil {
		t.Fatal(err)
	}
	client.Transport, client.Timeout = transport, time.Second
	t.Cleanup(client.CloseIdleConnections)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/private-path-sentinel?token=private-query-sentinel", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Baggage", "token=private-baggage-sentinel")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 5))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || string(body) != "data" {
		t.Fatalf("body = %q, read = %v, close = %v", body, readErr, closeErr)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || strings.Contains(fmt.Sprint(spans), "private-") {
		t.Fatal("invalid or unsafe exported client span")
	}
	select {
	case sc := <-received:
		if !sc.IsValid() || sc.TraceID() != spans[0].SpanContext.TraceID() || sc.SpanID() != spans[0].SpanContext.SpanID() {
			t.Fatal("peer did not receive the client span")
		}
	case <-ctx.Done():
		t.Fatal("peer context not received within budget")
	}
}
