//go:build integration

package testhost

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/codec"
	kitfs "github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/process"
	"github.com/kbukum/gokit/security/tlstest"
)

func TestBuiltHostReadinessAndOwnedProcessCleanup(t *testing.T) {
	binary := os.Getenv("GOKIT_AUTH_HOST_BINARY")
	if !filepath.IsAbs(binary) {
		t.Fatal("required prerequisite: make auth-host-build and set GOKIT_AUTH_HOST_BINARY to the absolute built binary")
	}
	certs := tlstest.GenerateTLSCerts(t)
	fixture, err := NewFixture()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	fixturePath := filepath.Join(directory, "fixture.json")
	data, err := codec.Encode(codec.CompactJSON(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := kitfs.WriteAtomic(fixturePath, []byte(data), "fixture"); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"-fixture", fixturePath, "-state", filepath.Join(directory, "sessions.db"),
		"-origin", "https://localhost:0", "-cert", certs.CertFile, "-key", certs.KeyFile,
		"-run-id", "owned-process-run", "-build-id", "owned-process-build",
	}
	cfg := process.DefaultPersistentConfig()
	cfg.Readiness, cfg.OutputMarker = process.ReadyOnOutput, "auth-host-ready "
	cfg.ReadinessTimeout, cfg.ShutdownGracePeriod = 30*time.Second, 10*time.Second
	cfg.Lifecycle.GracePeriod = 10 * time.Second
	command := process.Command{Binary: binary, Args: args, EnvPolicy: process.EnvEmpty, MaxOutputBytes: 64 << 10}
	run, err := process.StartPersistent(t.Context(), command, cfg)
	if run != nil {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			outcome, err := run.Process.Shutdown(ctx)
			if err != nil || !outcome.Complete {
				t.Errorf("owned host cleanup: complete=%v error=%v", outcome.Complete, err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	var origin string
	for _, line := range strings.Split(string(run.Startup.Stdout), "\n") {
		if strings.HasPrefix(line, cfg.OutputMarker) {
			value, err := codec.Decode[struct {
				Origin string `json:"origin"`
			}](codec.CompactJSON(), strings.TrimPrefix(line, cfg.OutputMarker))
			if err != nil {
				t.Fatal(err)
			}
			origin = value.Origin
		}
	}
	address, err := url.Parse(origin)
	if err != nil || address.Scheme != "https" || address.Hostname() != "localhost" || address.Port() == "" {
		t.Fatal("host did not publish an exact HTTPS listener")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: certs.CertPool, MinVersion: tls.VersionTLS13}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: time.Second}
	response := request(t, client, http.MethodGet, origin+"/_test/ready", "", "")
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		t.Fatal(err)
	}
	ready, err := codec.Decode[Ready](codec.CompactJSON(), string(body))
	if err != nil || response.StatusCode != http.StatusOK || ready.Protocol != ProtocolVersion || ready.RunID != "owned-process-run" || ready.BuildID != "owned-process-build" || ready.SchemaVersion != 1 {
		t.Fatal("built host readiness did not match the configured identity and actual schema")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	start := time.Now()
	outcome, err := run.Process.Shutdown(ctx)
	if err != nil || !outcome.Complete || time.Since(start) > 12*time.Second {
		t.Fatal("built host did not gracefully release its owned process within the budget")
	}
	connection, err := net.DialTimeout("tcp", address.Host, 100*time.Millisecond)
	if err == nil {
		if closeErr := connection.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal("host listener remained after owned shutdown")
	}
}
