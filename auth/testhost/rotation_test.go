package testhost

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestReloginRetiresOldAccessButPreservesFamilyLogout(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	first := login(t, host, client, fixture)
	origin, err := url.Parse(host.Origin())
	if err != nil {
		t.Fatal(err)
	}
	oldCookies := client.Jar.Cookies(origin)
	stream := request(t, client, http.MethodGet, host.Origin()+EventsPath, "", "")
	closed := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, stream.Body); close(closed) }()
	second := login(t, host, client, fixture)
	if !second.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatal("relogin extended absolute session expiry")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("relogin retained the prior generation's stream")
	}
	oldJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	oldJar.SetCookies(origin, oldCookies)
	oldClient := &http.Client{Transport: client.Transport, Jar: oldJar, Timeout: time.Second}
	rejected := request(t, oldClient, http.MethodGet, host.Origin()+"/auth/session", "", "")
	if rejected.StatusCode != http.StatusUnauthorized {
		t.Fatal("relogin left the old browser credential active")
	}
	logout := request(t, oldClient, http.MethodPost, host.Origin()+"/auth/logout", "{}", first.CSRFToken)
	if logout.StatusCode != http.StatusNoContent {
		t.Fatal("delayed old-generation logout did not revoke its family")
	}
	status := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", "")
	if status.StatusCode != http.StatusUnauthorized {
		t.Fatal("old-generation logout left the replacement active")
	}
}

func TestLostPrivilegeRotationAndUnavailableMutationRemainClosed(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	document := login(t, host, client, fixture)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, host.Origin()+"/auth/session", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range client.Jar.Cookies(req.URL) {
		req.AddCookie(cookie)
	}
	principal, err := host.manager.Authenticate(req)
	if err != nil {
		t.Fatal(err)
	}
	record, err := host.store.Lookup(t.Context(), principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	control(t, host, client, fixture, "/_test/scenario", `{"name":"unavailable-store"}`)
	if err := host.store.Relogin(t.Context(), principal.Reference, record); apperrors.Normalize(err).Code != apperrors.ErrCodeServiceUnavailable {
		t.Fatal("relogin bypassed the authoritative-store outage")
	}
	if _, err := host.manager.Rotate(t.Context(), principal.Reference, principal); apperrors.Normalize(err).Code != apperrors.ErrCodeServiceUnavailable {
		t.Fatal("privilege rotation succeeded with unavailable storage")
	}
	control(t, host, client, fixture, "/_test/scenario", `{"name":"healthy-store"}`)
	_, err = host.manager.Rotate(t.Context(), principal.Reference, auth.Principal{
		Subject: "fixture-user", Kind: auth.User,
		Restrictions: auth.Restrictions{Mode: auth.Restricted, Resources: []string{"resource-a"}, Scopes: []string{"read"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stale := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", "")
	if stale.StatusCode != http.StatusUnauthorized {
		t.Fatal("lost rotation response preserved access through the old cookie")
	}
	logout := request(t, client, http.MethodPost, host.Origin()+"/auth/logout", "{}", document.CSRFToken)
	if logout.StatusCode != http.StatusNoContent {
		t.Fatal("lost rotation prevented old-generation family logout")
	}
}

func TestStaleRevokedCookieRecoversThroughLogin(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	first := login(t, host, client, fixture)
	origin, err := url.Parse(host.Origin())
	if err != nil {
		t.Fatal(err)
	}
	stale := client.Jar.Cookies(origin)
	// The logout commits on a client whose response the browser never sees, so the browser keeps its HttpOnly cookie.
	lostJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	lostJar.SetCookies(origin, stale)
	lost := &http.Client{Transport: client.Transport, Jar: lostJar, Timeout: time.Second}
	if logout := request(t, lost, http.MethodPost, host.Origin()+"/auth/logout", "{}", first.CSRFToken); logout.StatusCode != http.StatusNoContent {
		t.Fatal("logout failed")
	}
	if status := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", ""); status.StatusCode != http.StatusUnauthorized || len(status.Cookies()) != 0 {
		t.Fatal("revoked cookie remained authenticated or status set a cookie")
	}
	login(t, host, client, fixture)
	if fresh := client.Jar.Cookies(origin); len(fresh) != 1 || fresh[0].Value == stale[0].Value {
		t.Fatal("login did not replace the stale cookie")
	}
	if status := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", ""); status.StatusCode != http.StatusOK {
		t.Fatal("recovered browser is not authenticated")
	}
	if status := request(t, lost, http.MethodGet, host.Origin()+"/auth/session", "", ""); status.StatusCode != http.StatusUnauthorized {
		t.Fatal("stale cookie recovered access")
	}
}
