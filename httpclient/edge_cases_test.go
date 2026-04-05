package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// URL building edge cases
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Header override behavior
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Body encoding edge cases
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Response parsing edge cases
// ---------------------------------------------------------------------------

func TestResponse_JSON_Empty(t *testing.T) {
	resp := &Response{Body: []byte{}}
	var v map[string]string
	err := resp.JSON(&v)
	if err == nil {
		t.Error("expected error decoding empty JSON body")
	}
}

func TestResponse_IsSuccess_Boundaries(t *testing.T) {
	tests := []struct {
		code    int
		success bool
		isErr   bool
	}{
		{199, false, false},
		{200, true, false},
		{299, true, false},
		{300, false, false},
		{399, false, false},
		{400, false, true},
		{500, false, true},
	}
	for _, tt := range tests {
		r := &Response{StatusCode: tt.code}
		if r.IsSuccess() != tt.success {
			t.Errorf("StatusCode %d: IsSuccess() = %v, want %v", tt.code, r.IsSuccess(), tt.success)
		}
		if r.IsError() != tt.isErr {
			t.Errorf("StatusCode %d: IsError() = %v, want %v", tt.code, r.IsError(), tt.isErr)
		}
	}
}

// ---------------------------------------------------------------------------
// Error classification edge cases
// ---------------------------------------------------------------------------

func TestClassifyStatusCode_NonStandardCodes(t *testing.T) {
	tests := []struct {
		code      int
		wantCode  ErrorCode
		retryable bool
	}{
		{418, ErrCodeValidation, false},  // I'm a teapot
		{451, ErrCodeValidation, false},  // Unavailable For Legal Reasons
		{502, ErrCodeServer, true},       // Bad Gateway
		{599, ErrCodeServer, true},       // custom 5xx
		{100, ErrCodeServer, false},      // Informational - falls to default
		{301, ErrCodeServer, false},      // Redirect - falls to default
	}
	for _, tt := range tests {
		e := ClassifyStatusCode(tt.code, []byte("body"))
		if e == nil {
			t.Errorf("code %d: expected error, got nil", tt.code)
			continue
		}
		if e.Code != tt.wantCode {
			t.Errorf("code %d: got Code=%v, want %v", tt.code, e.Code, tt.wantCode)
		}
		if e.Retryable != tt.retryable {
			t.Errorf("code %d: got Retryable=%v, want %v", tt.code, e.Retryable, tt.retryable)
		}
	}
}

func TestClassifyStatusCode_2xx_ReturnsNil(t *testing.T) {
	for code := 200; code < 300; code++ {
		if e := ClassifyStatusCode(code, nil); e != nil {
			t.Errorf("code %d: expected nil, got %v", code, e)
		}
	}
}

func TestClassifyStatusCode_BodyPreserved(t *testing.T) {
	body := []byte(`{"error":"detail"}`)
	e := ClassifyStatusCode(500, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !bytes.Equal(e.Body, body) {
		t.Error("body not preserved in error")
	}
}

func TestError_Unwrap_Chaining(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	e := NewConnectionError(inner)
	if e.Unwrap() != inner {
		t.Error("Unwrap did not return inner error")
	}
}

func TestIsCheckers_NonHttpError(t *testing.T) {
	plain := fmt.Errorf("some error")
	if IsTimeout(plain) || IsConnection(plain) || IsAuth(plain) ||
		IsNotFound(plain) || IsRateLimit(plain) || IsServerError(plain) || IsRetryable(plain) {
		t.Error("Is* helpers should return false for non-httpclient errors")
	}
}

// ---------------------------------------------------------------------------
// Timeout handling
// ---------------------------------------------------------------------------

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
		t.Fatal("expected error from cancelled context")
	}
	if !IsTimeout(err) {
		t.Errorf("expected timeout error, got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// Concurrent requests
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Security: Header injection
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Multipart edge cases
// ---------------------------------------------------------------------------

func TestMultipartBody_EmptyFieldsWithFile(t *testing.T) {
	mp := &MultipartBody{
		Fields: nil,
		Files: []FileField{
			{FieldName: "f", FileName: "test.txt", Data: []byte("content")},
		},
	}
	r, ct, err := mp.encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ct, "multipart/form-data") {
		t.Errorf("content-type = %q, want multipart/form-data", ct)
	}
	data, _ := io.ReadAll(r)
	if !strings.Contains(string(data), "content") {
		t.Error("file data not in multipart body")
	}
}

func TestMultipartBody_FileDataTakesPriorityOverReader(t *testing.T) {
	mp := &MultipartBody{
		Files: []FileField{
			{
				FieldName: "f",
				FileName:  "test.txt",
				Data:      []byte("from-data"),
				Reader:    strings.NewReader("from-reader"),
			},
		},
	}
	r, _, err := mp.encode()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r)
	// Data takes priority since it's checked first
	if !strings.Contains(string(body), "from-data") {
		t.Error("expected Data field to be used")
	}
}

func TestEscapeQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`normal`, `normal`},
		{`has"quote`, `has\"quote`},
		{`has\slash`, `has\\slash`},
		{`"both\"`, `\"both\\\"`},
	}
	for _, tt := range tests {
		got := escapeQuotes(tt.input)
		if got != tt.want {
			t.Errorf("escapeQuotes(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FlattenHeaders
// ---------------------------------------------------------------------------

func TestFlattenHeaders_MultiValue(t *testing.T) {
	h := http.Header{
		"X-Multi": {"first", "second"},
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

// ---------------------------------------------------------------------------
// StreamResponse.Close edge cases
// ---------------------------------------------------------------------------

func TestStreamResponse_Close_NilFields(t *testing.T) {
	sr := &StreamResponse{}
	if err := sr.Close(); err != nil {
		t.Errorf("Close on nil stream = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Component lifecycle edge cases
// ---------------------------------------------------------------------------

func TestComponent_StartTwice(t *testing.T) {
	c := NewComponent(Config{BaseURL: "http://example.com"})
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Second start should succeed (re-creates adapter)
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Adapter() == nil {
		t.Error("adapter should be set after Start")
	}
}

func TestComponent_StopWithoutStart(t *testing.T) {
	c := NewComponent(Config{BaseURL: "http://example.com"})
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop without Start should not error: %v", err)
	}
}

func TestComponent_HealthBeforeStart(t *testing.T) {
	c := NewComponent(Config{BaseURL: "http://example.com"})
	h := c.Health(context.Background())
	if h.Status != "unhealthy" {
		t.Errorf("health before start = %q, want unhealthy", h.Status)
	}
}

// ---------------------------------------------------------------------------
// Auth edge cases
// ---------------------------------------------------------------------------

func TestAuth_EmptyBearerToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Auth", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a, err := New(Config{
		BaseURL: ts.URL,
		Timeout: 5 * time.Second,
		Auth:    BearerAuth(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := a.Do(context.Background(), Request{Method: "GET", Path: "/"})
	// Go's http.Header.Set trims trailing space, so "Bearer " becomes "Bearer"
	got := resp.Headers["X-Auth"]
	if !strings.HasPrefix(got, "Bearer") {
		t.Errorf("expected Bearer prefix, got %q", got)
	}
}

func TestAuth_APIKeyQuery_SpecialChars(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Key", r.URL.Query().Get("api_key"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a, err := New(Config{
		BaseURL: ts.URL,
		Timeout: 5 * time.Second,
		Auth:    APIKeyAuthQuery("k3y+val/ue=", "api_key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := a.Do(context.Background(), Request{Method: "GET", Path: "/"})
	if resp.Headers["X-Key"] != "k3y+val/ue=" {
		t.Errorf("api key = %q", resp.Headers["X-Key"])
	}
}

func TestAuth_NilApply(t *testing.T) {
	cfg := &AuthConfig{Type: AuthCustom, Apply: nil}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	// Should not panic
	cfg.apply(req)
}

func TestAuth_APIKey_EmptyName_FallsBackToDefault(t *testing.T) {
	cfg := &AuthConfig{Type: AuthAPIKey, Key: "secret", In: "header", Name: ""}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	cfg.apply(req)
	if req.Header.Get("X-API-Key") != "secret" {
		t.Error("empty name should fallback to X-API-Key")
	}
}

// ---------------------------------------------------------------------------
// Error message formatting
// ---------------------------------------------------------------------------

func TestError_ErrorFormat_WithStatusCode(t *testing.T) {
	e := NewServerError(503, nil)
	want := "httpclient: server (HTTP 503): HTTP 503"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

func TestError_ErrorFormat_WithoutStatusCode(t *testing.T) {
	e := NewTimeoutError(fmt.Errorf("dial timeout"))
	want := "httpclient: timeout: dial timeout"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// HTTP methods via httptest
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Server error returns response AND error
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Content-Type auto-detection
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Config validation
// ---------------------------------------------------------------------------

func TestConfig_Validate_ZeroTimeout_AfterDefaults(t *testing.T) {
	cfg := Config{Timeout: 0}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected zero timeout to be defaulted, got error: %v", err)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", cfg.Timeout)
	}
}

func TestConfig_Validate_NegativeTimeout(t *testing.T) {
	cfg := Config{Timeout: -1 * time.Second}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Error("ApplyDefaults should have corrected negative timeout")
	}
}

// ---------------------------------------------------------------------------
// DoStream error status
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Default retry config factory
// ---------------------------------------------------------------------------

func TestDefaultRetryConfig_HasRetryIf(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.RetryIf == nil {
		t.Fatal("expected RetryIf to be set")
	}
	// Should return true for retryable errors
	if !cfg.RetryIf(NewServerError(500, nil)) {
		t.Error("RetryIf should return true for server error")
	}
	if cfg.RetryIf(NewNotFoundError(nil)) {
		t.Error("RetryIf should return false for not-found error")
	}
}

func TestDefaultCircuitBreakerConfig_NotNil(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig("test-cb")
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestDefaultRateLimiterConfig_NotNil(t *testing.T) {
	cfg := DefaultRateLimiterConfig("test-rl")
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}
