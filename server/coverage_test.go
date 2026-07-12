package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server"
)

const specJSON = `{"swagger":"2.0","host":"old","basePath":"/old","info":{"title":"t","version":"1"},"paths":{}}`

func testLogger() *logging.Logger { return logging.NewDefault("test") }

// healthyCheck is a minimal endpoint.HealthChecker for endpoint registration tests.
func healthyCheck(context.Context) []component.Health {
	return []component.Health{{Name: "x", Status: component.StatusHealthy}}
}

func TestListenAddrAndConfig(t *testing.T) {
	s := newTestServer(t)
	if s.ListenAddr() != nil {
		t.Fatalf("ListenAddr should be nil before Start")
	}
	if !s.Config().Enabled {
		t.Fatalf("Config().Enabled should be true")
	}
}

func TestRegisterDefaultEndpoints(t *testing.T) {
	s := newTestServer(t)
	s.RegisterDefaultEndpoints("svc", healthyCheck)
	s.ApplyMiddleware()

	for _, path := range []string{"/health", "/healthz", "/livez", "/readyz", "/info", "/metrics"} {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code >= 500 {
			t.Fatalf("%s: status %d", path, w.Code)
		}
	}
}

func TestRegisterPprof(t *testing.T) {
	s := newTestServer(t)
	s.RegisterPprof()
	s.ApplyMiddleware()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", http.NoBody)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pprof heap: status %d", w.Code)
	}
}

func TestMountDocsFromConfig(t *testing.T) {
	// disabled → no-op
	s := newTestServer(t)
	s.MountDocsFromConfig([]byte(specJSON))

	// enabled with inline spec bytes
	cfg := newTestConfig()
	cfg.Docs.Enabled = true
	s2 := server.New(cfg, testLogger())
	s2.MountDocsFromConfig([]byte(specJSON))
	s2.ApplyMiddleware()

	req := httptest.NewRequest(http.MethodGet, "/docs", http.NoBody)
	w := httptest.NewRecorder()
	s2.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/docs status %d", w.Code)
	}

	// enabled but no spec → warn + no-op (should not panic)
	cfg2 := newTestConfig()
	cfg2.Docs.Enabled = true
	server.New(cfg2, testLogger()).MountDocsFromConfig()
}

func TestMountDocsFromConfig_SpecFile(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(specPath, []byte(specJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.Docs.Enabled = true
	cfg.Docs.SpecFile = specPath
	s := server.New(cfg, testLogger())
	s.MountDocsFromConfig()
	s.ApplyMiddleware()

	req := httptest.NewRequest(http.MethodGet, "/docs", http.NoBody)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/docs status %d", w.Code)
	}

	// missing file → error path, no panic
	cfg2 := newTestConfig()
	cfg2.Docs.Enabled = true
	cfg2.Docs.SpecFile = filepath.Join(dir, "missing.json")
	server.New(cfg2, testLogger()).MountDocsFromConfig()
}

func TestComponentDescribeAndRoutes(t *testing.T) {
	s := newTestServer(t)
	s.RegisterDefaultEndpoints("svc", healthyCheck)
	s.Handle("/greeter.Greeter/", http.NotFoundHandler())

	comp := server.NewComponent(s)
	if desc := comp.Describe(); desc.Name != "HTTP Server" {
		t.Fatalf("describe name = %q", desc.Name)
	}
	if len(comp.Routes()) == 0 {
		t.Fatalf("expected routes")
	}
	if h := comp.Health(context.Background()); h.Status != component.StatusHealthy {
		t.Fatalf("health = %v", h.Status)
	}
}
