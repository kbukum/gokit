package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/bootstrap"
	bootstraptest "github.com/kbukum/gokit/bootstrap/testutil"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/server"
)

var _ bootstrap.Listener = (*server.Component)(nil)

func TestComponentListenerRouteContract(t *testing.T) {
	s := newTestServer(t)
	bootstraptest.AssertListenerRoutes(t, server.NewComponent(s), s.Handler())
}

func TestComponentDescribeAndRoutes(t *testing.T) {
	s := newTestServer(t)
	s.RegisterDefaultEndpoints("svc", healthyCheck)
	s.Handle("/greeter.Greeter/", http.NotFoundHandler())

	comp := server.NewComponent(s)
	comp.Handle("/module.Service/", http.NotFoundHandler())
	if mounts := s.Mounts(); len(mounts) != 2 || mounts[1].Pattern != "/module.Service/" {
		t.Fatalf("mounts = %v", mounts)
	}
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

func TestInvalidRegistrationsReturnErrors(t *testing.T) {
	s := newTestServer(t)
	comp := server.NewComponent(s)
	ok := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if err := comp.Handle("/taken/", ok); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	for name, register := range map[string]func() error{
		"Handle nil":         func() error { return comp.Handle("/a/", nil) },
		"Handle typed nil":   func() error { return comp.Handle("/b/", http.HandlerFunc(nil)) },
		"Handle bad pattern": func() error { return comp.Handle("GET /{bad", ok) },
		"Handle conflict":    func() error { return comp.Handle("/taken/", ok) },
		"Fallback nil":       func() error { return comp.Fallback(nil) },
		"Fallback typed nil": func() error { return comp.Fallback(http.HandlerFunc(nil)) },
	} {
		if err := register(); !errors.Is(err, server.ErrInvalidRoute) {
			t.Errorf("%s: err = %v, want ErrInvalidRoute", name, err)
		}
	}
	if mounts := s.Mounts(); len(mounts) != 1 {
		t.Errorf("mounts = %v, want only /taken/", mounts)
	}
}

func TestComponentFallbackServesUnmatchedRequests(t *testing.T) {
	s := newTestServer(t)
	s.RegisterDefaultEndpoints("svc", healthyCheck)
	comp := server.NewComponent(s)
	comp.Handle("/module.Service/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("rpc")) }))
	comp.Fallback(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("spa")) }))
	s.ApplyMiddleware()
	for path, want := range map[string]string{"/module.Service/Call": "rpc", "/settings": "spa"} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		if rec.Code != http.StatusOK || rec.Body.String() != want {
			t.Errorf("GET %s = %d %q, want %q", path, rec.Code, rec.Body, want)
		}
	}
}
