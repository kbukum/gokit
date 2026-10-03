package server_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/server"
	"github.com/kbukum/gokit/server/spa"
)

func TestOnDiskSPAIsBehindAPIAndConfined(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("application"), 0o600); err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(private, []byte("not-an-asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(private, filepath.Join(directory, "escape.js")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	s := server.New(&server.Config{}, nil)
	s.GinEngine().GET("/api/status", func(c *gin.Context) { c.String(http.StatusOK, "api") })
	if err := s.MountSPA(root.FS(), spa.Config{}); err != nil {
		t.Fatal(err)
	}
	s.ApplyMiddleware()
	for _, tc := range []struct {
		path   string
		status int
		body   string
	}{
		{"/api/status", 200, "api"},
		{"/deep/client/route", 200, "application"},
		{"/api/missing", 404, ""},
		{"/escape.js", 500, ""},
	} {
		request := httptest.NewRequest(http.MethodGet, tc.path, http.NoBody)
		request.Header.Set("Accept", "text/html")
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != tc.status || (tc.body != "" && response.Body.String() != tc.body) {
			t.Errorf("%s: %d %q", tc.path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "not-an-asset") {
			t.Fatal("escaped build root")
		}
	}
	if err := s.MountSPA(nil, spa.Config{}); err == nil {
		t.Fatal("invalid filesystem accepted")
	}
}
