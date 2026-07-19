package connect

import (
	"net/http"
	"testing"
)

type recordingMounter struct {
	patterns []string
	handlers []http.Handler
}

func (m *recordingMounter) Handle(pattern string, handler http.Handler) {
	m.patterns = append(m.patterns, pattern)
	m.handlers = append(m.handlers, handler)
}

func TestMountRegistersHandler(t *testing.T) {
	mounter := &recordingMounter{}
	handler := &testHandler{}

	Mount(mounter, "/svc.Service/", handler)

	if len(mounter.patterns) != 1 || mounter.patterns[0] != "/svc.Service/" {
		t.Fatalf("patterns = %v, want [/svc.Service/]", mounter.patterns)
	}
	if len(mounter.handlers) != 1 || mounter.handlers[0] != handler {
		t.Fatal("handler was not registered")
	}
}

func TestMountServicesRegistersEachService(t *testing.T) {
	mounter := &recordingMounter{}
	handlerA := &testHandler{id: "a"}
	handlerB := &testHandler{id: "b"}

	MountServices(
		mounter,
		NewService("/a.Service/", handlerA),
		NewService("/b.Service/", handlerB),
	)

	wantPatterns := []string{"/a.Service/", "/b.Service/"}
	if len(mounter.patterns) != len(wantPatterns) {
		t.Fatalf("registered %d services, want %d", len(mounter.patterns), len(wantPatterns))
	}
	for i, want := range wantPatterns {
		if mounter.patterns[i] != want {
			t.Fatalf("patterns[%d] = %q, want %q", i, mounter.patterns[i], want)
		}
	}
	if mounter.handlers[0] != handlerA || mounter.handlers[1] != handlerB {
		t.Fatal("services registered with wrong handlers")
	}
}

func TestMountServicesWithNoServicesIsNoop(t *testing.T) {
	mounter := &recordingMounter{}

	MountServices(mounter)

	if len(mounter.patterns) != 0 || len(mounter.handlers) != 0 {
		t.Fatalf("registered handlers for no services: patterns=%v handlers=%d", mounter.patterns, len(mounter.handlers))
	}
}
