package testhost

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestHostRejectsUntrustedTLSAndOwnsCanceledCleanup(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	untrusted := &http.Client{Timeout: time.Second}
	if response, err := untrusted.Get(host.Origin() + "/_test/ready"); err == nil {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("untrusted certificate accepted")
	}
	document := login(t, host, client, fixture)
	stream := request(t, client, http.MethodGet, host.Origin()+EventsPath, "", "")
	if stream.StatusCode != http.StatusOK {
		t.Fatal("stream setup failed")
	}
	reset := control(t, host, client, fixture, "/_test/reset", "{}")
	if reset.StatusCode != http.StatusConflict {
		t.Fatal("reset accepted an active protected stream")
	}
	status := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", document.CSRFToken)
	if status.StatusCode != http.StatusOK || len(status.Cookies()) != 0 {
		t.Fatal("status renewed a credential or active reset destroyed state")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	if err := host.Close(canceled); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 10*time.Second || host.bus.Stats().ActiveStreams != 0 {
		t.Fatal("canceled cleanup exceeded budget or retained subscriptions")
	}
}

func TestHostFailedTLSStartupReleasesItsPartialDatabase(t *testing.T) {
	t.Parallel()
	fixture, err := NewFixture()
	if err != nil {
		t.Fatal(err)
	}

	cfg := hostConfig(t)
	validCertificate := cfg.CertFile
	cfg.CertFile = filepath.Join(t.TempDir(), "missing.pem")
	if _, err := New(t.Context(), cfg, fixture); err == nil {
		t.Fatal("failed certificate setup reported success")
	}
	cfg.CertFile = validCertificate
	host, err := New(t.Context(), cfg, fixture)
	if err != nil {
		t.Fatalf("failed startup left resources unusable: %v", err)
	}
	if err := host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestDomainClockDoesNotControlStreamWriteDeadlines(t *testing.T) {
	t.Parallel()
	host, client, _ := liveHost(t)
	host.clock.Set(host.startedAt.Add(-time.Hour))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, host.Origin()+EventsPath, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-API-Key", host.APIKey())
	response, err := client.Do(req)
	if err != nil {
		t.Fatalf("domain clock changed the real HTTPS write budget: %v", err)
	}
	t.Cleanup(func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	})
	if response.StatusCode != http.StatusOK {
		t.Fatal("header credential did not establish the protected stream")
	}
}
