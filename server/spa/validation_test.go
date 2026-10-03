package spa_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kbukum/gokit/server/spa"
)

func TestSPARejectsInvalidBuildAndConfiguration(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{"index.html": {Data: []byte("index")}}
	for _, tc := range []struct {
		name  string
		files fs.FS
		cfg   spa.Config
	}{
		{"nil files", nil, spa.Config{}},
		{"missing index", fstest.MapFS{}, spa.Config{}},
		{"oversized index", fstest.MapFS{"index.html": {Data: make([]byte, 1024*1024+1)}}, spa.Config{}},
		{"unsafe CSP", files, spa.Config{CSP: "script-src 'unsafe-inline'"}},
		{"immutable index", files, spa.Config{ImmutableAssets: []string{"index.html"}}},
		{"invalid asset", files, spa.Config{ImmutableAssets: []string{"../private"}}},
		{"missing asset", files, spa.Config{ImmutableAssets: []string{"missing.js"}}},
		{"invalid prefix", files, spa.Config{ReservedPrefixes: []string{"/"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := spa.New(tc.files, tc.cfg); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestSPANonceMatchesBodyAndChangesPerRequest(t *testing.T) {
	t.Parallel()
	handler, err := spa.New(fstest.MapFS{
		"index.html":  {Data: []byte(`<script nonce="{nonce}"></script>`)},
		"favicon.svg": {Data: []byte(`<svg/>`)},
	}, spa.Config{ReservedPrefixes: []string{"/custom-api"}})
	if err != nil {
		t.Fatal(err)
	}
	var previous string
	for range 2 {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		body := response.Body.String()
		nonce := strings.TrimSuffix(strings.TrimPrefix(body, `<script nonce="`), `"></script>`)
		if nonce == previous || !strings.Contains(response.Header().Get("Content-Security-Policy"), "'nonce-"+nonce+"'") {
			t.Fatalf("nonce mismatch/reuse: %s", body)
		}
		previous = nonce
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodPost, "/", 404},
		{http.MethodHead, "/", 200},
		{http.MethodGet, "/custom-api/unknown", 404},
		{http.MethodGet, "/../secret", 404},
		{http.MethodGet, "/favicon.svg", 200},
		{http.MethodGet, "/not-html-navigation", 404},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, http.NoBody))
		if response.Code != tc.status {
			t.Errorf("%s %s: %d", tc.method, tc.path, response.Code)
		}
		if tc.method == http.MethodHead && response.Body.Len() != 0 {
			t.Fatal("HEAD returned a body")
		}
		if tc.path == "/favicon.svg" && response.Header().Get("Cache-Control") != "no-cache" {
			t.Fatal("unfingerprinted asset cached immutably")
		}
	}
}
