package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/kbukum/gokit/workload"
)

// buildContainerConfig converts a DeployRequest into Docker-specific configs.
func (m *Manager) buildContainerConfig(req workload.DeployRequest) (*container.Config, *container.HostConfig, *network.NetworkingConfig, *ocispec.Platform) {
	labels := mergeLabels(m.defaultLabels, req.Labels)
	labels["managed-by"] = "gokit-workload"

	env := make([]string, 0, len(req.Environment))
	for k, v := range req.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

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

	// Ports
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
			if mem, err := workload.ParseMemory(req.Resources.MemoryLimit); err == nil {
				hostCfg.Memory = mem
			}
		}
		if req.Resources.CPULimit != "" {
			if cpu, err := workload.ParseCPU(req.Resources.CPULimit); err == nil {
				hostCfg.NanoCPUs = cpu
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

	// Extra hosts and DNS
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
	netName := m.resolveNetwork(req.Network)
	if netName != "" && netName != "host" && netName != "bridge" && netName != "none" {
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
	platformSpec := m.resolvePlatform(req.Platform)

	return containerCfg, hostCfg, networkCfg, platformSpec
}

// ensureImage pulls the image if not present locally.
func (m *Manager) ensureImage(ctx context.Context, imageName string, platform string) error {
	_, err := m.client.ImageInspect(ctx, imageName)
	if err == nil {
		return nil
	}

	m.log.Info("pulling image", map[string]interface{}{"image": imageName})

	pullOpts := image.PullOptions{}
	plat := platform
	if plat == "" {
		plat = m.cfg.Platform
	}
	if plat != "" {
		pullOpts.Platform = plat
	}

	reader, err := m.client.ImagePull(ctx, imageName, pullOpts)
	if err != nil {
		return fmt.Errorf("pull %s: %w", imageName, err)
	}
	defer reader.Close() //nolint:errcheck // Error on close is safe to ignore for read operations
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

// resolveNetwork returns the network name from the request or config default.
func (m *Manager) resolveNetwork(netCfg *workload.NetworkConfig) string {
	if netCfg != nil && netCfg.Mode != "" {
		return netCfg.Mode
	}
	return m.cfg.Network
}

// resolvePlatform parses "os/arch" into an OCI platform spec.
func (m *Manager) resolvePlatform(platform string) *ocispec.Platform {
	plat := platform
	if plat == "" {
		plat = m.cfg.Platform
	}
	if plat == "" {
		return nil
	}
	parts := strings.SplitN(plat, "/", 2)
	if len(parts) == 2 {
		return &ocispec.Platform{OS: parts[0], Architecture: parts[1]}
	}
	return nil
}

// mergeLabels combines default labels with request labels (request wins).
func mergeLabels(defaults, request map[string]string) map[string]string {
	labels := make(map[string]string, len(defaults)+len(request))
	for k, v := range defaults {
		labels[k] = v
	}
	for k, v := range request {
		labels[k] = v
	}
	return labels
}
