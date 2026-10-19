package spa_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kbukum/gokit/server/spa"
)

func TestSPARoutingCacheAndNonce(t *testing.T) {
	t.Parallel()
	assets := fstest.MapFS{
		"index.html":             {Data: []byte(`<html><script nonce="{nonce}" src="/assets/app.abcdef12.js"></script></html>`)},
		"assets/app.abcdef12.js": {Data: []byte(`console.log("app")`)},
	}
	handler, err := spa.New(assets, spa.Config{ImmutableAssets: []string{"assets/app.abcdef12.js"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		status int
		cache  string
	}{
		{"/", 200, "no-cache, no-store"},
		{"/runs/123", 200, "no-cache, no-store"},
		{"/runs/123/", 200, "no-cache, no-store"},
		{"/index.html", 200, "no-cache, no-store"},
		{"/api/missing", 404, ""},
		{"/rpc/missing", 404, ""},
		{"/assets/missing", 404, ""},
		{"/missing.js", 404, ""},
		{"/assets/app.abcdef12.js", 200, "public, max-age=31536000, immutable"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, tc.path, http.NoBody)
			request.Header.Set("Accept", "text/html")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status || response.Header().Get("Cache-Control") != tc.cache {
				t.Fatalf("response %d, cache %q", response.Code, response.Header().Get("Cache-Control"))
			}
			if tc.cache == "no-cache, no-store" {
				if strings.Contains(response.Body.String(), "{nonce}") || !strings.Contains(response.Header().Get("Content-Security-Policy"), "'nonce-") {
					t.Fatalf("nonce not rendered: %s", response.Body.String())
				}
			}
		})
	}
}

func TestSPAImmutableDirsCacheEveryFileBeneathThem(t *testing.T) {
	t.Parallel()
	handler, err := spa.New(fstest.MapFS{
		"index.html":                 {Data: []byte(`<html></html>`)},
		"assets/app.abcdef12.js":     {Data: []byte(`app`)},
		"assets/chunks/view.12ab.js": {Data: []byte(`view`)},
		"assets-other/plain.js":      {Data: []byte(`plain`)},
		"favicon.svg":                {Data: []byte(`<svg/>`)},
	}, spa.Config{ImmutableDirs: []string{"assets"}})
	if err != nil {
		t.Fatal(err)
	}
	for path, cache := range map[string]string{
		"/assets/app.abcdef12.js":     "public, max-age=31536000, immutable",
		"/assets/chunks/view.12ab.js": "public, max-age=31536000, immutable",
		"/assets-other/plain.js":      "no-cache",
		"/favicon.svg":                "no-cache",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != cache {
			t.Errorf("%s: %d, cache %q", path, response.Code, response.Header().Get("Cache-Control"))
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/missing.js", http.NoBody))
	if response.Code != http.StatusNotFound || response.Header().Get("Cache-Control") != "" {
		t.Fatalf("missing immutable file: %d, cache %q", response.Code, response.Header().Get("Cache-Control"))
	}
}
