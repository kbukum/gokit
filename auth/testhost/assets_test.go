package testhost

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSameOriginAssetsCannotShadowProtectedOrControlRoutes(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("<html>isolated-asset-fixture</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, client, _ := liveHost(t, func(c *Config) { c.AssetsDir = directory })
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, host.Origin()+"/client/route", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/html")
	page, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	data, err := io.ReadAll(io.LimitReader(page.Body, 4096))
	if err != nil {
		t.Fatal(err)
	}
	if page.StatusCode != http.StatusOK || !strings.Contains(string(data), "isolated-asset-fixture") {
		t.Fatal("same-origin asset fallback failed")
	}
	for _, path := range []string{"/auth/missing", "/events/missing", "/_test/missing", "/gokit.auth.v1.IdentityService/missing"} {
		response := request(t, client, http.MethodGet, host.Origin()+path, "", "")
		if response.StatusCode != http.StatusNotFound {
			t.Fatal("asset fallback shadowed a reserved backend route")
		}
	}
	status := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", "")
	if status.StatusCode != http.StatusUnauthorized {
		t.Fatal("asset fallback shadowed authoritative session status")
	}
}

func TestMissingAssetsFailsStartupAndReleasesDatabase(t *testing.T) {
	t.Parallel()
	cfg := hostConfig(t)
	cfg.AssetsDir = filepath.Join(t.TempDir(), "missing")
	fixture, err := NewFixture()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(t.Context(), cfg, fixture); err == nil {
		t.Fatal("missing asset root reported successful startup")
	}
	cfg.AssetsDir = ""
	host, err := New(t.Context(), cfg, fixture)
	if err != nil {
		t.Fatal("failed asset setup retained its database")
	}
	if err := host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}
