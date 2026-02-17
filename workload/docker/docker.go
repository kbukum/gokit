package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

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

	// Ensure image is available
	if err := m.ensureImage(ctx, req.Image, req.Platform); err != nil {
		return nil, fmt.Errorf("docker: pull image: %w", err)
	}

	// Build container config
	containerCfg, hostCfg, networkCfg, platform := m.buildConfigs(req)

	// Create container
	resp, err := m.client.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, platform, req.Name)
	if err != nil {
		return nil, fmt.Errorf("docker: create container: %w", err)
	}

	// Start container
	if err := m.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Clean up on failure
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

// Status returns the current status of a Docker container.
func (m *Manager) Status(ctx context.Context, id string) (*workload.WorkloadStatus, error) {
	info, err := m.client.ContainerInspect(ctx, id)
	if err != nil {
		if client.IsErrNotFound(err) {
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

// Logs retrieves log output from a Docker container.
func (m *Manager) Logs(ctx context.Context, id string, opts workload.LogOptions) ([]string, error) {
	logOpts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	}
	if opts.Tail > 0 {
		logOpts.Tail = strconv.Itoa(opts.Tail)
	}
	if opts.Since > 0 {
		logOpts.Since = time.Now().Add(-opts.Since).Format(time.RFC3339)
	}

	reader, err := m.client.ContainerLogs(ctx, id, logOpts)
	if err != nil {
		return nil, fmt.Errorf("docker: get logs: %w", err)
	}
	defer reader.Close()

	var lines []string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		// Docker multiplexes stdout/stderr with an 8-byte header; strip it
		if len(line) > 8 {
			line = line[8:]
		}
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

// StreamLogs implements LogStreamer for real-time log streaming.
func (m *Manager) StreamLogs(ctx context.Context, id string, opts workload.LogOptions) (io.ReadCloser, error) {
	logOpts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	}
	if opts.Tail > 0 {
		logOpts.Tail = strconv.Itoa(opts.Tail)
	}
	return m.client.ContainerLogs(ctx, id, logOpts)
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
	for i, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		infos[i] = workload.WorkloadInfo{
			ID:      c.ID,
			Name:    name,
			Image:   c.Image,
			Status:  c.State,
			Labels:  c.Labels,
			Created: time.Unix(c.Created, 0),
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

// Exec executes a command inside a running container.
func (m *Manager) Exec(ctx context.Context, id string, cmd []string) (*workload.ExecResult, error) {
	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := m.client.ContainerExecCreate(ctx, id, execCfg)
	if err != nil {
		return nil, fmt.Errorf("docker: exec create: %w", err)
	}

	resp, err := m.client.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: exec attach: %w", err)
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := io.Copy(&stdout, resp.Reader); err != nil {
		return nil, fmt.Errorf("docker: exec read output: %w", err)
	}

	inspect, err := m.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return nil, fmt.Errorf("docker: exec inspect: %w", err)
	}

	return &workload.ExecResult{
		ExitCode: inspect.ExitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

// Stats returns resource usage statistics for a container.
func (m *Manager) Stats(ctx context.Context, id string) (*workload.WorkloadStats, error) {
	resp, err := m.client.ContainerStats(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("docker: stats: %w", err)
	}
	defer resp.Body.Close()

	var stats container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("docker: decode stats: %w", err)
	}

	cpuPercent := 0.0
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	if sysDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / sysDelta) * float64(stats.CPUStats.OnlineCPUs) * 100.0
	}

	return &workload.WorkloadStats{
		CPUPercent:     cpuPercent,
		MemoryUsage:    int64(stats.MemoryStats.Usage),
		MemoryLimit:    int64(stats.MemoryStats.Limit),
		NetworkRxBytes: sumNetworkRx(stats.Networks),
		NetworkTxBytes: sumNetworkTx(stats.Networks),
		PIDs:           int(stats.PidsStats.Current),
	}, nil
}

// Compile-time checks.
var (
	_ workload.Manager      = (*Manager)(nil)
	_ workload.ExecProvider = (*Manager)(nil)
	_ workload.StatsProvider = (*Manager)(nil)
	_ workload.LogStreamer   = (*Manager)(nil)
)

// buildConfigs converts a DeployRequest into Docker-specific configs.
func (m *Manager) buildConfigs(req workload.DeployRequest) (*container.Config, *container.HostConfig, *network.NetworkingConfig, *ocispec.Platform) {
	// Labels: merge defaults + request labels
	labels := make(map[string]string)
	for k, v := range m.defaultLabels {
		labels[k] = v
	}
	for k, v := range req.Labels {
		labels[k] = v
	}
	labels["managed-by"] = "gokit-workload"

	// Environment
	var env []string
	for k, v := range req.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Container config
	containerCfg := &container.Config{
		Image:  req.Image,
		Env:    env,
		Labels: labels,
	}
	if len(req.Command) > 0 {
		containerCfg.Cmd = req.Command
	}
	if req.WorkDir != "" {
		containerCfg.WorkingDir = req.WorkDir
	}

	// Exposed ports + port bindings
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}
	for _, p := range req.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		containerPort := nat.Port(fmt.Sprintf("%d/%s", p.Container, proto))
		exposedPorts[containerPort] = struct{}{}
		if p.Host > 0 {
			portBindings[containerPort] = []nat.PortBinding{
				{HostPort: strconv.Itoa(p.Host)},
			}
		}
	}
	if len(exposedPorts) > 0 {
		containerCfg.ExposedPorts = exposedPorts
	}

	// Host config
	hostCfg := &container.HostConfig{
		AutoRemove:   req.AutoRemove,
		PortBindings: portBindings,
	}

	if req.RestartPolicy != "" && req.RestartPolicy != "no" {
		hostCfg.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(req.RestartPolicy)}
	}

	// Resources
	if req.Resources != nil {
		if req.Resources.MemoryLimit != "" {
			if mem, err := parseMemory(req.Resources.MemoryLimit); err == nil {
				hostCfg.Resources.Memory = mem
			}
		}
		if req.Resources.CPULimit != "" {
			if cpu, err := parseCPU(req.Resources.CPULimit); err == nil {
				hostCfg.Resources.NanoCPUs = cpu
			}
		}
	}

	// Volumes
	for _, v := range req.Volumes {
		mode := "rw"
		if v.ReadOnly {
			mode = "ro"
		}
		hostCfg.Binds = append(hostCfg.Binds, fmt.Sprintf("%s:%s:%s", v.Source, v.Target, mode))
	}

	// Extra hosts
	if req.Network != nil {
		for host, ip := range req.Network.Hosts {
			hostCfg.ExtraHosts = append(hostCfg.ExtraHosts, fmt.Sprintf("%s:%s", host, ip))
		}
		if len(req.Network.DNS) > 0 {
			hostCfg.DNS = req.Network.DNS
		}
	}

	// Network
	var networkCfg *network.NetworkingConfig
	netName := ""
	if req.Network != nil && req.Network.Mode != "" {
		netName = req.Network.Mode
	} else if m.cfg.Network != "" {
		netName = m.cfg.Network
	}
	if netName != "" && netName != "host" && netName != "bridge" {
		networkCfg = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				netName: {},
			},
		}
	}
	if netName == "host" {
		hostCfg.NetworkMode = "host"
	}

	// Platform
	var platformSpec *ocispec.Platform
	plat := req.Platform
	if plat == "" {
		plat = m.cfg.Platform
	}
	if plat != "" {
		parts := strings.SplitN(plat, "/", 2)
		if len(parts) == 2 {
			platformSpec = &ocispec.Platform{OS: parts[0], Architecture: parts[1]}
		}
	}

	return containerCfg, hostCfg, networkCfg, platformSpec
}

// ensureImage pulls the image if not present locally.
func (m *Manager) ensureImage(ctx context.Context, imageName string, platform string) error {
	_, _, err := m.client.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		return nil // already present
	}

	m.log.Info("pulling image", map[string]interface{}{"image": imageName})

	pullOpts := image.PullOptions{}
	if platform != "" {
		pullOpts.Platform = platform
	} else if m.cfg.Platform != "" {
		pullOpts.Platform = m.cfg.Platform
	}

	reader, err := m.client.ImagePull(ctx, imageName, pullOpts)
	if err != nil {
		return fmt.Errorf("pull %s: %w", imageName, err)
	}
	defer reader.Close()
	// Consume the pull output to completion
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func sumNetworkRx(networks map[string]container.NetworkStats) int64 {
	var total int64
	for _, n := range networks {
		total += int64(n.RxBytes)
	}
	return total
}

func sumNetworkTx(networks map[string]container.NetworkStats) int64 {
	var total int64
	for _, n := range networks {
		total += int64(n.TxBytes)
	}
	return total
}

// parseMemory converts memory strings like "512m", "1g" to bytes.
func parseMemory(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty memory string")
	}

	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "gi"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "gi")
	case strings.HasSuffix(s, "mi"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "mi")
	case strings.HasSuffix(s, "g"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "m"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "k")
	}

	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse memory %q: %w", s, err)
	}
	return val * multiplier, nil
}

// parseCPU converts CPU strings like "0.5", "1", "500m" to nanoseconds.
func parseCPU(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "m") {
		// millicores: "500m" = 0.5 CPU
		val, err := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		if err != nil {
			return 0, err
		}
		return int64(val * 1e6), nil // millicores to nanocpus
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int64(val * 1e9), nil // CPUs to nanocpus
}
