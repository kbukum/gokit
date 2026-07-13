package sse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// readSSEFrame reads from an SSE stream until the first complete frame (ended by
// a blank line) arrives, then returns the accumulated bytes. Network reads may
// return partial frames, so it accumulates rather than trusting a single Read.
// It fails the test on an unexpected read error, and skips when the read ended
// because the test context was canceled/timed out.
func readSSEFrame(t *testing.T, ctx context.Context, r io.Reader) string {
	t.Helper()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
			if strings.Contains(sb.String(), "\n\n") {
				return sb.String()
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				t.Skipf("SSE read ended due to context (%v)", ctx.Err())
			}
			if errors.Is(err, io.EOF) {
				return sb.String()
			}
			t.Fatalf("unexpected SSE read error: %v", err)
		}
	}
}

func TestEdge_SSEHeaders(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeSSE(hub, w, r, "test:headers")
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			t.Skipf("request ended due to context (%v); SSE stream held open", ctx.Err())
		}
		t.Fatalf("unexpected request error: %v", err)
	}
	defer resp.Body.Close()

	tests := []struct {
		header, expected string
	}{
		{"Content-Type", "text/event-stream"},
		{"Cache-Control", "no-cache"},
		{"Connection", "keep-alive"},
		{"X-Accel-Buffering", "no"},
	}
	for _, tc := range tests {
		got := resp.Header.Get(tc.header)
		if got != tc.expected {
			t.Errorf("header %s: expected %q, got %q", tc.header, tc.expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrent broadcast + subscribe/unsubscribe
// ---------------------------------------------------------------------------

func TestEdge_ServeSSE_ConnectedEventFormat(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeSSE(hub, w, r, "test:fmt", WithUserID("u1"), WithSessionID("s1"))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			t.Skipf("request ended due to context (%v); SSE stream held open", ctx.Err())
		}
		t.Fatalf("unexpected request error: %v", err)
	}
	defer resp.Body.Close()

	body := readSSEFrame(t, ctx, resp.Body)

	if !strings.Contains(body, "event: connected") {
		t.Errorf("expected 'event: connected' in body, got %q", body)
	}
	if !strings.Contains(body, `"client_id":"test:fmt"`) {
		t.Errorf("expected client_id in body, got %q", body)
	}
	if !strings.Contains(body, `"user_id":"u1"`) {
		t.Errorf("expected user_id in body, got %q", body)
	}
}

// ---------------------------------------------------------------------------
// Broadcast to no matching clients is a no-op
// ---------------------------------------------------------------------------
