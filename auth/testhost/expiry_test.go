package testhost

import (
	"net/http"
	"testing"
	"time"
)

func TestExpiredBrowserCanReauthenticateWithoutResettingState(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	first := login(t, host, client, fixture)
	expired := control(t, host, client, fixture, "/_test/scenario", `{"name":"expired"}`)
	if expired.StatusCode != http.StatusNoContent {
		t.Fatal("expiry scenario failed")
	}
	status := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", "")
	if status.StatusCode != http.StatusUnauthorized {
		t.Fatal("expired session remained authenticated")
	}
	second := login(t, host, client, fixture)
	if !second.ExpiresAt.Equal(first.ExpiresAt.Add(time.Hour)) {
		t.Fatal("expired recovery did not create the declared new absolute lifetime")
	}
	status = request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", "")
	if status.StatusCode != http.StatusOK || len(status.Cookies()) != 0 {
		t.Fatal("reauthentication failed to establish authoritative state")
	}
}
