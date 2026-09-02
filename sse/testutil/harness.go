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

// Harness runs an [sse.Hub] behind an httptest.Server that serves a single SSE
// endpoint configured with caller-supplied [sse.ServeOption]s. The hub loop and
// server are torn down automatically via t.Cleanup.
type Harness struct {
	// Hub is the running hub; broadcast to it to drive connected streams.
	Hub *sse.Hub
	// Server is the backing httptest.Server.
	Server *httptest.Server
}

// New starts a Harness serving ServeSSE at the server root with the given base
// clientID and options. When the options include an [sse.WithClientIdentity]
// resolver, the resolved routing key overrides baseClientID per connection.
func New(t *testing.T, baseClientID string, opts ...sse.ServeOption) *Harness {
	t.Helper()

	hub := sse.NewHub()
	go hub.Run()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse.ServeSSE(hub, w, r, baseClientID, opts...)
	})
	server := httptest.NewServer(handler)

	t.Cleanup(func() {
		server.Close()
		hub.Stop()
	})
	return &Harness{Hub: hub, Server: server}
}

// Connect opens an SSE connection. When token is non-empty it is sent as an
// `Authorization: Bearer <token>` header — never as a query parameter. The
// returned [StreamClient] wraps the response body regardless of status, so
// callers can assert on both accepted (200) and rejected (401/403) connections;
// close it when done.
func (h *Harness) Connect(ctx context.Context, token string) (*StreamClient, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Server.URL, http.NoBody)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", security.BearerAuthScheme+" "+token)
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

// RequireStatus asserts the stream's response status, drains and closes rejected
// bodies so no test goroutine leaks a held connection, and returns the response.
func RequireStatus(t *testing.T, stream *StreamClient, want int) {
	t.Helper()
	resp := stream.Response() //nolint:bodyclose // stream owns the body; the caller closes accepted streams, and rejected ones are drained/closed below.
	if resp.StatusCode != want {
		t.Fatalf("expected status %d, got %d", want, resp.StatusCode)
	}
	if want != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = stream.Close()
	}
}
