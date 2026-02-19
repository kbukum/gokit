package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/server/middleware"
)

// ---------------------------------------------------------------------------
// Recovery
// ---------------------------------------------------------------------------

func TestRecovery_NoPanic(t *testing.T) {
	log := logger.NewDefault("test")
	handler := middleware.Recovery(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/", http.NoBody))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRecovery_Panic(t *testing.T) {
	log := logger.NewDefault("test")
	handler := middleware.Recovery(log)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/test", http.NoBody))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["error"] != "Internal server error" {
		t.Fatalf("unexpected error message: %s", body["error"])
	}
}

// ---------------------------------------------------------------------------
// RequestID
// ---------------------------------------------------------------------------

func TestRequestID_GeneratesID(t *testing.T) {
	handler := middleware.RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler should see the generated ID in request headers
		if r.Header.Get("X-Request-Id") == "" {
			t.Error("expected X-Request-Id in request headers")
		}
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/", http.NoBody))

	if rr.Header().Get("X-Request-Id") == "" {
		t.Error("expected X-Request-Id in response headers")
	}
}

func TestRequestID_PreservesExisting(t *testing.T) {
	handler := middleware.RequestID()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", http.NoBody)
	req.Header.Set("X-Request-Id", "custom-id-123")
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Request-Id"); got != "custom-id-123" {
		t.Fatalf("expected custom-id-123, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// CORS
// ---------------------------------------------------------------------------

func TestCORS_SetHeaders(t *testing.T) {
	cfg := &middleware.CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	}
	handler := middleware.CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", http.NoBody)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("expected https://example.com, got %s", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST" {
		t.Fatalf("expected 'GET, POST', got %s", got)
	}
}

func TestCORS_Preflight(t *testing.T) {
	cfg := &middleware.CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST"},
	}
	handler := middleware.CORS(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for OPTIONS preflight")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/api/v1/users", http.NoBody)
	req.Header.Set("Origin", "https://app.example.com")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS preflight, got %d", rr.Code)
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	cfg := &middleware.CORSConfig{
		AllowedOrigins: []string{"https://allowed.com"},
	}
	handler := middleware.CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", http.NoBody)
	req.Header.Set("Origin", "https://evil.com")
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS header for disallowed origin, got %s", got)
	}
}

func TestCORS_Credentials(t *testing.T) {
	cfg := &middleware.CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
	}
	handler := middleware.CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", http.NoBody)
	req.Header.Set("Origin", "https://app.example.com")
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected 'true', got %s", got)
	}
}

// ---------------------------------------------------------------------------
// RequestLogger
// ---------------------------------------------------------------------------

func TestRequestLogger_LogsRequest(t *testing.T) {
	log := logger.NewDefault("test")
	handler := middleware.RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("POST", "/api/users", http.NoBody))

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
}

func TestRequestLogger_SkipsHealth(t *testing.T) {
	log := logger.NewDefault("test")
	called := false
	handler := middleware.RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/health", http.NoBody))

	if !called {
		t.Error("handler should still be called for health endpoints")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// BodySizeLimit
// ---------------------------------------------------------------------------

func TestBodySizeLimit_AppliesLimit(t *testing.T) {
	handler := middleware.BodySizeLimit("1KB")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("POST", "/upload", http.NoBody))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Chain
// ---------------------------------------------------------------------------

func TestChain_Order(t *testing.T) {
	var order []string

	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1-before")
			next.ServeHTTP(w, r)
			order = append(order, "m1-after")
		})
	}
	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2-before")
			next.ServeHTTP(w, r)
			order = append(order, "m2-after")
		})
	}

	chain := middleware.Chain(m1, m2)
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/", http.NoBody))

	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Fatalf("position %d: expected %s, got %s (full: %v)", i, v, order[i], order)
		}
	}
}

// ---------------------------------------------------------------------------
// statusWriter — Flush support
// ---------------------------------------------------------------------------

type flushRecorder struct {
	http.ResponseWriter
	flushed bool
}

func (f *flushRecorder) Flush() { f.flushed = true }

func TestStatusWriter_Flush(t *testing.T) {
	fr := &flushRecorder{ResponseWriter: httptest.NewRecorder()}

	// The statusWriter is internal but we test it through RequestLogger
	// which wraps the writer. We verify streaming works by checking Flush propagation.
	log := logger.NewDefault("test")
	handler := middleware.RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(fr, httptest.NewRequest("GET", "/stream", http.NoBody))

	if !fr.flushed {
		t.Error("expected Flush to be delegated to underlying writer")
	}
}
