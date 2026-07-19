package docker

import (
	"context"
	"net/http"
	"testing"

	"github.com/moby/moby/api/types/container"

	"github.com/kbukum/gokit/workload"
)

func TestSystemInfoMapsDaemonFieldsAndGPU(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		if dockerPath(req.URL.Path) != "/info" {
			return http.StatusNotFound, `{}`
		}
		return http.StatusOK, jsonBody(t, map[string]any{
			"OSType":          "linux",
			"Architecture":    "x86_64",
			"MemTotal":        1024 * 1024 * 512,
			"NCPU":            8,
			"Driver":          "overlay2",
			"ServerVersion":   "26.0",
			"KernelVersion":   "6.0",
			"OperatingSystem": "Linux",
			"Runtimes":        map[string]any{"runc": map[string]any{}, "nvidia": map[string]any{}},
		})
	})

	info, err := manager.SystemInfo(context.Background())
	if err != nil {
		t.Fatalf("system info: %v", err)
	}
	if info.OS != "linux" || info.Architecture != "x86_64" || info.TotalMemoryMB != 512 || info.CPUs != 8 || info.StorageDriver != "overlay2" || len(info.GPUs) != 1 {
		t.Fatalf("system info = %#v", info)
	}
}

func TestDiskUsageAggregatesDaemonUsage(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/system/df":
			return http.StatusOK, jsonBody(t, map[string]any{
				"ImageUsage": map[string]any{
					"Items": []map[string]any{{"Id": "img", "RepoTags": []string{"app:1"}, "Size": 100, "SharedSize": 20, "Created": 1710000000}},
				},
				"ContainerUsage": map[string]any{
					"Items": []map[string]any{{"SizeRw": 30}},
				},
				"VolumeUsage": map[string]any{
					"Items": []map[string]any{{"UsageData": map[string]any{"Size": 40}}, {"UsageData": map[string]any{"Size": 0}}},
				},
				"BuildCacheUsage": map[string]any{
					"Items": []map[string]any{{"Size": 50, "Shared": false}, {"Size": 60, "Shared": true}},
				},
			})
		case "/info":
			return http.StatusOK, jsonBody(t, map[string]any{"DockerRootDir": "/remote/docker"})
		default:
			return http.StatusNotFound, `{}`
		}
	})
	manager.cfg.Host = "tcp://remote:2376"

	usage, err := manager.DiskUsage(context.Background())
	if err != nil {
		t.Fatalf("disk usage: %v", err)
	}
	if usage.ImagesSize != 100 || len(usage.Images) != 1 || usage.Images[0].Size != 80 || usage.ContainersSize != 30 || usage.VolumesSize != 40 || usage.BuildCacheSize != 50 || usage.DataRootPath != "/remote/docker" || usage.DataRootTotal != 0 {
		t.Fatalf("disk usage = %#v", usage)
	}
}

func TestNetworkSumHelpers(t *testing.T) {
	t.Parallel()

	stats := map[string]containerNetworkStats{
		"eth0": {rx: 1, tx: 2},
		"eth1": {rx: 3, tx: 4},
	}
	converted := makeDockerNetworkStats(stats)
	if got := sumNetworkRx(converted); got != 4 {
		t.Fatalf("rx = %d", got)
	}
	if got := sumNetworkTx(converted); got != 6 {
		t.Fatalf("tx = %d", got)
	}
}

type containerNetworkStats struct {
	rx uint64
	tx uint64
}

func makeDockerNetworkStats(stats map[string]containerNetworkStats) map[string]container.NetworkStats {
	result := make(map[string]container.NetworkStats, len(stats))
	for name, stat := range stats {
		result[name] = container.NetworkStats{RxBytes: stat.rx, TxBytes: stat.tx}
	}
	return result
}

var _ = workload.DiskUsage{}
