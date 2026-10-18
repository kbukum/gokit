package spa_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/server/spa"
)

func TestDirServesOnlyWhileStarted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>app</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := spa.NewDir("web", dir, spa.Config{})
	if err != nil {
		t.Fatal(err)
	}
	serve := func() int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		return rec.Code
	}
	if code := serve(); code != http.StatusServiceUnavailable {
		t.Fatalf("unstarted ServeHTTP = %d", code)
	}
	if got := h.Health(t.Context()).Status; got != component.StatusUnhealthy {
		t.Fatalf("unstarted Health = %v", got)
	}
	if err := h.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(t.Context()); err == nil {
		t.Fatal("second Start succeeded")
	}
	if code := serve(); code != http.StatusOK {
		t.Fatalf("started ServeHTTP = %d", code)
	}
	if got := h.Health(t.Context()); got.Status != component.StatusHealthy || got.Name != "web" || h.Name() != "web" {
		t.Fatalf("started Health = %+v", got)
	}
	for range 2 {
		if err := h.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if code := serve(); code != http.StatusServiceUnavailable {
		t.Fatalf("stopped ServeHTTP = %d", code)
	}
}

func TestDirStartFailures(t *testing.T) {
	t.Parallel()
	if _, err := spa.NewDir("", t.TempDir(), spa.Config{}); err == nil {
		t.Fatal("unnamed dir accepted")
	}
	if _, err := spa.NewDir("web", "", spa.Config{}); err == nil {
		t.Fatal("empty dir accepted")
	}
	missing, err := spa.NewDir("web", filepath.Join(t.TempDir(), "missing"), spa.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := missing.Start(t.Context()); err == nil {
		t.Fatal("missing directory started")
	}
	h, err := spa.NewDir("web", t.TempDir(), spa.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(t.Context()); err == nil {
		t.Fatal("directory without an app started")
	}
	if got := h.Health(t.Context()).Status; got != component.StatusUnhealthy {
		t.Fatalf("failed Start left Health %v", got)
	}
}
