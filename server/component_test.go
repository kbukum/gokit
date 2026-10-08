package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/server"
)

var _ bootstrap.Listener = (*server.Component)(nil)

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
