package httpclient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaxResponseBytes_RejectsOversizeBufferedBody(t *testing.T) {
	t.Parallel()

	big := bytes.Repeat([]byte("x"), 1<<20) // 1 MiB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, MaxResponseBodyBytes: 1024})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// An oversized success body must be rejected outright rather than returned as a
	// silently truncated (and possibly still-decodable) partial body.
	_, err = c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/"})
	if !IsResponseTooLarge(err) {
		t.Fatalf("Do error = %v, want response-too-large", err)
	}
	if IsRetryable(err) {
		t.Fatal("response-too-large error should not be retryable")
	}
}

func TestMaxResponseBytes_AtLimitReturned(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("x"), 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, MaxResponseBodyBytes: 1024})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(resp.Body) != 1024 {
		t.Fatalf("body = %d bytes, want 1024", len(resp.Body))
	}
}

func TestMaxResponseBytes_DefaultApplied(t *testing.T) {
	t.Parallel()

	var c Config
	c.ApplyDefaults()
	if c.MaxResponseBodyBytes != defaultMaxResponseBytes {
		t.Fatalf("MaxResponseBodyBytes = %d, want default %d", c.MaxResponseBodyBytes, defaultMaxResponseBytes)
	}
}
