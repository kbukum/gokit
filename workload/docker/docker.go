package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/workload"
)

func init() {
	workload.RegisterFactory(workload.ProviderDocker, func(cfg workload.Config, providerCfg any, log *logger.Logger) (workload.Manager, error) {
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

// NewManager creates a new Docker workload manager.
func NewManager(cfg *Config, defaultLabels map[string]string, log *logger.Logger) (*Manager, error) {
	opts := []client.Opt{
		client.WithHost(cfg.Host),
	}
	if cfg.APIVersion != "" {
		opts = append(opts, client.WithVersion(cfg.APIVersion))
	} else {
		opts = append(opts, client.WithAPIVersionNegotiation())
	}
	if cfg.TLS != nil && cfg.TLS.Cert != "" {
		opts = append(opts, client.WithTLSClientConfig(cfg.TLS.CACert, cfg.TLS.Cert, cfg.TLS.Key))
	}

	cli, err := client.NewClientWithOpts(opts...)
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

	resp, err := m.client.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, platform, req.Name)
	if err != nil {
		return nil, fmt.Errorf("docker: create container: %w", err)
	}

	if err := m.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = m.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
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
	return m.client.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
}

// Remove removes a Docker container.
func (m *Manager) Remove(ctx context.Context, id string) error {
	return m.client.ContainerRemove(ctx, id, container.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
}

// Restart restarts a Docker container.
func (m *Manager) Restart(ctx context.Context, id string) error {
	timeout := 30
	return m.client.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
}

// Wait blocks until the container exits and returns the exit status.
func (m *Manager) Wait(ctx context.Context, id string) (*workload.WaitResult, error) {
	statusCh, errCh := m.client.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("docker: wait: %w", err)
		}
	case status := <-statusCh:
		result := &workload.WaitResult{StatusCode: status.StatusCode}
		if status.Error != nil {
			result.Error = status.Error.Message
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return nil, nil
}

// Status returns the current status of a Docker container.
func (m *Manager) Status(ctx context.Context, id string) (*workload.WorkloadStatus, error) {
	info, err := m.client.ContainerInspect(ctx, id)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return &workload.WorkloadStatus{
				ID:     id,
				Status: workload.StatusNotFound,
			}, nil
		}
		return nil, fmt.Errorf("docker: inspect container: %w", err)
	}

	ws := &workload.WorkloadStatus{
		ID:      info.ID,
		Name:    strings.TrimPrefix(info.Name, "/"),
		Image:   info.Config.Image,
		Running: info.State.Running,
		Healthy: info.State.Running,
	}

	if info.State.Health != nil {
		ws.Healthy = info.State.Health.Status == "healthy"
	}

	switch {
	case info.State.Running:
		ws.Status = workload.StatusRunning
	case info.State.Restarting:
		ws.Status = workload.StatusRestarting
	case info.State.ExitCode != 0:
		ws.Status = workload.StatusError
	default:
		ws.Status = workload.StatusStopped
	}

	ws.ExitCode = info.State.ExitCode
	ws.Message = info.State.Status
	ws.Restarts = info.RestartCount

	if info.State.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, info.State.StartedAt); err == nil {
			ws.StartedAt = t
		}
	}
	if info.State.FinishedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, info.State.FinishedAt); err == nil {
			ws.StoppedAt = t
		}
	}

	return ws, nil
}

// List returns containers matching the filter.
func (m *Manager) List(ctx context.Context, filter workload.ListFilter) ([]workload.WorkloadInfo, error) {
	f := filters.NewArgs()
	for k, v := range filter.Labels {
		f.Add("label", fmt.Sprintf("%s=%s", k, v))
	}
	if filter.Name != "" {
		f.Add("name", filter.Name)
	}
	if filter.Status != "" {
		f.Add("status", filter.Status)
	}

	containers, err := m.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers: %w", err)
	}

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
			Status:  containers[i].State,
			Labels:  containers[i].Labels,
			Created: time.Unix(containers[i].Created, 0),
		}
	}
	return infos, nil
}

// HealthCheck verifies Docker is available.
func (m *Manager) HealthCheck(ctx context.Context) error {
	_, err := m.client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("docker: health check failed: %w", err)
	}
	return nil
}

// Compile-time interface checks.
var (
	_ workload.Manager       = (*Manager)(nil)
	_ workload.ExecProvider  = (*Manager)(nil)
	_ workload.StatsProvider = (*Manager)(nil)
	_ workload.LogStreamer   = (*Manager)(nil)
	_ workload.EventWatcher  = (*Manager)(nil)
)
