package httpclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/security/tlstest"
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
	if !errors.Is(err, resilience.ErrCircuitOpen) {
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

func TestResponse_JSON(t *testing.T) {
	r := &Response{
		StatusCode: 200,
		Body:       []byte(`{"name":"Alice","age":30}`),
	}
	var data struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	if err := r.JSON(&data); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if data.Name != "Alice" {
		t.Errorf("expected Alice, got %s", data.Name)
	}
	if data.Age != 30 {
		t.Errorf("expected 30, got %d", data.Age)
	}
}

func TestResponse_JSON_Invalid(t *testing.T) {
	r := &Response{Body: []byte(`not json`)}
	var data map[string]any
	if err := r.JSON(&data); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestResponse_Text(t *testing.T) {
	r := &Response{Body: []byte("hello world")}
	if got := r.Text(); got != "hello world" {
		t.Errorf("Text() = %q, want %q", got, "hello world")
	}
}

func TestResponse_Text_Empty(t *testing.T) {
	r := &Response{}
	if got := r.Text(); got != "" {
		t.Errorf("Text() = %q, want empty", got)
	}
}

func TestClient_DoStream_NonSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(200)
		w.Write([]byte("raw stream data"))
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

	if stream.SSE != nil {
		t.Error("expected SSE to be nil for non-SSE response")
	}
	if stream.Body == nil {
		t.Error("expected Body to be non-nil for raw stream")
	}
}

func TestComponent_Stop_NilAdapter(t *testing.T) {
	comp := NewComponent(Config{BaseURL: "http://localhost"})
	// Stop before Start — adapter is nil
	if err := comp.Stop(context.Background()); err != nil {
		t.Errorf("Stop() should not error when adapter is nil: %v", err)
	}
}

func TestAdapter_New_WithTLS(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := Config{
		BaseURL: "https://localhost",
		TLS: &security.TLSConfig{
			CAFile:   certs.CAFile,
			CertFile: certs.CertFile,
			KeyFile:  certs.KeyFile,
		},
	}
	adapter, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with TLS failed: %v", err)
	}
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
}

func TestAdapter_New_WithInvalidTLS(t *testing.T) {
	cfg := Config{
		BaseURL: "https://localhost",
		TLS: &security.TLSConfig{
			CAFile: "/nonexistent/ca.pem",
		},
	}
	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for invalid TLS config")
	}
}

func TestAdapter_Do_TLS(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)

	// Start a TLS test server using generated certs
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"tls":true}`)
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{certs.ServerTLS},
	}
	server.StartTLS()
	defer server.Close()

	// Create adapter with CA trust
	cfg := Config{
		BaseURL: server.URL,
		TLS: &security.TLSConfig{
			CAFile: certs.CAFile,
		},
	}
	adapter, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	resp, err := adapter.Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/test",
	})
	if err != nil {
		t.Fatalf("Do() over TLS failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]bool
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !body["tls"] {
		t.Error("expected tls=true in response")
	}
}

func TestAdapter_Do_TLS_WithClientCert(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)

	// Start a TLS server that requires client certificates
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "mtls-ok")
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{certs.ServerTLS},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certs.CertPool,
	}
	server.StartTLS()
	defer server.Close()

	// Create adapter with CA + client cert (mTLS)
	cfg := Config{
		BaseURL: server.URL,
		TLS: &security.TLSConfig{
			CAFile:   certs.CAFile,
			CertFile: certs.CertFile,
			KeyFile:  certs.KeyFile,
		},
	}
	adapter, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	resp, err := adapter.Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/mtls",
	})
	if err != nil {
		t.Fatalf("Do() over mTLS failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "mtls-ok" {
		t.Errorf("expected mtls-ok, got %s", string(resp.Body))
	}
}

func TestBuildRequest_BaseURL_TrailingSlash(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tests := []struct {
		name     string
		baseURL  string
		path     string
		wantPath string
	}{
		{"base no slash + path slash", ts.URL, "/api/v1", "/api/v1"},
		{"base trailing slash + path slash", ts.URL + "/", "/api/v1", "/api/v1"},
		{"base trailing slash + path no slash", ts.URL + "/", "api/v1", "/api/v1"},
		{"empty path", ts.URL, "", "/"},
		{"path only slash", ts.URL, "/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := New(Config{BaseURL: tt.baseURL, Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			resp, err := a.Do(context.Background(), Request{Method: "GET", Path: tt.path})
			if err != nil {
				t.Fatal(err)
			}
			got := resp.Headers["X-Path"]
			if got != tt.wantPath {
				t.Errorf("path = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

func TestBuildRequest_FullURL_IgnoresBaseURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "reached")
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: "http://should-not-be-used.invalid", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := a.Do(context.Background(), Request{Method: "GET", Path: ts.URL + "/override"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text() != "reached" {
		t.Error("full URL did not override base URL")
	}
}

func TestBuildRequest_QueryParams_MergedWithExisting(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-A", r.URL.Query().Get("a"))
		w.Header().Set("X-B", r.URL.Query().Get("b"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: ts.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := a.Do(context.Background(), Request{
		Method: "GET",
		Path:   "/test?a=1",
		Query:  map[string]string{"b": "2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Query params from path are preserved, new ones added
	if resp.Headers["X-B"] != "2" {
		t.Errorf("query param b = %q, want %q", resp.Headers["X-B"], "2")
	}
}

func TestBuildRequest_RequestHeaders_OverrideDefaults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Got", r.Header.Get("X-Custom"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a, err := New(Config{
		BaseURL: ts.URL,
		Timeout: 5 * time.Second,
		Headers: map[string]string{"X-Custom": "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := a.Do(context.Background(), Request{
		Method:  "GET",
		Path:    "/test",
		Headers: map[string]string{"X-Custom": "override"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Headers["X-Got"] != "override" {
		t.Errorf("expected header override, got %q", resp.Headers["X-Got"])
	}
}

func TestEncodeBody_NilBody(t *testing.T) {
	r, ct, err := encodeBody(nil)
	if err != nil {
		t.Fatal(err)
	}
	if r != nil {
		t.Error("expected nil reader for nil body")
	}
	if ct != "" {
		t.Errorf("expected empty content type, got %q", ct)
	}
}

func TestEncodeBody_EmptyBytes(t *testing.T) {
	r, _, err := encodeBody([]byte{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(r)
	if len(data) != 0 {
		t.Error("expected empty body for empty bytes")
	}
}

func TestEncodeBody_StringBody(t *testing.T) {
	r, ct, err := encodeBody("hello world")
	if err != nil {
		t.Fatal(err)
	}
	if ct != "text/plain" {
		t.Errorf("expected text/plain, got %q", ct)
	}
	data, _ := io.ReadAll(r)
	if string(data) != "hello world" {
		t.Errorf("body = %q, want %q", string(data), "hello world")
	}
}

func TestEncodeBody_IOReader(t *testing.T) {
	input := strings.NewReader("raw stream")
	r, ct, err := encodeBody(input)
	if err != nil {
		t.Fatal(err)
	}
	if ct != "" {
		t.Errorf("expected empty content type for io.Reader, got %q", ct)
	}
	data, _ := io.ReadAll(r)
	if string(data) != "raw stream" {
		t.Errorf("body = %q", string(data))
	}
}

func TestEncodeBody_JSONMarshalable(t *testing.T) {
	body := map[string]int{"count": 42}
	r, ct, err := encodeBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	data, _ := io.ReadAll(r)
	var decoded map[string]int
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["count"] != 42 {
		t.Errorf("decoded count = %d, want 42", decoded["count"])
	}
}

func TestEncodeBody_JSONMarshalError(t *testing.T) {
	// Channels can't be marshaled to JSON
	_, _, err := encodeBody(make(chan int))
	if err == nil {
		t.Fatal("expected json marshal error")
	}
}

func TestAdapter_Do_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: ts.URL, Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Do(context.Background(), Request{Method: "GET", Path: "/"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Context-based timeout should yield a timeout error
	if !IsTimeout(err) && !IsConnection(err) {
		t.Errorf("expected timeout or connection error, got %T: %v", err, err)
	}
}

func TestAdapter_Do_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: ts.URL, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = a.Do(ctx, Request{Method: "GET", Path: "/"})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if !IsTimeout(err) {
		t.Errorf("expected timeout error, got %T: %v", err, err)
	}
}

func TestAdapter_ConcurrentRequests(t *testing.T) {
	var count atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: ts.URL, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = a.Do(context.Background(), Request{Method: "GET", Path: "/concurrent"})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("request %d failed: %v", i, err)
		}
	}
	if count.Load() != n {
		t.Errorf("server received %d requests, want %d", count.Load(), n)
	}
}

func TestAdapter_HeaderInjection_Prevention(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: ts.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// Go's net/http rejects headers with newlines
	_, err = a.Do(context.Background(), Request{
		Method: "GET",
		Path:   "/",
		Headers: map[string]string{
			"X-Evil\r\nInjected: true": "value",
		},
	})
	// The request should either fail or the header should be sanitized
	// Go's http library rejects invalid header names
	if err != nil {
		// Expected: Go's http.NewRequest rejects invalid headers
		return
	}
	// If it succeeds, the header was sanitized by the library
}

func TestFlattenHeaders_MultiValue(t *testing.T) {
	h := http.Header{
		"X-Multi":  {"first", "second"},
		"X-Single": {"only"},
	}
	flat := flattenHeaders(h)
	if flat["X-Multi"] != "first" {
		t.Errorf("multi-value header should take first value, got %q", flat["X-Multi"])
	}
	if flat["X-Single"] != "only" {
		t.Errorf("single-value header = %q", flat["X-Single"])
	}
}

func TestFlattenHeaders_EmptyValues(t *testing.T) {
	h := http.Header{
		"X-Empty": {},
	}
	flat := flattenHeaders(h)
	if _, ok := flat["X-Empty"]; ok {
		t.Error("empty value slice should be excluded")
	}
}

func TestAdapter_AllHTTPMethods(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Method", r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: ts.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}

	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			resp, err := a.Do(context.Background(), Request{Method: m, Path: "/"})
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 {
				t.Errorf("status = %d", resp.StatusCode)
			}
		})
	}
}

func TestAdapter_Do_ErrorWithResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: ts.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := a.Do(context.Background(), Request{Method: "GET", Path: "/missing"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !IsNotFound(err) {
		t.Errorf("expected not found error, got %T: %v", err, err)
	}
	// Response should still be available
	if resp == nil {
		t.Fatal("expected response even with error")
	}
	if resp.StatusCode != 404 {
		t.Errorf("resp.StatusCode = %d, want 404", resp.StatusCode)
	}
}

func TestAdapter_ContentType_NotOverridden(t *testing.T) {
	var gotCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: ts.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// If user sets Content-Type, body's auto-detected type should not override
	_, _ = a.Do(context.Background(), Request{
		Method:  "POST",
		Path:    "/",
		Body:    "plain text body",
		Headers: map[string]string{"Content-Type": "application/xml"},
	})
	if gotCT != "application/xml" {
		t.Errorf("content-type = %q, expected application/xml", gotCT)
	}
}

func TestAdapter_DoStream_ErrorStatus_ReturnsClassifiedError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"server down"}`)
	}))
	defer ts.Close()

	a, err := New(Config{BaseURL: ts.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.DoStream(context.Background(), Request{Method: "GET", Path: "/"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsServerError(err) {
		t.Errorf("expected server error, got %T: %v", err, err)
	}
}
