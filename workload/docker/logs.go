package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"

	"github.com/kbukum/gokit/workload"
)

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
