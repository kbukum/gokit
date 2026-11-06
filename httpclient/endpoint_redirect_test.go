package httpclient

import (
	"net"
	"net/http"
	"testing"

	"github.com/kbukum/gokit/security"
)

func redirectHop(t *testing.T, method string) *http.Request {
	t.Helper()
	return redirectTo(t, method, "https://cdn.example/blob")
}

func redirectTo(t *testing.T, method, target string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Range", "bytes=0-9")
	req.Header.Set("User-Agent", "kit")
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-Api-Key", "secret")
	req.Header.Set("Cookie", "session=secret")
	return req
}

func TestEndpointRedirectFollowsSafeReadsWithoutCredentials(t *testing.T) {
	t.Parallel()
	check := endpointRedirect(2)
	origin := redirectTo(t, http.MethodGet, "https://hub.example/file")
	hop := redirectHop(t, http.MethodGet)
	if err := check(hop, []*http.Request{origin}); err != nil {
		t.Fatalf("redirect refused: %v", err)
	}
	for _, name := range []string{"Authorization", "X-Api-Key", "Cookie"} {
		if hop.Header.Get(name) != "" {
			t.Fatalf("redirect forwarded %s", name)
		}
	}
	if hop.Header.Get("Range") != "bytes=0-9" || hop.Header.Get("Accept") == "" || hop.Header.Get("User-Agent") != "kit" {
		t.Fatalf("redirect dropped safe headers: %v", hop.Header)
	}
}

func TestEndpointRedirectKeepsCredentialsOnlyWhileOnTheOrigin(t *testing.T) {
	t.Parallel()
	check := endpointRedirect(3)
	origin := redirectTo(t, http.MethodGet, "https://hub.example/repo/resolve/main/README.md")
	same := redirectTo(t, http.MethodGet, "https://hub.example/api/resolve-cache/README.md")
	if err := check(same, []*http.Request{origin}); err != nil {
		t.Fatal(err)
	}
	if same.Header.Get("Authorization") == "" {
		t.Fatal("a same-origin redirect dropped the credentials a gated file needs")
	}
	// Once a hop has left the origin, coming back does not restore credentials.
	back := redirectTo(t, http.MethodGet, "https://hub.example/again")
	if err := check(back, []*http.Request{origin, redirectHop(t, http.MethodGet)}); err != nil {
		t.Fatal(err)
	}
	if back.Header.Get("Authorization") != "" || back.Header.Get("Cookie") != "" {
		t.Fatal("credentials returned after a cross-origin hop")
	}
	port := redirectTo(t, http.MethodGet, "https://hub.example:8443/x")
	if err := check(port, []*http.Request{origin}); err != nil {
		t.Fatal(err)
	}
	if port.Header.Get("Authorization") != "" {
		t.Fatal("a different port kept credentials")
	}
}

func TestEndpointRedirectRefusesUnsafeOrExcessHops(t *testing.T) {
	t.Parallel()
	check := endpointRedirect(1)
	if err := check(redirectHop(t, http.MethodPost), []*http.Request{redirectHop(t, http.MethodPost)}); err == nil {
		t.Fatal("redirect of a non-read request was followed")
	}
	via := []*http.Request{redirectHop(t, http.MethodGet), redirectHop(t, http.MethodGet)}
	if err := check(redirectHop(t, http.MethodGet), via); err == nil {
		t.Fatal("redirect past the hop limit was followed")
	}
}

func TestConfigValidatesEndpointRedirects(t *testing.T) {
	t.Parallel()
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, net.DefaultResolver, &net.Dialer{})
	if err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]Config{
		"negative":       {EndpointPolicy: policy, EndpointRedirects: -1},
		"too many":       {EndpointPolicy: policy, EndpointRedirects: maxEndpointRedirects + 1},
		"without policy": {EndpointRedirects: 1},
	} {
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: Validate() accepted invalid redirects", name)
		}
	}
	ok := Config{EndpointPolicy: policy, EndpointRedirects: 3}
	ok.ApplyDefaults()
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}
