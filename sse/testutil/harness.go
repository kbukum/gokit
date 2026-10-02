package testutil

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/sse"
)

// Harness serves an SSE Bus and endpoint with automatic teardown.
type Harness struct {
	// Bus publishes events and exposes resource accounting.
	Bus *sse.Bus
	// Server is the backing httptest.Server.
	Server *httptest.Server
}

// New serves a real endpoint with the supplied limits and authorization.
func New(t *testing.T, limits sse.Limits, cfg sse.HandlerConfig) *Harness {
	t.Helper()
	bus, err := sse.NewBus(limits)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := sse.NewHandler(bus, cfg)
	if err != nil {
		bus.Close()
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		bus.Close()
		server.Close()
	})
	return &Harness{Bus: bus, Server: server}
}

// Connect opens an SSE connection. When token is non-empty it is sent as an
// `Authorization: Bearer <token>` header — never as a query parameter. The
// returned [StreamClient] wraps the response body regardless of status, so
// callers can assert on both accepted (200) and rejected (401/403) connections;
// close it when done.
func (h *Harness) Connect(ctx context.Context, token string) (*StreamClient, error) {
	return h.Resume(ctx, token, "")
}

// Resume sends an acknowledged application cursor in Last-Event-ID.
func (h *Harness) Resume(ctx context.Context, token, cursor string) (*StreamClient, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Server.URL, http.NoBody)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", security.BearerAuthScheme+" "+token)
	}
	if cursor != "" {
		req.Header.Set("Last-Event-ID", cursor)
	}
	resp, err := h.Server.Client().Do(req) //nolint:bodyclose // body ownership transfers to StreamClient; closed via StreamClient.Close (or RequireStatus for rejections).
	if err != nil {
		return nil, err
	}
	return newStreamClient(resp), nil
}

// MustConnect opens a connection and fails the test on a transport error. It does
// not assert on status; use [RequireStatus] for that.
func (h *Harness) MustConnect(t *testing.T, ctx context.Context, token string) *StreamClient {
	t.Helper()
	stream, err := h.Connect(ctx, token)
	if err != nil {
		t.Fatalf("connecting SSE stream: %v", err)
	}
	return stream
}

// RequireStatus asserts the stream's response status, and drains and closes
// rejected bodies so no test goroutine leaks a held connection.
func RequireStatus(t *testing.T, stream *StreamClient, want int) {
	t.Helper()
	resp := stream.Response() //nolint:bodyclose // stream owns the body; the caller closes accepted streams, and rejected ones are drained/closed below.
	if resp.StatusCode != want {
		t.Fatalf("expected status %d, got %d", want, resp.StatusCode)
	}
	if want != http.StatusOK {
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Errorf("draining rejected SSE body: %v", err)
		}
		if err := stream.Close(); err != nil {
			t.Errorf("closing rejected SSE stream: %v", err)
		}
	}
}
