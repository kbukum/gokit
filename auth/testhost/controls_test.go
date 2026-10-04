package testhost

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kbukum/gokit/component"
)

func TestScenarioControlsRejectInvalidInputAndConcurrentHolds(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	for _, input := range []string{"not-json", `{"name":"unknown"}`, strings.Repeat("x", 257)} {
		response := control(t, host, client, fixture, "/_test/scenario", input)
		if response.StatusCode != http.StatusUnprocessableEntity || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("invalid scenario input was not an explicit non-cacheable failure")
		}
	}
	first := control(t, host, client, fixture, "/_test/scenario", `{"name":"hold-status"}`)
	second := control(t, host, client, fixture, "/_test/scenario", `{"name":"hold-status"}`)
	if first.StatusCode != http.StatusNoContent || second.StatusCode != http.StatusConflict {
		t.Fatal("multiple status holds were accepted")
	}
	if err := host.Close(t.Context()); err != nil {
		t.Fatal("unclaimed status hold prevented owned shutdown")
	}
}

func TestHealthTracksAuthoritativeStoreAndShutdown(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	if host.Health(t.Context()).Status != component.StatusHealthy {
		t.Fatal("started host did not report authoritative health")
	}
	if err := host.Start(t.Context()); err == nil {
		t.Fatal("duplicate lifecycle start acquired another environment")
	}
	control(t, host, client, fixture, "/_test/scenario", `{"name":"unavailable-store"}`)
	if host.Health(t.Context()).Status != component.StatusUnhealthy {
		t.Fatal("unavailable authoritative store reported healthy")
	}
	login := request(t, client, http.MethodPost, host.Origin()+"/auth/login", `{"username":"fixture-user","password":"invalid"}`, "")
	if login.StatusCode != http.StatusUnauthorized {
		t.Fatal("invalid login created a session")
	}
	control(t, host, client, fixture, "/_test/scenario", `{"name":"healthy-store"}`)
	if err := host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if host.Health(t.Context()).Status != component.StatusUnhealthy {
		t.Fatal("closed database reported healthy")
	}
}
