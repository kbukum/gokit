package connect

import (
	"net/http"
	"testing"
)

func TestNewServiceExposesPathAndHandler(t *testing.T) {
	handler := &testHandler{}

	svc := NewService("/pkg.Service/", handler)

	if svc.Path() != "/pkg.Service/" {
		t.Fatalf("Path() = %q, want /pkg.Service/", svc.Path())
	}
	if svc.Handler() != handler {
		t.Fatal("Handler() did not return original handler")
	}
}

type testHandler struct {
	id string
}

func (*testHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}
