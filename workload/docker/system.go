package docker

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"

	"github.com/kbukum/gokit/workload"
)

// SystemInfo returns host-level system information from the Docker daemon.
func (m *Manager) SystemInfo(ctx context.Context) (*workload.SystemInfo, error) {
	info, err := m.client.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker: system info: %w", err)
	}

	si := &workload.SystemInfo{
		OS:              info.OSType,
		Architecture:    info.Architecture,
		TotalMemoryMB:   info.MemTotal / (1024 * 1024),
		CPUs:            info.NCPU,
		StorageDriver:   info.Driver,
		RuntimeVersion:  info.ServerVersion,
		KernelVersion:   info.KernelVersion,
		OperatingSystem: info.OperatingSystem,
	}

	// Detect GPUs via Docker runtime info.
	// nvidia-container-runtime registers itself; presence indicates GPU support.
	for name := range info.Runtimes {
		if name == "nvidia" {
			si.GPUs = append(si.GPUs, workload.GPUInfo{
				Name: "NVIDIA GPU (detected via runtime)",
			})
		}
	}

	return si, nil
}

// DiskUsage returns Docker disk usage broken down by category.
func (m *Manager) DiskUsage(ctx context.Context) (*workload.DiskUsage, error) {
	du, err := m.client.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: disk usage: %w", err)
	}

	result := &workload.DiskUsage{}

	for _, img := range du.Images {
		entry := workload.ImageDiskEntry{
			ID:         img.ID,
			RepoTags:   img.RepoTags,
			Size:       img.Size - img.SharedSize,
			SharedSize: img.SharedSize,
		}
		if img.Created > 0 {
			entry.Created = time.Unix(img.Created, 0)
		}
		result.Images = append(result.Images, entry)
		result.ImagesSize += img.Size
	}

	for _, c := range du.Containers {
		result.ContainersSize += c.SizeRw
	}

	for _, v := range du.Volumes {
		if v.UsageData.Size > 0 {
			result.VolumesSize += v.UsageData.Size
		}
	}

	for _, bc := range du.BuildCache {
		if !bc.Shared {
			result.BuildCacheSize += bc.Size
		}
	}

	// Best-effort filesystem capacity for local daemons.
	info, infoErr := m.client.Info(ctx)
	if infoErr == nil && info.DockerRootDir != "" {
		result.DataRootPath = info.DockerRootDir
		if isLocal(m.cfg.Host) {
			if total, free, ok := statfs(info.DockerRootDir); ok {
				result.DataRootTotal = total
				result.DataRootFree = free
			}
		}
	}

	return result, nil
}

// isLocal returns true when the Docker host points to a local daemon.
func isLocal(host string) bool {
	return host == "" || host == "unix:///var/run/docker.sock" ||
		host == "npipe:////./pipe/docker_engine" ||
		host == "fd://"
}

// statfs returns total and free bytes for the filesystem containing path.
// Returns false on platforms or errors where this is unavailable.
func statfs(path string) (total, free int64, ok bool) {
	if runtime.GOOS == "windows" {
		return 0, 0, false
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, false
	}
	// Bsize is platform-dependent: int64 on linux, int32 on darwin.
	// The conversions are required for the darwin build but unconvert
	// flags them as unnecessary on linux. Keep them, suppress the lint.
	total = int64(stat.Blocks) * int64(stat.Bsize) //nolint:unconvert // see comment above
	free = int64(stat.Bavail) * int64(stat.Bsize)  //nolint:unconvert // see comment above
	return total, free, true
}
