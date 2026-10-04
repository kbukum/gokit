package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/testhost"
	"github.com/kbukum/gokit/codec"
	"github.com/kbukum/gokit/security/tlstest"
)

func TestInitFixtureIsPrivateAndNeverOverwrites(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "fixture.json")
	var output bytes.Buffer
	if err := run(t.Context(), []string{"-init-fixture", path}, &output); err != nil {
		t.Fatal(err)
	}
	fixture, err := testhost.LoadFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(output.Bytes(), []byte(fixture.Password)) || bytes.Contains(output.Bytes(), []byte(fixture.ControlToken)) {
		t.Fatal("fixture secrets leaked to output")
	}
	if err := run(t.Context(), []string{"-init-fixture", path}, &output); err == nil {
		t.Fatal("existing private fixture overwritten")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("fixture permissions differ from 0600")
	}
}

func TestIncompleteServeConfigFailsBeforeListening(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"-fixture", "/does/not/exist"}, {"-origin", "http://localhost:4443"}, {"-unknown"}, {"positional"}, {"-init-fixture", "relative.json"}} {
		if err := run(t.Context(), args, &bytes.Buffer{}); err == nil {
			t.Fatal("incomplete/insecure host config started")
		}
	}
}

type announcementWriter struct{ records chan string }

func (w announcementWriter) Write(data []byte) (int, error) {
	if len(data) > 4096 {
		return 0, errors.New("announcement exceeded its test budget")
	}
	select {
	case w.records <- string(data):
		return len(data), nil
	default:
		return 0, errors.New("announcement writer is full")
	}
}

type unavailableWriter struct{}

func (unavailableWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}

func TestServeOwnsTrustedReadinessAndCanceledShutdown(t *testing.T) {
	t.Parallel()
	certs := tlstest.GenerateTLSCerts(t)
	fixturePath := filepath.Join(t.TempDir(), "fixture.json")
	if err := initializeFixture(fixturePath, io.Discard); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"-fixture", fixturePath, "-origin", "https://localhost:0",
		"-cert", certs.CertFile, "-key", certs.KeyFile,
		"-state", filepath.Join(t.TempDir(), "sessions.db"),
		"-run-id", "cli-run", "-build-id", "cli-build",
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("CLI retained owned resources after cancellation")
		}
	})
	output := announcementWriter{records: make(chan string, 1)}
	go func() { done <- run(ctx, args, output) }()
	var announcement string
	select {
	case announcement = <-output.records:
	case err := <-done:
		done <- err
		t.Fatalf("CLI stopped before readiness: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("CLI did not announce bounded readiness")
	}
	fixture, err := testhost.LoadFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(announcement, fixture.Password) || strings.Contains(announcement, fixture.ControlToken) {
		t.Fatal("CLI announcement contains private fixture material")
	}
	document, err := codec.Decode[struct {
		Origin   string `json:"origin"`
		Protocol string `json:"protocol"`
		RunID    string `json:"runId"`
		BuildID  string `json:"buildId"`
	}](codec.CompactJSON(), strings.TrimPrefix(announcement, "auth-host-ready "))
	if err != nil {
		t.Fatal(err)
	}
	if document.Protocol != testhost.ProtocolVersion || document.RunID != "cli-run" || document.BuildID != "cli-build" {
		t.Fatal("CLI announcement lost exact readiness identity")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: certs.CertPool, MinVersion: tls.VersionTLS13}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: time.Second}
	response, err := client.Get(document.Origin + "/_test/ready")
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatal("CLI announced an unready host")
	}
	cancel()
	select {
	case err := <-done:
		done <- err
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("CLI shutdown exceeded ten seconds")
	}
	if response, err := client.Get(document.Origin + "/_test/ready"); err == nil {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("CLI retained its HTTPS listener after shutdown")
	}
	if err := run(t.Context(), args, unavailableWriter{}); err == nil {
		t.Fatal("failed announcement reported successful startup")
	}
}
