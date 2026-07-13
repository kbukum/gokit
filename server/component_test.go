package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/server"
)

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
