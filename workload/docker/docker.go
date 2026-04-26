package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/workload"
)

// Register installs the Docker provider into the supplied workload
// factory registry. Call this once at application startup before
// invoking [workload.New]. Returns an error if the provider name is
// already registered.
func Register(registry *workload.FactoryRegistry) error {
	return registry.Register(workload.ProviderDocker, func(cfg workload.Config, providerCfg any, log *logger.Logger) (workload.Manager, error) {
		c := &Config{}
		if providerCfg != nil {
			pc, ok := providerCfg.(*Config)
			if !ok {
				return nil, fmt.Errorf("docker: expected *docker.Config, got %T", providerCfg)
			}
			c = pc
		}
		c.ApplyDefaults()
		if err := c.Validate(); err != nil {
			return nil, err
		}
		return NewManager(c, cfg.DefaultLabels, log)
	})
}

// Manager implements workload.Manager using the Docker Engine SDK.
type Manager struct {
	client        *client.Client
	cfg           *Config
	defaultLabels map[string]string
	log           *logger.Logger
}

// Client returns the underlying Docker SDK client for direct operations
// not covered by the Manager's high-level API.
func (m *Manager) Client() *client.Client { return m.client }

// NewManager creates a new Docker workload manager.
func NewManager(cfg *Config, defaultLabels map[string]string, log *logger.Logger) (*Manager, error) {
	opts := []client.Opt{
		client.WithHost(cfg.Host),
	}
	if cfg.APIVersion != "" {
		opts = append(opts, client.WithAPIVersion(cfg.APIVersion))
	}
	if cfg.TLS != nil && cfg.TLS.IsEnabled() {
		opts = append(opts, client.WithTLSClientConfig(cfg.TLS.CAFile, cfg.TLS.CertFile, cfg.TLS.KeyFile))
	}

	cli, err := client.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker: create client: %w", err)
	}

	return &Manager{
		client:        cli,
		cfg:           cfg,
		defaultLabels: defaultLabels,
		log:           log,
	}, nil
}

// Deploy creates and starts a Docker container.
func (m *Manager) Deploy(ctx context.Context, req workload.DeployRequest) (*workload.DeployResult, error) {
	m.log.Info("deploying workload", map[string]interface{}{
		"name":  req.Name,
		"image": req.Image,
	})

	if err := m.ensureImage(ctx, req.Image, req.Platform); err != nil {
		return nil, fmt.Errorf("docker: pull image: %w", err)
	}

	containerCfg, hostCfg, networkCfg, platform := m.buildContainerConfig(req)

	resp, err := m.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           containerCfg,
		HostConfig:       hostCfg,
		NetworkingConfig: networkCfg,
		Platform:         platform,
		Name:             req.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("docker: create container: %w", err)
	}

	if _, err := m.client.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		_, _ = m.client.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
		return nil, fmt.Errorf("docker: start container: %w", err)
	}

	m.log.Info("workload deployed", map[string]interface{}{
		"id":   resp.ID[:12],
		"name": req.Name,
	})

	return &workload.DeployResult{
		ID:     resp.ID,
		Name:   req.Name,
		Status: workload.StatusRunning,
	}, nil
}

// Stop gracefully stops a Docker container.
func (m *Manager) Stop(ctx context.Context, id string) error {
	timeout := 30
	_, err := m.client.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: &timeout})
	return err
}

// Remove removes a Docker container.
func (m *Manager) Remove(ctx context.Context, id string) error {
	_, err := m.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
	return err
}

// Restart restarts a Docker container.
func (m *Manager) Restart(ctx context.Context, id string) error {
	timeout := 30
	_, err := m.client.ContainerRestart(ctx, id, client.ContainerRestartOptions{Timeout: &timeout})
	return err
}

// Wait blocks until the container exits and returns the exit status.
func (m *Manager) Wait(ctx context.Context, id string) (*workload.WaitResult, error) {
	waitResult := m.client.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-waitResult.Error:
		if err != nil {
			return nil, fmt.Errorf("docker: wait: %w", err)
		}
	case status := <-waitResult.Result:
		result := &workload.WaitResult{StatusCode: status.StatusCode}
		if status.Error != nil {
			result.Error = status.Error.Message
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// Wait blocks until the container exits and returns the exit status.
	// Returns (nil, nil) only on the unreachable post-select fall-through; the
	// three real arms each return.
	//
	//nolint:nilnil // unreachable defensive return; staticcheck flow analysis
	// confirms select arms cover all paths.
	return nil, nil
}

// Status returns the current status of a Docker container.
func (m *Manager) Status(ctx context.Context, id string) (*workload.WorkloadStatus, error) {
	res, err := m.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return &workload.WorkloadStatus{
				ID:     id,
				Status: workload.StatusNotFound,
			}, nil
		}
		return nil, fmt.Errorf("docker: inspect container: %w", err)
	}
	info := res.Container

	ws := &workload.WorkloadStatus{
		ID:   info.ID,
		Name: strings.TrimPrefix(info.Name, "/"),
	}
	if info.Config != nil {
		ws.Image = info.Config.Image
	}

	state := info.State
	if state == nil {
		ws.Status = workload.StatusStopped
		ws.Restarts = info.RestartCount
		return ws, nil
	}
	applyState(ws, state)
	ws.Restarts = info.RestartCount

	return ws, nil
}

// applyState copies fields from a Docker container State onto the workload
// status. Extracted to keep [Manager.Status] within nestif's complexity
// budget; State pointers are derived from a single ContainerInspect response
// so the helper is intentionally narrow and not exported.
func applyState(ws *workload.WorkloadStatus, state *container.State) {
	ws.Running = state.Running
	ws.Healthy = state.Running
	if state.Health != nil {
		ws.Healthy = state.Health.Status == "healthy"
	}
	switch {
	case state.Running:
		ws.Status = workload.StatusRunning
	case state.Restarting:
		ws.Status = workload.StatusRestarting
	case state.ExitCode != 0:
		ws.Status = workload.StatusError
	default:
		ws.Status = workload.StatusStopped
	}
	ws.ExitCode = state.ExitCode
	ws.Message = string(state.Status)
	if state.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, state.StartedAt); err == nil {
			ws.StartedAt = t
		}
	}
	if state.FinishedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, state.FinishedAt); err == nil {
			ws.StoppedAt = t
		}
	}
}

// List returns containers matching the filter.
func (m *Manager) List(ctx context.Context, filter workload.ListFilter) ([]workload.WorkloadInfo, error) {
	f := make(client.Filters)
	for k, v := range filter.Labels {
		f.Add("label", fmt.Sprintf("%s=%s", k, v))
	}
	if filter.Name != "" {
		f.Add("name", filter.Name)
	}
	if filter.Status != "" {
		f.Add("status", filter.Status)
	}

	listResult, err := m.client.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers: %w", err)
	}

	containers := listResult.Items
	infos := make([]workload.WorkloadInfo, len(containers))
	for i := range containers {
		name := ""
		if len(containers[i].Names) > 0 {
			name = strings.TrimPrefix(containers[i].Names[0], "/")
		}
		infos[i] = workload.WorkloadInfo{
			ID:      containers[i].ID,
			Name:    name,
			Image:   containers[i].Image,
			Status:  string(containers[i].State),
			Labels:  containers[i].Labels,
			Created: time.Unix(containers[i].Created, 0),
		}
	}
	return infos, nil
}

// HealthCheck verifies Docker is available.
func (m *Manager) HealthCheck(ctx context.Context) error {
	_, err := m.client.Ping(ctx, client.PingOptions{})
	if err != nil {
		return fmt.Errorf("docker: health check failed: %w", err)
	}
	return nil
}

// Compile-time interface checks.
var (
	_ workload.Manager            = (*Manager)(nil)
	_ workload.ExecProvider       = (*Manager)(nil)
	_ workload.StatsProvider      = (*Manager)(nil)
	_ workload.LogStreamer        = (*Manager)(nil)
	_ workload.EventWatcher       = (*Manager)(nil)
	_ workload.SystemInfoProvider = (*Manager)(nil)
	_ workload.DiskUsageProvider  = (*Manager)(nil)
	_ workload.ImageInspector     = (*Manager)(nil)
	_ workload.ImageEventWatcher  = (*Manager)(nil)
)
