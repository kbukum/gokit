package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/component"
)

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
