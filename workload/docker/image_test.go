package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"

	"github.com/kbukum/gokit/workload"
)

func TestImagePullEncodesOptionsAndReportsProgress(t *testing.T) {
	t.Parallel()

	var query string
	var registryAuth string
	manager := newTestManager(t, func(req *http.Request) (int, string) {
		if dockerPath(req.URL.Path) != "/images/create" {
			return http.StatusNotFound, `{}`
		}
		query = req.URL.RawQuery
		registryAuth = req.Header.Get("X-Registry-Auth")
		return http.StatusOK, `{"status":"Pulling","id":"layer1","progressDetail":{"current":1,"total":2},"progress":"1/2"}` + "\n" + `{"status":"Done","id":"layer1"}` + "\n"
	})

	var progress []ImagePullProgress
	err := manager.ImagePull(context.Background(), "registry.example/app:1", WithPullPlatform("linux/arm64"), WithPullAuth(&registry.AuthConfig{Username: "u", Password: "p"}), WithPullProgress(func(p ImagePullProgress) {
		progress = append(progress, p)
	}))
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if !strings.Contains(query, "fromImage=registry.example%2Fapp") || !strings.Contains(query, "tag=1") || !strings.Contains(query, "platform=linux%2Farm64") {
		t.Fatalf("pull query missing image/platform: %s", query)
	}
	decoded, err := base64.URLEncoding.DecodeString(registryAuth)
	if err != nil || !strings.Contains(string(decoded), `"username":"u"`) {
		t.Fatalf("registry auth was not base64 JSON: %q err %v", registryAuth, err)
	}
	if len(progress) != 2 || progress[0].Status != "Pulling" || progress[1].Status != "Done" {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestImagePullDrainsStreamWithoutProgress(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		if dockerPath(req.URL.Path) != "/images/create" {
			return http.StatusNotFound, `{}`
		}
		return http.StatusOK, `{"status":"ok"}` + "\n"
	})
	if err := manager.ImagePull(context.Background(), "alpine:latest"); err != nil {
		t.Fatalf("pull: %v", err)
	}
}

func TestImageListRemoveExistsAndInspect(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/images/json":
			return http.StatusOK, `[{"Id":"sha256:1","RepoTags":["app:1"],"Size":123,"Created":1710000000}]`
		case "/images/app:1":
			if req.Method != http.MethodDelete || req.URL.Query().Get("force") != "1" {
				return http.StatusBadRequest, `{"message":"bad remove"}`
			}
			return http.StatusOK, `[{"Untagged":"app:1"}]`
		case "/images/app:1/json":
			return http.StatusOK, jsonBody(t, map[string]any{
				"Id":           "sha256:1",
				"RepoTags":     []string{"app:1"},
				"RepoDigests":  []string{"app@sha256:abc"},
				"Size":         123,
				"Architecture": "amd64",
				"Os":           "linux",
				"Created":      created.Format(time.RFC3339Nano),
				"RootFS":       map[string]any{"Type": "layers", "Layers": []string{"sha256:a"}},
				"Config":       map[string]any{"Env": []string{"A=1"}, "Labels": map[string]string{"k": "v"}, "Cmd": []string{"run"}, "Entrypoint": []string{"entry"}},
			})
		case "/images/missing/json":
			return http.StatusNotFound, `{"message":"not found"}`
		default:
			return http.StatusNotFound, `{}`
		}
	})

	images, err := manager.ImageList(context.Background())
	if err != nil || len(images) != 1 || images[0].ID != "sha256:1" || images[0].Created.IsZero() {
		t.Fatalf("images=%#v err=%v", images, err)
	}
	if err := manager.ImageRemove(context.Background(), "app:1", true); err != nil {
		t.Fatalf("remove: %v", err)
	}
	exists, err := manager.ImageExists(context.Background(), "app:1")
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
	exists, err = manager.ImageExists(context.Background(), "missing")
	if err != nil || exists {
		t.Fatalf("missing exists=%v err=%v", exists, err)
	}
	detail, err := manager.ImageInspect(context.Background(), "app:1")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if detail.ID != "sha256:1" || detail.OS != "linux" || detail.Architecture != "amd64" || len(detail.Layers) != 1 || detail.Config.Labels["k"] != "v" || !detail.Created.Equal(created) {
		t.Fatalf("detail = %#v", detail)
	}
	if _, err := manager.ImageInspect(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing inspect error = %v", err)
	}
}

func TestEnsureImagePullsWhenImageIsMissing(t *testing.T) {
	t.Parallel()

	var pulled bool
	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/images/app:1/json":
			return http.StatusNotFound, `{"message":"not found"}`
		case "/images/create":
			pulled = true
			if req.URL.Query().Get("platform") != "linux/arm64" {
				return http.StatusBadRequest, `{"message":"missing platform"}`
			}
			return http.StatusOK, `{"status":"ok"}` + "\n"
		default:
			return http.StatusNotFound, `{}`
		}
	})

	if err := manager.ensureImage(context.Background(), "app:1", "linux/arm64"); err != nil {
		t.Fatalf("ensure image: %v", err)
	}
	if !pulled {
		t.Fatal("missing image should be pulled")
	}
}

func TestStatsComputesResourceUsage(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		if dockerPath(req.URL.Path) != "/containers/id/stats" || req.URL.Query().Get("stream") != "false" {
			return http.StatusNotFound, `{}`
		}
		return http.StatusOK, jsonBody(t, map[string]any{
			"cpu_stats":    map[string]any{"cpu_usage": map[string]any{"total_usage": 300}, "system_cpu_usage": 1000, "online_cpus": 2},
			"precpu_stats": map[string]any{"cpu_usage": map[string]any{"total_usage": 100}, "system_cpu_usage": 500},
			"memory_stats": map[string]any{"usage": 256, "limit": 1024},
			"networks":     map[string]any{"eth0": map[string]any{"rx_bytes": 10, "tx_bytes": 20}, "eth1": map[string]any{"rx_bytes": 1, "tx_bytes": 2}},
			"pids_stats":   map[string]any{"current": 4},
		})
	})

	stats, err := manager.Stats(context.Background(), "id")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.CPUPercent != 80 || stats.MemoryUsage != 256 || stats.MemoryLimit != 1024 || stats.NetworkRxBytes != 11 || stats.NetworkTxBytes != 22 || stats.PIDs != 4 {
		t.Fatalf("stats = %#v", stats)
	}
}

func TestEncodeAuthRoundTripsRegistryConfig(t *testing.T) {
	t.Parallel()

	encoded, err := encodeAuth(&registry.AuthConfig{Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("encode auth: %v", err)
	}
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode auth: %v", err)
	}
	var auth registry.AuthConfig
	if err := json.Unmarshal(decoded, &auth); err != nil {
		t.Fatalf("unmarshal auth: %v", err)
	}
	if auth.Username != "user" || auth.Password != "pass" {
		t.Fatalf("auth = %#v", auth)
	}
}

func FuzzResolvePlatform(f *testing.F) {
	f.Add("linux/amd64")
	f.Add("linux")
	f.Add("")
	f.Fuzz(func(t *testing.T, platform string) {
		manager := &Manager{cfg: &Config{}}
		got := manager.resolvePlatform(platform)
		if got != nil && (got.OS == "" || got.Architecture == "") {
			t.Fatalf("partial platform for %q: %#v", platform, got)
		}
	})
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	return string(b)
}

var (
	_ = readAll
	_ = client.ImagePullOptions{}
	_ = workload.ImageEventFilter{}
)
