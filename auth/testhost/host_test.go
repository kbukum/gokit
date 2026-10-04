package testhost

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/codec"
	"github.com/kbukum/gokit/security/tlstest"
)

type sessionDocument struct {
	Status   string `json:"status"`
	Identity struct {
		Subject string `json:"subject"`
		Kind    string `json:"kind"`
	} `json:"identity"`
	ExpiresAt time.Time `json:"expiresAt"`
	CSRFToken string    `json:"csrfToken"`
}

func liveHost(t *testing.T, configure ...func(*Config)) (*Host, *http.Client, Fixture) {
	t.Helper()
	certs := tlstest.GenerateTLSCerts(t)
	fixture, err := NewFixture()
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Origin: "https://localhost:0", StateFile: filepath.Join(t.TempDir(), "sessions.db"),
		CertFile: certs.CertFile, KeyFile: certs.KeyFile, RunID: "integration-run", BuildID: "integration-build",
	}
	for _, change := range configure {
		change(&cfg)
	}
	host, err := New(context.Background(), cfg, fixture)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: certs.CertPool, MinVersion: tls.VersionTLS13}}
	t.Cleanup(transport.CloseIdleConnections)
	return host, &http.Client{Transport: transport, Jar: jar, Timeout: 5 * time.Second}, fixture
}

// fixtureResponse borrows a body whose lifetime is owned by the test cleanup.
type fixtureResponse struct{ *http.Response }

func request(t *testing.T, client *http.Client, method, address, body, csrf string) *fixtureResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, method, address, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://"+req.URL.Host)
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	})
	return &fixtureResponse{response}
}

func login(t *testing.T, host *Host, client *http.Client, fixture Fixture) sessionDocument {
	t.Helper()
	body, err := codec.Encode(codec.CompactJSON(), struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{"fixture-user", fixture.Password})
	if err != nil {
		t.Fatal(err)
	}
	response := request(t, client, http.MethodPost, host.Origin()+"/auth/login", body, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status %d", response.StatusCode)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "__Host-session" || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Domain != "" || cookies[0].Path != "/" {
		t.Fatal("session cookie contract mismatch")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		t.Fatal(err)
	}
	document, err := codec.Decode[sessionDocument](codec.CompactJSON(), string(data))
	if err != nil {
		t.Fatal(err)
	}
	if document.Status != "authenticated" || document.Identity.Subject != "fixture-user" || document.Identity.Kind != "user" || document.CSRFToken == "" || document.ExpiresAt.IsZero() {
		t.Fatal("session document contract mismatch")
	}
	return document
}

func TestRealHTTPSCookieConnectStreamAndLogout(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	document := login(t, host, client, fixture)
	ready := request(t, client, http.MethodGet, host.Origin()+"/_test/ready", "", "")
	data, err := io.ReadAll(io.LimitReader(ready.Body, 4096))
	if err != nil {
		t.Fatal(err)
	}

	if ready.StatusCode != http.StatusOK || !strings.Contains(string(data), ProtocolVersion) || !strings.Contains(string(data), "integration-run") {
		t.Fatal("exact readiness identity missing")
	}
	blocked := request(t, client, http.MethodPost, host.Origin()+OperationPath, "{}", "")
	if blocked.StatusCode != http.StatusForbidden {
		t.Fatalf("Connect without CSRF = %d", blocked.StatusCode)
	}
	allowed := request(t, client, http.MethodPost, host.Origin()+OperationPath, "{}", document.CSRFToken)
	if allowed.StatusCode != http.StatusOK {
		t.Fatalf("authenticated Connect = %d", allowed.StatusCode)
	}
	stream := request(t, client, http.MethodGet, host.Origin()+EventsPath, "", "")
	if stream.StatusCode != http.StatusOK || stream.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("authenticated SSE failed")
	}
	closed := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, stream.Body); closed <- err }()
	start := time.Now()
	logout := request(t, client, http.MethodPost, host.Origin()+"/auth/logout", "{}", document.CSRFToken)
	if logout.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d", logout.StatusCode)
	}
	select {
	case <-closed:
		if time.Since(start) > time.Second {
			t.Fatal("local stream release exceeded one second")
		}
	case <-time.After(time.Second):
		t.Fatal("logout retained an open stream")
	}
	status := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", "")
	if status.StatusCode != http.StatusUnauthorized || len(status.Cookies()) != 0 {
		t.Fatal("logout resurrected session or status rewrote cookie")
	}
}

func control(t *testing.T, host *Host, client *http.Client, fixture Fixture, path, body string) *fixtureResponse {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, host.Origin()+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test-Control", fixture.ControlToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	})
	return &fixtureResponse{response}
}

func TestRealHostOutageExpiryAndResetPreserveRoutes(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	document := login(t, host, client, fixture)
	unauthorized := request(t, client, http.MethodPost, host.Origin()+"/_test/reset", "{}", "")
	if unauthorized.StatusCode != http.StatusNotFound {
		t.Fatal("scenario control exposed without runner capability")
	}
	outage := control(t, host, client, fixture, "/_test/scenario", `{"name":"unavailable-store"}`)
	if outage.StatusCode != http.StatusNoContent {
		t.Fatal("store fault injection failed")
	}
	status := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", "")
	if status.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("unavailable store became successful or terminal-invalid session")
	}
	ready := request(t, client, http.MethodGet, host.Origin()+"/_test/ready", "", "")
	if ready.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("unavailable store counted as ready")
	}
	recovery := control(t, host, client, fixture, "/_test/scenario", `{"name":"healthy-store"}`)
	if recovery.StatusCode != http.StatusNoContent {
		t.Fatal("store recovery failed")
	}
	expire := control(t, host, client, fixture, "/_test/scenario", `{"name":"expired"}`)
	if expire.StatusCode != http.StatusNoContent {
		t.Fatal("expiry control failed")
	}
	blocked := request(t, client, http.MethodPost, host.Origin()+OperationPath, "{}", document.CSRFToken)
	if blocked.StatusCode != http.StatusUnauthorized {
		t.Fatal("expired session reached protected operation")
	}
	reset := control(t, host, client, fixture, "/_test/reset", "{}")
	if reset.StatusCode != http.StatusNoContent {
		t.Fatal("quiescent reset failed")
	}
	login(t, host, client, fixture)
	ready = request(t, client, http.MethodGet, host.Origin()+"/_test/ready", "", "")
	if ready.StatusCode != http.StatusOK {
		t.Fatal("reset erased routes or migration metadata")
	}
}
