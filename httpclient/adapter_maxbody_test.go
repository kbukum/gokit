package httpclient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaxResponseBytes_CapsBufferedBody(t *testing.T) {
	t.Parallel()

	big := bytes.Repeat([]byte("x"), 1<<20) // 1 MiB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, MaxResponseBytes: 1024})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(resp.Body) != 1024 {
		t.Fatalf("body = %d bytes, want cap of 1024", len(resp.Body))
	}
}

func TestMaxResponseBytes_DefaultApplied(t *testing.T) {
	t.Parallel()

	var c Config
	c.ApplyDefaults()
	if c.MaxResponseBytes != defaultMaxResponseBytes {
		t.Fatalf("MaxResponseBytes = %d, want default %d", c.MaxResponseBytes, defaultMaxResponseBytes)
	}
}
