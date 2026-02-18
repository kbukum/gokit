package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"

	"github.com/kbukum/gokit/workload"
)

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
	defer resp.Body.Close() //nolint:errcheck // Error on close is safe to ignore for read operations

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

// sumNetworkRx totals received bytes across all network interfaces.
func sumNetworkRx(networks map[string]container.NetworkStats) int64 {
	var total int64
	for _, n := range networks {
		total += int64(n.RxBytes)
	}
	return total
}

// sumNetworkTx totals transmitted bytes across all network interfaces.
func sumNetworkTx(networks map[string]container.NetworkStats) int64 {
	var total int64
	for _, n := range networks {
		total += int64(n.TxBytes)
	}
	return total
}
