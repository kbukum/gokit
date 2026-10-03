package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/component"
	componenttest "github.com/kbukum/gokit/component/testutil"
	"github.com/kbukum/gokit/server"
)

func TestIngressRejectsBeforeDependenciesQuiesce(t *testing.T) {
	s := server.New(&server.Config{Host: "127.0.0.1"}, nil)
	s.ApplyDefaults("quiesce", nil)
	registry := component.NewRegistry()
	dependency := &componenttest.Component{ComponentName: "dependency", QuiesceFunc: func() error {
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/livez", http.NoBody))
		if response.Code != 503 {
			t.Errorf("dependency quiesced while ingress still admitted requests: %d", response.Code)
		}
		return nil
	}}
	if err := registry.Register(dependency); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(server.NewComponent(s)); err != nil {
		t.Fatal(err)
	}
	if err := registry.StartAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := registry.QuiesceAll(); err != nil {
		t.Fatal(err)
	}
	if err := registry.StopAll(t.Context()); err != nil {
		t.Fatal(err)
	}
}
