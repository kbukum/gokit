package docker

import (
	"context"
	"fmt"
	"time"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/kbukum/gokit/workload"
)

// ImageInspect returns detailed metadata for a specific image.
func (m *Manager) ImageInspect(ctx context.Context, ref string) (*workload.ImageDetail, error) {
	raw, _, err := m.client.ImageInspectWithRaw(ctx, ref)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil, fmt.Errorf("docker: image %s not found: %w", ref, err)
		}
		return nil, fmt.Errorf("docker: inspect image %s: %w", ref, err)
	}

	detail := &workload.ImageDetail{
		ID:           raw.ID,
		RepoTags:     raw.RepoTags,
		RepoDigests:  raw.RepoDigests,
		Size:         raw.Size,
		Architecture: raw.Architecture,
		OS:           raw.Os,
	}

	// Created is a string in RFC3339 format.
	if raw.Created != "" {
		if t, parseErr := time.Parse(time.RFC3339Nano, raw.Created); parseErr == nil {
			detail.Created = t
		}
	}

	if raw.RootFS.Type != "" {
		detail.Layers = raw.RootFS.Layers
	}

	if raw.Config != nil {
		detail.Config = workload.ImageConfig{
			Env:        raw.Config.Env,
			Labels:     raw.Config.Labels,
			Cmd:        raw.Config.Cmd,
			Entrypoint: raw.Config.Entrypoint,
		}
	}

	return detail, nil
}
