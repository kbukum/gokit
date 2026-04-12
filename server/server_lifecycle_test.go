package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/server"
	"github.com/kbukum/gokit/server/endpoint"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestConfig() *server.Config {
	cfg := &server.Config{
		Host:    "127.0.0.1",
		Port:    0,
		Enabled: true,
	}
	cfg.ApplyDefaults()
	return cfg
}

func newTestServer(t *testing.T) *server.Server {
	t.Helper()
	log := logger.NewDefault("test")
	return server.New(newTestConfig(), log)
}

// freePort asks the OS for a free port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// ---------------------------------------------------------------------------
// Server creation
// ---------------------------------------------------------------------------

func TestNew_ReturnsNonNil(t *testing.T) {
	s := newTestServer(t)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestNew_GinEngineAccessible(t *testing.T) {
	s := newTestServer(t)
	if s.GinEngine() == nil {
		t.Fatal("expected non-nil gin engine")
	}
}

func TestNew_ConfigStored(t *testing.T) {
	cfg := newTestConfig()
	cfg.Host = "127.0.0.1"
	log := logger.NewDefault("test")
	s := server.New(cfg, log)
	got := s.Config()
	if got.Host != "127.0.0.1" {
		t.Errorf("host: want 127.0.0.1, got %s", got.Host)
	}
}

func TestNew_AddrFormats(t *testing.T) {
	cfg := &server.Config{Host: "0.0.0.0", Port: 9999}
	log := logger.NewDefault("test")
	s := server.New(cfg, log)
	if got := s.Addr(); got != "0.0.0.0:9999" {
		t.Errorf("addr: want 0.0.0.0:9999, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// Config defaults and validation
// ---------------------------------------------------------------------------

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := &server.Config{}
	cfg.ApplyDefaults()

	if cfg.Port != 8080 {
		t.Errorf("port: want 8080, got %d", cfg.Port)
	}
	if cfg.ReadTimeout != 15 {
		t.Errorf("read_timeout: want 15, got %d", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 15 {
		t.Errorf("write_timeout: want 15, got %d", cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 60 {
		t.Errorf("idle_timeout: want 60, got %d", cfg.IdleTimeout)
	}
	if cfg.MaxBodySize != "10MB" {
		t.Errorf("max_body_size: want 10MB, got %s", cfg.MaxBodySize)
	}
}

func TestConfig_ApplyDefaults_PreservesExisting(t *testing.T) {
	cfg := &server.Config{Port: 3000, ReadTimeout: 30}
	cfg.ApplyDefaults()
	if cfg.Port != 3000 {
		t.Errorf("port preserved: want 3000, got %d", cfg.Port)
	}
	if cfg.ReadTimeout != 30 {
		t.Errorf("read_timeout preserved: want 30, got %d", cfg.ReadTimeout)
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := newTestConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestConfig_Validate_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"negative", -1},
		{"too_high", 70000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &server.Config{Port: tt.port}
			if err := cfg.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestConfig_Validate_NegativeTimeouts(t *testing.T) {
	tests := []struct {
		name string
		cfg  server.Config
	}{
		{"read", server.Config{ReadTimeout: -1}},
		{"write", server.Config{WriteTimeout: -1}},
		{"idle", server.Config{IdleTimeout: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Error("expected validation error for negative timeout")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Handle() mounting
// ---------------------------------------------------------------------------

func TestHandle_MountsHandler(t *testing.T) {
	s := newTestServer(t)

	called := false
	s.Handle("/test.Service/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	mounts := s.Mounts()
	if len(mounts) != 1 {
		t.Fatalf("mounts: want 1, got %d", len(mounts))
	}
	if mounts[0].Pattern != "/test.Service/" {
		t.Errorf("pattern: want /test.Service/, got %s", mounts[0].Pattern)
	}

	// Verify the handler is reachable through the server handler
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/test.Service/Method")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if !called {
		t.Error("mounted handler not called")
	}
}

func TestHandle_MultipleMounts(t *testing.T) {
	s := newTestServer(t)

	s.Handle("/svc.A/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	s.Handle("/svc.B/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	if got := len(s.Mounts()); got != 2 {
		t.Errorf("mounts: want 2, got %d", got)
	}
}

func TestMounts_EmptyByDefault(t *testing.T) {
	s := newTestServer(t)
	if mounts := s.Mounts(); len(mounts) != 0 {
		t.Errorf("want 0 mounts, got %d", len(mounts))
	}
}

// ---------------------------------------------------------------------------
// ApplyMiddleware
// ---------------------------------------------------------------------------

func TestApplyMiddleware_ChangesHandler(t *testing.T) {
	s := newTestServer(t)

	before := s.Handler()
	s.ApplyMiddleware()
	after := s.Handler()

	// After applying middleware the handler should be a new wrapped handler
	if fmt.Sprintf("%p", before) == fmt.Sprintf("%p", after) {
		t.Error("handler should be different after ApplyMiddleware")
	}
}

func TestApplyMiddleware_AddsRequestID(t *testing.T) {
	s := newTestServer(t)
	s.GinEngine().GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.Header.Get("X-Request-Id") == "" {
		t.Error("expected X-Request-Id header after middleware")
	}
}

func TestApplyMiddleware_RecoveryHandlesPanic(t *testing.T) {
	s := newTestServer(t)
	s.GinEngine().GET("/boom", func(_ *gin.Context) {
		panic("test panic")
	})
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/boom")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Start / Stop lifecycle
// ---------------------------------------------------------------------------

func TestStart_BindsPort(t *testing.T) {
	port := freePort(t)
	cfg := &server.Config{Host: "127.0.0.1", Port: port}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	s := server.New(cfg, log)

	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop(ctx)

	// Port should be bound — connecting should work
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("could not connect to port %d: %v", port, err)
	}
	conn.Close()
}

func TestStart_ServesHTTP(t *testing.T) {
	port := freePort(t)
	cfg := &server.Config{Host: "127.0.0.1", Port: port}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	s := server.New(cfg, log)

	s.GinEngine().GET("/hello", func(c *gin.Context) {
		c.String(http.StatusOK, "world")
	})
	s.ApplyMiddleware()

	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop(ctx)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/hello", port))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "world" {
		t.Errorf("body: want 'world', got %q", body)
	}
}

func TestStop_GracefulShutdown(t *testing.T) {
	port := freePort(t)
	cfg := &server.Config{Host: "127.0.0.1", Port: port}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	s := server.New(cfg, log)

	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// After stop, port should no longer accept connections
	time.Sleep(50 * time.Millisecond) // give goroutine time to finish
	_, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err == nil {
		t.Error("expected connection refused after stop")
	}
}

func TestStart_PortInUse(t *testing.T) {
	// Bind a port so the server can't use it
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	cfg := &server.Config{Host: "127.0.0.1", Port: port}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	s := server.New(cfg, log)

	ctx := context.Background()
	if err := s.Start(ctx); err == nil {
		s.Stop(ctx)
		t.Fatal("expected error when port in use")
	}
}

func TestStart_CancelledContext(t *testing.T) {
	cfg := &server.Config{Host: "127.0.0.1", Port: freePort(t)}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	s := server.New(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before start

	// A canceled context may or may not prevent Listen depending on OS timing.
	// We just verify it doesn't panic.
	err := s.Start(ctx)
	if err == nil {
		s.Stop(context.Background())
	}
}

// ---------------------------------------------------------------------------
// RegisterDefaultEndpoints
// ---------------------------------------------------------------------------

func TestRegisterDefaultEndpoints_Health(t *testing.T) {
	s := newTestServer(t)

	checker := func(_ context.Context) []component.Health {
		return []component.Health{
			{Name: "db", Status: component.StatusHealthy},
		}
	}
	s.RegisterDefaultEndpoints("test-service", checker)
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["status"] != "healthy" {
		t.Errorf("health status: want healthy, got %v", result["status"])
	}
	if result["service"] != "test-service" {
		t.Errorf("service: want test-service, got %v", result["service"])
	}
}

func TestRegisterDefaultEndpoints_HealthUnhealthy(t *testing.T) {
	s := newTestServer(t)

	checker := func(_ context.Context) []component.Health {
		return []component.Health{
			{Name: "db", Status: component.StatusUnhealthy, Message: "connection lost"},
		}
	}
	s.RegisterDefaultEndpoints("test-service", checker)
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status: want 503, got %d", resp.StatusCode)
	}
}

func TestRegisterDefaultEndpoints_Info(t *testing.T) {
	s := newTestServer(t)
	s.RegisterDefaultEndpoints("test-service", nil)
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/info")
	if err != nil {
		t.Fatalf("GET /info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["service"] != "test-service" {
		t.Errorf("service: want test-service, got %v", result["service"])
	}
}

func TestRegisterDefaultEndpoints_Metrics(t *testing.T) {
	s := newTestServer(t)
	s.RegisterDefaultEndpoints("test-service", nil)
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["goroutines"]; !ok {
		t.Error("expected goroutines in metrics response")
	}
}

// ---------------------------------------------------------------------------
// ApplyDefaults
// ---------------------------------------------------------------------------

func TestApplyDefaults_RegistersEndpointsAndMiddleware(t *testing.T) {
	s := newTestServer(t)
	checker := func(_ context.Context) []component.Health {
		return nil
	}
	s.ApplyDefaults("svc", checker)

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	for _, path := range []string{"/health", "/info", "/metrics"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: want 200, got %d", path, resp.StatusCode)
		}
	}
}

// ---------------------------------------------------------------------------
// Handler() returns a usable http.Handler
// ---------------------------------------------------------------------------

func TestHandler_ServesGinRoutes(t *testing.T) {
	s := newTestServer(t)
	s.GinEngine().GET("/api/v1/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/data")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// h2c (HTTP/2 cleartext) support
// ---------------------------------------------------------------------------

func TestHandler_H2C_ResponseHeaders(t *testing.T) {
	s := newTestServer(t)
	s.GinEngine().GET("/h2c-test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Standard HTTP/1.1 should still work through h2c handler
	resp, err := http.Get(ts.URL + "/h2c-test")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Concurrent handler registration
// ---------------------------------------------------------------------------

func TestHandle_SequentialMultipleRegistration(t *testing.T) {
	s := newTestServer(t)

	for i := 0; i < 10; i++ {
		pattern := fmt.Sprintf("/svc.S%d/", i)
		s.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	}

	if got := len(s.Mounts()); got != 10 {
		t.Errorf("mounts after registration: want 10, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Component wrapper
// ---------------------------------------------------------------------------

func TestComponent_Name(t *testing.T) {
	s := newTestServer(t)
	c := server.NewComponent(s)
	if name := c.Name(); name != "http-server" {
		t.Errorf("name: want http-server, got %s", name)
	}
}

func TestComponent_ImplementsInterface(t *testing.T) {
	s := newTestServer(t)
	c := server.NewComponent(s)
	var _ component.Component = c
}

func TestComponent_HealthWhenInitialized(t *testing.T) {
	s := newTestServer(t)
	c := server.NewComponent(s)

	h := c.Health(context.Background())
	if h.Status != component.StatusHealthy {
		t.Errorf("health: want healthy, got %s", h.Status)
	}
}

func TestComponent_Describe(t *testing.T) {
	cfg := &server.Config{Host: "0.0.0.0", Port: 8080}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	s := server.New(cfg, log)
	c := server.NewComponent(s)

	desc := c.Describe()
	if desc.Name != "HTTP Server" {
		t.Errorf("describe name: want 'HTTP Server', got %q", desc.Name)
	}
	if desc.Port != 8080 {
		t.Errorf("describe port: want 8080, got %d", desc.Port)
	}
}

func TestComponent_DescribeWithMounts(t *testing.T) {
	s := newTestServer(t)
	s.Handle("/my_pkg.MyService/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	c := server.NewComponent(s)
	desc := c.Describe()
	if desc.Details == "" {
		t.Error("expected non-empty details with mounts")
	}
}

func TestComponent_Routes(t *testing.T) {
	s := newTestServer(t)
	s.GinEngine().GET("/users", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	s.GinEngine().POST("/users", func(c *gin.Context) {
		c.String(http.StatusCreated, "ok")
	})
	c := server.NewComponent(s)

	routes := c.Routes()
	if len(routes) < 2 {
		t.Fatalf("routes: want >= 2, got %d", len(routes))
	}
}

func TestComponent_RoutesIncludeMounts(t *testing.T) {
	s := newTestServer(t)
	s.Handle("/svc.Test/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	c := server.NewComponent(s)
	routes := c.Routes()

	found := false
	for _, r := range routes {
		if r.Method == "CONNECT" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected CONNECT route for mounted handler")
	}
}

func TestComponent_StartStop(t *testing.T) {
	port := freePort(t)
	cfg := &server.Config{Host: "127.0.0.1", Port: port}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	s := server.New(cfg, log)
	c := server.NewComponent(s)

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Should be reachable
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	conn.Close()

	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Health endpoint with degraded component
// ---------------------------------------------------------------------------

func TestHealth_DegradedComponent(t *testing.T) {
	s := newTestServer(t)

	checker := func(_ context.Context) []component.Health {
		return []component.Health{
			{Name: "cache", Status: component.StatusDegraded, Message: "slow"},
		}
	}
	s.RegisterDefaultEndpoints("test-service", checker)
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	// Degraded should still return 200
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "degraded" {
		t.Errorf("health status: want degraded, got %v", result["status"])
	}
}

func TestHealth_NilChecker(t *testing.T) {
	s := newTestServer(t)
	s.RegisterDefaultEndpoints("test-service", nil)
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Endpoint-level tests
// ---------------------------------------------------------------------------

func TestLivenessEndpoint(t *testing.T) {
	s := newTestServer(t)
	s.GinEngine().GET("/livez", endpoint.Liveness("test-svc"))
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/livez")
	if err != nil {
		t.Fatalf("GET /livez: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "alive" {
		t.Errorf("liveness status: want alive, got %v", result["status"])
	}
}

func TestReadinessEndpoint_Ready(t *testing.T) {
	s := newTestServer(t)
	checker := func(_ context.Context) []component.Health {
		return []component.Health{
			{Name: "db", Status: component.StatusHealthy},
		}
	}
	s.GinEngine().GET("/readyz", endpoint.Readiness("test-svc", checker))
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "ready" {
		t.Errorf("readiness: want ready, got %v", result["status"])
	}
}

func TestReadinessEndpoint_NotReady(t *testing.T) {
	s := newTestServer(t)
	checker := func(_ context.Context) []component.Health {
		return []component.Health{
			{Name: "db", Status: component.StatusUnhealthy, Message: "down"},
		}
	}
	s.GinEngine().GET("/readyz", endpoint.Readiness("test-svc", checker))
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status: want 503, got %d", resp.StatusCode)
	}
}

func TestReadinessEndpoint_NilChecker(t *testing.T) {
	s := newTestServer(t)
	s.GinEngine().GET("/readyz", endpoint.Readiness("test-svc", nil))
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Multiple Start/Stop cycles
// ---------------------------------------------------------------------------

func TestStartStop_MultipleCycles(t *testing.T) {
	// Verify a server can be stopped without error even if started fresh each time.
	// Note: Go's http.Server cannot be restarted after Shutdown, so we create new ones.
	for i := 0; i < 3; i++ {
		port := freePort(t)
		cfg := &server.Config{Host: "127.0.0.1", Port: port}
		cfg.ApplyDefaults()
		log := logger.NewDefault("test")
		s := server.New(cfg, log)

		ctx := context.Background()
		if err := s.Start(ctx); err != nil {
			t.Fatalf("cycle %d start: %v", i, err)
		}
		if err := s.Stop(ctx); err != nil {
			t.Fatalf("cycle %d stop: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrent requests during shutdown
// ---------------------------------------------------------------------------

func TestStop_DuringActiveRequests(t *testing.T) {
	port := freePort(t)
	cfg := &server.Config{Host: "127.0.0.1", Port: port}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	s := server.New(cfg, log)

	reqStarted := make(chan struct{})
	s.GinEngine().GET("/slow", func(c *gin.Context) {
		close(reqStarted)
		time.Sleep(200 * time.Millisecond)
		c.String(http.StatusOK, "done")
	})
	s.ApplyMiddleware()

	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Start a slow request
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/slow", port))
		if err != nil {
			return // connection reset is acceptable during shutdown
		}
		resp.Body.Close()
	}()

	<-reqStarted
	// Initiate graceful shutdown while the request is in flight
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// CORS through middleware
// ---------------------------------------------------------------------------

func TestMiddleware_CORS_PreflightPassthrough(t *testing.T) {
	s := newTestServer(t)
	s.GinEngine().POST("/api/data", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	s.ApplyMiddleware()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("OPTIONS", ts.URL+"/api/data", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS: %v", err)
	}
	resp.Body.Close()

	if resp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected Access-Control-Allow-Origin header")
	}
}
