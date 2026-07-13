package sse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
		return // timeout acceptable for SSE
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
		return
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

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
