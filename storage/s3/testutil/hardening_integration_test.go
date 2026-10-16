//go:build integration

package testutil

import (
	"context"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// probeImage reads the fixture's socket tables from its network namespace, so the check needs no shell in the fixture image.
const probeImage = "docker.io/library/busybox:1.37.0-musl@sha256:5cec3fc171c87218698e85a52af7087de727372aae264a787b8112901a5b0092"

func TestFixtureEffectiveContainerHardening(t *testing.T) {
	t.Parallel()
	f, err := StartS3(t.Context(), t.TempDir(), IntegrationOptions(t)...)
	if f != nil {
		t.Cleanup(func() {
			if closeErr := f.Close(context.WithoutCancel(t.Context())); closeErr != nil {
				t.Error(closeErr)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	if f.ContainerID() == "" {
		t.Fatal("running fixture omitted its network attachment identity")
	}
	actual, err := f.container.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if actual.Config.User == "" || strings.HasPrefix(actual.Config.User, "0:") || actual.Config.User == "root" {
		t.Fatal("storage container runs as root")
	}
	host := actual.HostConfig
	if !host.ReadonlyRootfs || !slices.Contains(host.CapDrop, "ALL") || !slices.Contains(host.SecurityOpt, "no-new-privileges") {
		t.Fatal("storage container lacks required privilege constraints")
	}
	if host.Memory != 512<<20 || host.NanoCPUs != 1_000_000_000 || !strings.Contains(host.Tmpfs["/tmp"], "size=33554432") {
		t.Fatal("storage container resource limits changed")
	}
	for port, bindings := range actual.NetworkSettings.Ports {
		if len(bindings) == 0 {
			continue
		}
		if port.String() != "8333/tcp" {
			t.Fatal("non-S3 listener published")
		}
		for _, binding := range bindings {
			if !binding.HostIP.IsLoopback() {
				t.Fatal("S3 listener exposed outside host loopback")
			}
		}
	}
	data, err := listenerTables(t, f.ContainerID())
	if err != nil {
		t.Fatal(err)
	}
	s3Listening := false
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[3] != "0A" {
			continue
		}
		_, portText, found := strings.Cut(fields[1], ":")
		if !found {
			t.Fatal("malformed listener evidence")
		}
		port, parseErr := strconv.ParseUint(portText, 16, 16)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		s3Listening = s3Listening || port == 8333
		if slices.Contains([]uint64{6060, 7333, 8181, 9101, 23646}, port) {
			t.Fatalf("unused optional service is still listening on port %d", port)
		}
	}
	if !s3Listening {
		t.Fatal("listener evidence omitted the S3 listener")
	}
	if err := f.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f.ContainerID() != "" {
		t.Fatal("released fixture retained a network attachment identity")
	}
	if err := f.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func listenerTables(t *testing.T, id string) (_ []byte, err error) {
	t.Helper()
	if id == "" {
		return nil, errors.New("listener probe requires a running fixture")
	}
	probe, err := testcontainers.GenericContainer(t.Context(), testcontainers.GenericContainerRequest{Started: true, ContainerRequest: testcontainers.ContainerRequest{
		Image: probeImage, Entrypoint: []string{"cat"}, Cmd: []string{"/proc/net/tcp", "/proc/net/tcp6"},
		WaitingFor: wait.ForExit().WithExitTimeout(30 * time.Second),
		ConfigModifier: func(c *container.Config) {
			c.User = "65534:65534"
		},
		HostConfigModifier: func(c *container.HostConfig) {
			c.NetworkMode = container.NetworkMode("container:" + id)
			c.ReadonlyRootfs, c.CapDrop, c.SecurityOpt = true, []string{"ALL"}, []string{"no-new-privileges"}
			c.Memory, c.NanoCPUs = 32<<20, 250_000_000
		},
	}})
	if probe != nil {
		defer func() { err = errors.Join(err, probe.Terminate(context.WithoutCancel(t.Context()))) }()
	}
	if err != nil {
		return nil, err
	}
	state, err := probe.State(t.Context())
	if err != nil {
		return nil, err
	}
	if state.Running || state.ExitCode != 0 {
		return nil, errors.New("listener probe failed")
	}
	logs, err := probe.Logs(t.Context())
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, logs.Close()) }()
	return io.ReadAll(io.LimitReader(logs, 64<<10))
}
