package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/resilience"
)

func TestClient_Do_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/users/123" {
			t.Errorf("expected /users/123, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"name": "Alice"})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := c.Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/users/123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !resp.IsSuccess() {
		t.Error("expected IsSuccess=true")
	}
	if !strings.Contains(string(resp.Body), "Alice") {
		t.Errorf("response body should contain Alice, got %s", string(resp.Body))
	}
}

func TestClient_Do_POST_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := c.Do(context.Background(), Request{
		Method: http.MethodPost,
		Path:   "/users",
		Body:   map[string]string{"name": "Bob"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
}

func TestClient_Do_DefaultHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Custom"); got != "value" {
			t.Errorf("expected X-Custom=value, got %q", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(Config{
		BaseURL: srv.URL,
		Headers: map[string]string{"X-Custom": "value"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Do_QueryParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Errorf("expected page=2, got %q", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = c.Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/items",
		Query:  map[string]string{"page": "2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Do_Auth_Bearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %q", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(Config{
		BaseURL: srv.URL,
		Auth:    BearerAuth("test-token"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Do_Auth_PerRequestOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer override-token" {
			t.Errorf("expected override-token, got %q", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(Config{
		BaseURL: srv.URL,
		Auth:    BearerAuth("default-token"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = c.Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/",
		Auth:   BearerAuth("override-token"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Do_ErrorClassification(t *testing.T) {
	tests := []struct {
		code    int
		checker func(error) bool
	}{
		{401, IsAuth},
		{403, IsAuth},
		{404, IsNotFound},
		{429, IsRateLimit},
		{500, IsServerError},
		{503, IsServerError},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("HTTP_%d", tt.code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
				w.Write([]byte(`{"error":"test"}`))
			}))
			defer srv.Close()

			c, err := New(Config{BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			resp, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/"})
			if err == nil {
				t.Fatal("expected error")
			}
			if !tt.checker(err) {
				t.Errorf("error classification failed for HTTP %d: %v", tt.code, err)
			}
			if resp == nil {
				t.Fatal("expected response even on error")
			}
			if resp.StatusCode != tt.code {
				t.Errorf("expected status %d, got %d", tt.code, resp.StatusCode)
			}
		})
	}
}

func TestClient_Do_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = c.Do(ctx, Request{Method: http.MethodGet, Path: "/"})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestClient_Do_FullURL_IgnoresBaseURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: "http://should-not-be-used.invalid"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := c.Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   srv.URL + "/direct",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestClient_Do_Retry(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	retryCfg := resilience.DefaultRetryConfig()
	retryCfg.MaxAttempts = 3
	retryCfg.InitialBackoff = 10 * time.Millisecond
	retryCfg.RetryIf = IsRetryable

	c, err := New(Config{
		BaseURL: srv.URL,
		Retry:   &retryCfg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestClient_Do_CircuitBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	cbCfg := resilience.DefaultCircuitBreakerConfig("test")
	cbCfg.MaxFailures = 2

	c, err := New(Config{
		BaseURL:        srv.URL,
		CircuitBreaker: &cbCfg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	// Trip the circuit breaker
	for i := 0; i < 3; i++ {
		c.Do(ctx, Request{Method: http.MethodGet, Path: "/"})
	}

	// Next call should fail with circuit open
	_, err = c.Do(ctx, Request{Method: http.MethodGet, Path: "/"})
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	if err != resilience.ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestClient_DoStream_SSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, "data: hello\n\ndata: world\n\n")
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stream, err := c.DoStream(context.Background(), Request{Method: http.MethodGet, Path: "/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	if stream.SSE == nil {
		t.Fatal("expected SSE reader")
	}

	ev1, err := stream.SSE.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev1.Data != "hello" {
		t.Errorf("first event data = %q, want %q", ev1.Data, "hello")
	}

	ev2, err := stream.SSE.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev2.Data != "world" {
		t.Errorf("second event data = %q, want %q", ev2.Data, "world")
	}
}

func TestClient_DoStream_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = c.DoStream(context.Background(), Request{Method: http.MethodGet, Path: "/"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsAuth(err) {
		t.Errorf("expected auth error, got %v", err)
	}
}

func TestClient_Do_StringBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "text/plain" {
			t.Errorf("expected text/plain, got %q", ct)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = c.Do(context.Background(), Request{
		Method: http.MethodPost,
		Path:   "/",
		Body:   "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Do_ByteBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = c.Do(context.Background(), Request{
		Method: http.MethodPost,
		Path:   "/",
		Body:   []byte("raw bytes"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Unwrap(t *testing.T) {
	c, err := New(Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Unwrap() == nil {
		t.Error("Unwrap should return non-nil http.Client")
	}
}

func TestResponse_Helpers(t *testing.T) {
	r := &Response{StatusCode: 200}
	if !r.IsSuccess() {
		t.Error("200 should be success")
	}
	if r.IsError() {
		t.Error("200 should not be error")
	}

	r2 := &Response{StatusCode: 500}
	if r2.IsSuccess() {
		t.Error("500 should not be success")
	}
	if !r2.IsError() {
		t.Error("500 should be error")
	}
}
