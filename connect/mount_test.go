package connect

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

type recordingMounter struct {
	patterns []string
	handlers []http.Handler
	failOn   string
}

var errMountRejected = errors.New("rejected")

func (m *recordingMounter) Handle(pattern string, handler http.Handler) error {
	if pattern == m.failOn {
		return errMountRejected
	}
	m.patterns = append(m.patterns, pattern)
	m.handlers = append(m.handlers, handler)
	return nil
}

func TestMountRegistersHandler(t *testing.T) {
	mounter := &recordingMounter{}
	handler := &testHandler{}

	if err := Mount(mounter, "/svc.Service/", handler); err != nil {
		t.Fatal(err)
	}

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

	if err := MountServices(
		mounter,
		NewService("/a.Service/", handlerA),
		NewService("/b.Service/", handlerB),
	); err != nil {
		t.Fatal(err)
	}

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

	if err := MountServices(mounter); err != nil {
		t.Fatal(err)
	}

	if len(mounter.patterns) != 0 || len(mounter.handlers) != 0 {
		t.Fatalf("registered handlers for no services: patterns=%v handlers=%d", mounter.patterns, len(mounter.handlers))
	}
}

func TestMountServicesStopsAtTheFirstFailure(t *testing.T) {
	mounter := &recordingMounter{failOn: "/b.Service/"}
	err := MountServices(mounter,
		NewService("/a.Service/", &testHandler{}),
		NewService("/b.Service/", &testHandler{}),
		NewService("/c.Service/", &testHandler{}),
	)
	if !errors.Is(err, errMountRejected) || !strings.Contains(err.Error(), "/b.Service/") {
		t.Fatalf("MountServices = %v, want the /b.Service/ rejection", err)
	}
	if len(mounter.patterns) != 1 {
		t.Fatalf("patterns = %v, want only /a.Service/", mounter.patterns)
	}
}
