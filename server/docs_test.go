package server_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server"
)

const specJSON = `{"swagger":"2.0","host":"old","basePath":"/old","info":{"title":"t","version":"1"},"paths":{}}`

func testLogger() *logging.Logger { return logging.NewDefault("test") }

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
