//go:build integration

package testutil

import (
	"context"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestFixtureEffectiveContainerHardening(t *testing.T) {
	t.Parallel()
	f, err := StartS3(t.Context(), t.TempDir())
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
	exit, output, err := f.container.Exec(t.Context(), []string{"cat", "/proc/net/tcp", "/proc/net/tcp6"})
	if err != nil || exit != 0 {
		t.Fatal("could not inspect effective storage listeners")
	}
	data, err := io.ReadAll(io.LimitReader(output, 64<<10))
	if err != nil {
		t.Fatal(err)
	}
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
		if slices.Contains([]uint64{6060, 7333, 8181, 9101, 23646}, port) {
			t.Fatalf("unused optional service is still listening on port %d", port)
		}
	}
	if err := f.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}
