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

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/workload"
)

type dockerRoundTripFunc func(*http.Request) (*http.Response, error)

func (f dockerRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingReadCloser struct {
	err error
}

func (r failingReadCloser) Read([]byte) (int, error) { return 0, r.err }

func (r failingReadCloser) Close() error { return nil }

type contextReadCloser struct {
	ctx context.Context
}

func (r contextReadCloser) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (r contextReadCloser) Close() error { return nil }

type cancelAfterReadCloser struct {
	body   *strings.Reader
	cancel context.CancelFunc
}

func (r *cancelAfterReadCloser) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		r.cancel()
	}
	return n, err
}

func (r *cancelAfterReadCloser) Close() error { return nil }

func newTestManagerHTTP(t *testing.T, fn dockerRoundTripFunc) *Manager {
	t.Helper()
	httpClient := &http.Client{Transport: fn}
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

func newTestManager(t *testing.T, fn func(*http.Request) (int, string)) *Manager {
	t.Helper()
	return newTestManagerHTTP(t, func(req *http.Request) (*http.Response, error) {
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
	})
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
	if err := (&Config{Host: "tcp://docker", TLS: &security.TLSConfig{CertFile: "cert.pem"}}).Validate(); err == nil {
		t.Fatal("TLS config with cert_file but no key_file should fail validation")
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
	if _, ok := containerCfg.ExposedPorts[network.MustParsePort("8080/tcp")]; !ok {
		t.Fatalf("tcp port not exposed: %#v", containerCfg.ExposedPorts)
	}
	if _, ok := containerCfg.ExposedPorts[network.MustParsePort("5353/udp")]; !ok {
		t.Fatalf("udp port not exposed: %#v", containerCfg.ExposedPorts)
	}
	if hostCfg.PortBindings[network.MustParsePort("8080/tcp")][0].HostPort != "18080" {
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
	if createdBody["Image"] != "example/worker:1" {
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

func TestDeploySurfacesImageAndCreateFailures(t *testing.T) {
	t.Parallel()

	t.Run("image inspect failure does not pull", func(t *testing.T) {
		t.Parallel()

		var pulled bool
		manager := newTestManager(t, func(req *http.Request) (int, string) {
			switch dockerPath(req.URL.Path) {
			case "/images/example/worker:1/json":
				return http.StatusInternalServerError, `{"message":"registry unavailable"}`
			case "/images/create":
				pulled = true
				return http.StatusOK, `{"status":"pulled"}` + "\n"
			default:
				return http.StatusNotFound, `{}`
			}
		})

		_, err := manager.Deploy(context.Background(), workload.DeployRequest{Name: "worker", Image: "example/worker:1"})
		if err == nil || !strings.Contains(err.Error(), "inspect image") {
			t.Fatalf("expected inspect failure, got %v", err)
		}
		if pulled {
			t.Fatal("inspect failure must not be treated as a missing image pull")
		}
	})

	t.Run("image pull failure", func(t *testing.T) {
		t.Parallel()

		manager := newTestManager(t, func(req *http.Request) (int, string) {
			switch dockerPath(req.URL.Path) {
			case "/images/example/worker:1/json":
				return http.StatusNotFound, `{"message":"missing"}`
			case "/images/create":
				return http.StatusInternalServerError, `{"message":"pull denied"}`
			default:
				return http.StatusNotFound, `{}`
			}
		})

		_, err := manager.Deploy(context.Background(), workload.DeployRequest{Name: "worker", Image: "example/worker:1"})
		if err == nil || !strings.Contains(err.Error(), "pull image") {
			t.Fatalf("expected pull failure, got %v", err)
		}
	})

	t.Run("container create failure", func(t *testing.T) {
		t.Parallel()

		var started bool
		manager := newTestManager(t, func(req *http.Request) (int, string) {
			switch dockerPath(req.URL.Path) {
			case "/images/example/worker:1/json":
				return http.StatusOK, `{}`
			case "/containers/create":
				return http.StatusInternalServerError, `{"message":"create failed"}`
			case "/containers/container-id/start":
				started = true
				return http.StatusNoContent, ``
			default:
				return http.StatusNotFound, `{}`
			}
		})

		_, err := manager.Deploy(context.Background(), workload.DeployRequest{Name: "worker", Image: "example/worker:1"})
		if err == nil || !strings.Contains(err.Error(), "create container") {
			t.Fatalf("expected create failure, got %v", err)
		}
		if started {
			t.Fatal("start must not be called after create failure")
		}
	})
}

func TestDeployKeepsStartErrorWhenRollbackRemoveFails(t *testing.T) {
	t.Parallel()

	var removed bool
	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/images/example/worker:1/json":
			return http.StatusOK, `{}`
		case "/containers/create":
			return http.StatusCreated, `{"Id":"container-id"}`
		case "/containers/container-id/start":
			return http.StatusInternalServerError, `{"message":"start failed"}`
		case "/containers/container-id":
			removed = true
			return http.StatusInternalServerError, `{"message":"remove failed"}`
		default:
			return http.StatusNotFound, `{}`
		}
	})

	_, err := manager.Deploy(context.Background(), workload.DeployRequest{Name: "worker", Image: "example/worker:1"})
	if err == nil || !strings.Contains(err.Error(), "start container") || strings.Contains(err.Error(), "remove failed") {
		t.Fatalf("expected primary start failure only, got %v", err)
	}
	if !removed {
		t.Fatal("failed start should attempt best-effort cleanup")
	}
}

func TestLifecycleMethodsSurfaceDockerErrors(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/containers/id/stop", "/containers/id/restart", "/containers/id":
			return http.StatusInternalServerError, `{"message":"daemon failed"}`
		default:
			return http.StatusNotFound, `{}`
		}
	})

	for name, fn := range map[string]func(context.Context, string) error{
		"stop":    manager.Stop,
		"remove":  manager.Remove,
		"restart": manager.Restart,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := fn(context.Background(), "id")
			if err == nil || !strings.Contains(err.Error(), "daemon failed") {
				t.Fatalf("expected daemon error, got %v", err)
			}
		})
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

func TestWaitSurfacesTransportAndDecodeErrors(t *testing.T) {
	t.Parallel()

	t.Run("daemon error response", func(t *testing.T) {
		t.Parallel()

		manager := newTestManager(t, func(req *http.Request) (int, string) {
			if dockerPath(req.URL.Path) != "/containers/id/wait" {
				return http.StatusNotFound, `{}`
			}
			return http.StatusInternalServerError, `{"message":"wait failed"}`
		})

		_, err := manager.Wait(context.Background(), "id")
		if err == nil || !strings.Contains(err.Error(), "wait failed") {
			t.Fatalf("expected wait failure, got %v", err)
		}
	})

	t.Run("malformed wait body", func(t *testing.T) {
		t.Parallel()

		manager := newTestManager(t, func(req *http.Request) (int, string) {
			if dockerPath(req.URL.Path) != "/containers/id/wait" {
				return http.StatusNotFound, `{}`
			}
			return http.StatusOK, `not-json`
		})

		_, err := manager.Wait(context.Background(), "id")
		if err == nil || !strings.Contains(err.Error(), "docker: wait") {
			t.Fatalf("expected wait decode failure, got %v", err)
		}
	})
}

func TestWaitPrefersContextCancellationOverLateResult(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	manager := newTestManagerHTTP(t, func(req *http.Request) (*http.Response, error) {
		if dockerPath(req.URL.Path) != "/containers/id/wait" {
			return &http.Response{StatusCode: http.StatusNotFound, Status: http.StatusText(http.StatusNotFound), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       &cancelAfterReadCloser{body: strings.NewReader(`{"StatusCode":0}`), cancel: cancel},
			Request:    req,
		}, nil
	})

	_, err := manager.Wait(ctx, "id")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation to win over late result, got %v", err)
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

func TestStatusHandlesNilStateAndInspectFailure(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/containers/no-state/json":
			return http.StatusOK, jsonBody(t, map[string]any{
				"Id":           "no-state",
				"Name":         "/created",
				"RestartCount": 3,
				"Config":       map[string]any{"Image": "alpine"},
			})
		case "/containers/bad/json":
			return http.StatusInternalServerError, `{"message":"inspect failed"}`
		default:
			return http.StatusNotFound, `{}`
		}
	})

	status, err := manager.Status(context.Background(), "no-state")
	if err != nil {
		t.Fatalf("nil-state status: %v", err)
	}
	if status.Status != workload.StatusStopped || status.Restarts != 3 || status.Image != "alpine" {
		t.Fatalf("nil-state status = %#v", status)
	}

	_, err = manager.Status(context.Background(), "bad")
	if err == nil || !strings.Contains(err.Error(), "inspect container") {
		t.Fatalf("expected inspect failure, got %v", err)
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

func TestListAndHealthCheckSurfaceDockerErrors(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		switch dockerPath(req.URL.Path) {
		case "/containers/json":
			return http.StatusInternalServerError, `{"message":"list failed"}`
		case "/_ping":
			return http.StatusInternalServerError, `{"message":"ping failed"}`
		default:
			return http.StatusNotFound, `{}`
		}
	})

	if _, err := manager.List(context.Background(), workload.ListFilter{}); err == nil || !strings.Contains(err.Error(), "list containers") {
		t.Fatalf("expected list failure, got %v", err)
	}
	if err := manager.HealthCheck(context.Background()); err == nil || !strings.Contains(err.Error(), "health check failed") {
		t.Fatalf("expected health failure, got %v", err)
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

func TestLogsSurfaceDockerAndReadErrors(t *testing.T) {
	t.Parallel()

	t.Run("daemon error", func(t *testing.T) {
		t.Parallel()

		manager := newTestManager(t, func(req *http.Request) (int, string) {
			if dockerPath(req.URL.Path) != "/containers/id/logs" {
				return http.StatusNotFound, `{}`
			}
			return http.StatusInternalServerError, `{"message":"logs failed"}`
		})

		if _, err := manager.Logs(context.Background(), "id", workload.LogOptions{}); err == nil || !strings.Contains(err.Error(), "get logs") {
			t.Fatalf("expected logs error, got %v", err)
		}
		if stream, err := manager.StreamLogs(context.Background(), "id", workload.LogOptions{}); err == nil {
			_ = stream.Close()
			t.Fatal("expected stream logs error")
		}
	})

	t.Run("reader error", func(t *testing.T) {
		t.Parallel()

		readErr := errors.New("log stream failed")
		manager := newTestManagerHTTP(t, func(req *http.Request) (*http.Response, error) {
			if dockerPath(req.URL.Path) != "/containers/id/logs" {
				return &http.Response{StatusCode: http.StatusNotFound, Status: http.StatusText(http.StatusNotFound), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       failingReadCloser{err: readErr},
				Request:    req,
			}, nil
		})

		_, err := manager.Logs(context.Background(), "id", workload.LogOptions{})
		if !errors.Is(err, readErr) {
			t.Fatalf("expected reader error, got %v", err)
		}
	})
}

func TestLogsHonorCanceledContext(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(*http.Request) (int, string) {
		return http.StatusOK, "12345678late\n"
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := manager.Logs(ctx, "id", workload.LogOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled logs error, got %v", err)
	}
	if stream, err := manager.StreamLogs(ctx, "id", workload.LogOptions{}); !errors.Is(err, context.Canceled) {
		if stream != nil {
			_ = stream.Close()
		}
		t.Fatalf("expected canceled stream logs error, got %v", err)
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
