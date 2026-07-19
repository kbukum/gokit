package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/workload"
)

type dockerRoundTripFunc func(*http.Request) (*http.Response, error)

func (f dockerRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestManager(t *testing.T, fn func(*http.Request) (int, string)) *Manager {
	t.Helper()
	httpClient := &http.Client{Transport: dockerRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		status, body := fn(req)
		if status == 0 {
			status = http.StatusOK
		}
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	cli, err := client.New(
		client.WithHost("http://docker.example"),
		client.WithAPIVersion("1.55"),
		client.WithHTTPClient(httpClient),
	)
	if err != nil {
		t.Fatalf("new docker client: %v", err)
	}
	return &Manager{
		client:        cli,
		cfg:           &Config{Host: "http://docker.example", Platform: "linux/amd64"},
		defaultLabels: map[string]string{"team": "platform"},
		log:           logging.NewDefault("docker-test"),
	}
}

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	buf, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(buf)
}

func dockerPath(path string) string {
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)
	if len(parts) == 2 && strings.HasPrefix(parts[0], "v") {
		return "/" + parts[1]
	}
	return path
}

func TestConfigApplyDefaultsAndValidate(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.ApplyDefaults()
	if cfg.Host != "unix:///var/run/docker.sock" {
		t.Fatalf("default host = %q", cfg.Host)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config validates: %v", err)
	}
	if err := (&Config{}).Validate(); err == nil {
		t.Fatal("empty host should fail validation before defaults are applied")
	}
	if err := (&Config{Host: "tcp://docker", TLS: &security.TLSConfig{Enabled: true}}).Validate(); err == nil {
		t.Fatal("enabled empty TLS config should fail validation")
	}
}

func TestRegisterValidatesProviderConfig(t *testing.T) {
	t.Parallel()

	registry := workload.NewFactoryRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("register docker provider: %v", err)
	}
	_, err := workload.New(registry, workload.Config{Provider: workload.ProviderDocker}, "not config", logging.NewDefault("test"))
	if err == nil || !strings.Contains(err.Error(), "expected *docker.Config") {
		t.Fatalf("expected typed provider config error, got %v", err)
	}
}

func TestBuildContainerConfigTranslatesDeployRequest(t *testing.T) {
	t.Parallel()

	manager := &Manager{
		cfg:           &Config{Network: "shared", Platform: "linux/arm64"},
		defaultLabels: map[string]string{"env": "prod", "team": "platform"},
	}
	req := workload.DeployRequest{
		Name:          "worker",
		Image:         "example/worker:1",
		Command:       []string{"run"},
		WorkDir:       "/work",
		Environment:   map[string]string{"A": "1"},
		Labels:        map[string]string{"team": "jobs"},
		Ports:         []workload.PortMapping{{Container: 8080, Host: 18080}, {Container: 5353, Protocol: "udp"}, {Container: -1}},
		RestartPolicy: "on-failure",
		Resources:     &workload.ResourceConfig{MemoryLimit: "128Mi", CPULimit: "0.5"},
		Volumes:       []workload.VolumeMount{{Source: "/host", Target: "/data", ReadOnly: true}},
		Network:       &workload.NetworkConfig{Mode: "custom", Hosts: map[string]string{"db": "10.0.0.2"}, DNS: []string{"1.1.1.1", "not-ip"}},
	}

	containerCfg, hostCfg, networkCfg, platform := manager.buildContainerConfig(req)
	if containerCfg.Image != req.Image || containerCfg.WorkingDir != "/work" || len(containerCfg.Cmd) != 1 {
		t.Fatalf("container config did not preserve image, command, and working directory: %#v", containerCfg)
	}
	if containerCfg.Labels["team"] != "jobs" || containerCfg.Labels["env"] != "prod" || containerCfg.Labels["managed-by"] != "gokit-workload" {
		t.Fatalf("labels were not merged with request precedence: %#v", containerCfg.Labels)
	}
	if len(containerCfg.Env) != 1 || containerCfg.Env[0] != "A=1" {
		t.Fatalf("environment not translated: %#v", containerCfg.Env)
	}
	if _, ok := containerCfg.ExposedPorts["8080/tcp"]; !ok {
		t.Fatalf("tcp port not exposed: %#v", containerCfg.ExposedPorts)
	}
	if _, ok := containerCfg.ExposedPorts["5353/udp"]; !ok {
		t.Fatalf("udp port not exposed: %#v", containerCfg.ExposedPorts)
	}
	if hostCfg.PortBindings["8080/tcp"][0].HostPort != "18080" {
		t.Fatalf("host port binding not translated: %#v", hostCfg.PortBindings)
	}
	if hostCfg.RestartPolicy.Name != "on-failure" || hostCfg.Memory == 0 || hostCfg.NanoCPUs == 0 {
		t.Fatalf("host restart/resources not translated: %#v", hostCfg)
	}
	if len(hostCfg.Binds) != 1 || hostCfg.Binds[0] != "/host:/data:ro" {
		t.Fatalf("volume bind not translated: %#v", hostCfg.Binds)
	}
	if len(hostCfg.ExtraHosts) != 1 || len(hostCfg.DNS) != 1 {
		t.Fatalf("network host/DNS not translated: hosts=%#v dns=%#v", hostCfg.ExtraHosts, hostCfg.DNS)
	}
	if _, ok := networkCfg.EndpointsConfig["custom"]; !ok {
		t.Fatalf("custom network endpoint not configured: %#v", networkCfg)
	}
	if platform.OS != "linux" || platform.Architecture != "arm64" {
		t.Fatalf("platform not resolved: %#v", platform)
	}
}

func TestBuildContainerConfigSpecialNetworksAndInvalidResources(t *testing.T) {
	t.Parallel()

	manager := &Manager{cfg: &Config{Network: "host"}}
	_, hostCfg, networkCfg, platform := manager.buildContainerConfig(workload.DeployRequest{
		Image:         "alpine",
		RestartPolicy: "no",
		Resources:     &workload.ResourceConfig{MemoryLimit: "bad", CPULimit: "bad"},
	})
	if networkCfg != nil || hostCfg.NetworkMode != "host" {
		t.Fatalf("host networking should be host config only, network=%#v host=%#v", networkCfg, hostCfg.NetworkMode)
	}
	if hostCfg.RestartPolicy.Name != "" || hostCfg.Memory != 0 || hostCfg.NanoCPUs != 0 {
		t.Fatalf("invalid/no resources should be ignored: %#v", hostCfg)
	}
	if platform != nil {
		t.Fatalf("empty platform should resolve nil: %#v", platform)
	}
}

func TestResolveNetworkAndPlatform(t *testing.T) {
	t.Parallel()

	manager := &Manager{cfg: &Config{Network: "default-net", Platform: "linux/amd64"}}
	if got := manager.resolveNetwork(nil); got != "default-net" {
		t.Fatalf("config network = %q", got)
	}
	if got := manager.resolveNetwork(&workload.NetworkConfig{Mode: "request-net"}); got != "request-net" {
		t.Fatalf("request network = %q", got)
	}
	if got := manager.resolvePlatform(""); got.OS != "linux" || got.Architecture != "amd64" {
		t.Fatalf("config platform = %#v", got)
	}
	if got := manager.resolvePlatform("invalid"); got != nil {
		t.Fatalf("invalid platform should be nil: %#v", got)
	}
}

func TestManagerLifecycleMethodsUseDockerAPI(t *testing.T) {
	t.Parallel()

	var createdBody map[string]any
	var removed bool
	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch path := dockerPath(req.URL.Path); path {
		case "/images/example/worker:1/json":
			return http.StatusOK, `{}`
		case "/containers/create":
			if req.URL.Query().Get("name") != "worker" {
				return http.StatusBadRequest, `{"message":"missing name"}`
			}
			if err := json.NewDecoder(req.Body).Decode(&createdBody); err != nil {
				return http.StatusBadRequest, `{"message":"bad body"}`
			}
			return http.StatusCreated, `{"Id":"abcdef1234567890"}`
		case "/containers/abcdef1234567890/start", "/containers/abcdef1234567890/stop", "/containers/abcdef1234567890/restart":
			return http.StatusNoContent, ``
		case "/containers/abcdef1234567890":
			removed = req.Method == http.MethodDelete
			return http.StatusNoContent, ``
		default:
			return http.StatusNotFound, `{"message":"unexpected ` + path + `"}`
		}
	})

	result, err := manager.Deploy(context.Background(), workload.DeployRequest{Name: "worker", Image: "example/worker:1"})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if result.ID != "abcdef1234567890" || result.Status != workload.StatusRunning {
		t.Fatalf("deploy result = %#v", result)
	}
	config, ok := createdBody["Config"].(map[string]any)
	if !ok || config["Image"] != "example/worker:1" {
		t.Fatalf("create body did not include image config: %#v", createdBody)
	}
	if err := manager.Stop(context.Background(), result.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := manager.Restart(context.Background(), result.ID); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := manager.Remove(context.Background(), result.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !removed {
		t.Fatal("remove did not call DELETE container endpoint")
	}
}

func TestDeployRemovesContainerWhenStartFails(t *testing.T) {
	t.Parallel()

	removed := false
	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/images/example/worker:1/json":
			return http.StatusOK, `{}`
		case "/containers/create":
			return http.StatusCreated, `{"Id":"container-id"}`
		case "/containers/container-id/start":
			return http.StatusInternalServerError, `{"message":"boom"}`
		case "/containers/container-id":
			removed = true
			return http.StatusNoContent, ``
		default:
			return http.StatusNotFound, `{}`
		}
	})

	_, err := manager.Deploy(context.Background(), workload.DeployRequest{Name: "worker", Image: "example/worker:1"})
	if err == nil || !strings.Contains(err.Error(), "start container") {
		t.Fatalf("expected start failure, got %v", err)
	}
	if !removed {
		t.Fatal("failed start should remove created container")
	}
}

func TestWaitReturnsContainerExitStatusAndErrors(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		if dockerPath(req.URL.Path) != "/containers/id/wait" {
			return http.StatusNotFound, `{}`
		}
		return http.StatusOK, `{"StatusCode":7,"Error":{"Message":"failed"}}`
	})
	result, err := manager.Wait(context.Background(), "id")
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.StatusCode != 7 || result.Error != "failed" {
		t.Fatalf("wait result = %#v", result)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = manager.Wait(ctx, "id")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait error = %v", err)
	}
}

func TestStatusMapsDockerContainerState(t *testing.T) {
	t.Parallel()

	started := "2026-07-18T20:00:00Z"
	finished := "2026-07-18T21:00:00Z"
	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/containers/running/json":
			return http.StatusOK, jsonBody(t, map[string]any{
				"Id":           "running",
				"Name":         "/worker",
				"RestartCount": 2,
				"Config":       map[string]any{"Image": "alpine"},
				"State": map[string]any{
					"Running":    true,
					"Status":     "running",
					"StartedAt":  started,
					"FinishedAt": finished,
					"Health":     map[string]any{"Status": "healthy"},
				},
			})
		case "/containers/exited/json":
			return http.StatusOK, jsonBody(t, map[string]any{"Id": "exited", "Name": "/bad", "State": map[string]any{"ExitCode": 3, "Status": "exited"}})
		case "/containers/missing/json":
			return http.StatusNotFound, `{"message":"not found"}`
		default:
			return http.StatusInternalServerError, `{}`
		}
	})

	status, err := manager.Status(context.Background(), "running")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.ID != "running" || status.Name != "worker" || status.Image != "alpine" || status.Status != workload.StatusRunning || !status.Healthy || status.Restarts != 2 || status.StartedAt.IsZero() || status.StoppedAt.IsZero() {
		t.Fatalf("running status = %#v", status)
	}
	exited, err := manager.Status(context.Background(), "exited")
	if err != nil {
		t.Fatalf("exited status: %v", err)
	}
	if exited.Status != workload.StatusError || exited.ExitCode != 3 {
		t.Fatalf("exited status = %#v", exited)
	}
	missing, err := manager.Status(context.Background(), "missing")
	if err != nil {
		t.Fatalf("missing status: %v", err)
	}
	if missing.Status != workload.StatusNotFound {
		t.Fatalf("missing status = %#v", missing)
	}
}

func TestListAndHealthCheckUseFilters(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/containers/json":
			if req.URL.Query().Get("all") != "1" || !strings.Contains(req.URL.Query().Get("filters"), "team") {
				return http.StatusBadRequest, `{"message":"missing filters"}`
			}
			return http.StatusOK, `[{"Id":"abc","Names":["/worker"],"Image":"alpine","State":"running","Labels":{"team":"platform"},"Created":1710000000}]`
		case "/_ping":
			return http.StatusOK, "OK"
		default:
			return http.StatusNotFound, `{}`
		}
	})

	infos, err := manager.List(context.Background(), workload.ListFilter{Name: "worker", Status: "running", Labels: map[string]string{"team": "platform"}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(infos) != 1 || infos[0].Name != "worker" || infos[0].Status != "running" || infos[0].Created.IsZero() {
		t.Fatalf("infos = %#v", infos)
	}
	if err := manager.HealthCheck(context.Background()); err != nil {
		t.Fatalf("health: %v", err)
	}
}

func TestLogsStripDockerHeadersAndStream(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		if dockerPath(req.URL.Path) != "/containers/id/logs" {
			return http.StatusNotFound, `{}`
		}
		if req.URL.Query().Get("stdout") != "1" || req.URL.Query().Get("stderr") != "1" || req.URL.Query().Get("tail") != "2" {
			return http.StatusBadRequest, `bad query`
		}
		return http.StatusOK, "12345678hello\n12345678\n12345678world\n"
	})

	lines, err := manager.Logs(context.Background(), "id", workload.LogOptions{Tail: 2, Since: time.Minute})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if strings.Join(lines, ",") != "hello,world" {
		t.Fatalf("lines = %#v", lines)
	}
	stream, err := manager.StreamLogs(context.Background(), "id", workload.LogOptions{Tail: 2})
	if err != nil {
		t.Fatalf("stream logs: %v", err)
	}
	defer stream.Close()
	body, err := io.ReadAll(stream)
	if err != nil || !bytes.Contains(body, []byte("hello")) {
		t.Fatalf("stream body %q err %v", body, err)
	}
}

func TestIsLocalRecognizesDockerDaemonSchemes(t *testing.T) {
	t.Parallel()

	for _, host := range []string{"", "unix:///var/run/docker.sock", "npipe:////./pipe/docker_engine", "fd://"} {
		if !isLocal(host) {
			t.Fatalf("%q should be local", host)
		}
	}
	if isLocal("tcp://docker.example:2376") {
		t.Fatal("tcp daemon should not be treated as local")
	}
}
