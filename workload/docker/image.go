package docker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
)

// ImageInfo describes a local Docker image.
type ImageInfo struct {
	ID       string
	RepoTags []string
	Size     int64
	Created  time.Time
}

// ImagePullProgress reports pull progress for a single layer.
type ImagePullProgress struct {
	Status   string `json:"status"`
	ID       string `json:"id,omitempty"`
	Progress string `json:"progress,omitempty"`
	Current  int64  `json:"current,omitempty"`
	Total    int64  `json:"total,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ImagePullOption configures ImagePull behavior.
type ImagePullOption func(*imagePullOptions)

type imagePullOptions struct {
	platform   string
	authConfig *registry.AuthConfig
	onProgress func(ImagePullProgress)
}

// WithPullPlatform sets the target platform (e.g. "linux/amd64").
func WithPullPlatform(platform string) ImagePullOption {
	return func(o *imagePullOptions) { o.platform = platform }
}

// WithPullAuth sets registry credentials.
func WithPullAuth(auth *registry.AuthConfig) ImagePullOption {
	return func(o *imagePullOptions) { o.authConfig = auth }
}

// WithPullProgress sets a callback for pull progress updates.
func WithPullProgress(fn func(ImagePullProgress)) ImagePullOption {
	return func(o *imagePullOptions) { o.onProgress = fn }
}

// ImagePull pulls an image from a registry, optionally reporting progress.
func (m *Manager) ImagePull(ctx context.Context, ref string, opts ...ImagePullOption) error {
	o := &imagePullOptions{}
	for _, opt := range opts {
		opt(o)
	}

	platform := o.platform
	if platform == "" {
		platform = m.cfg.Platform
	}

	pullOpts := image.PullOptions{}
	if platform != "" {
		pullOpts.Platform = platform
	}
	if o.authConfig != nil {
		encoded, err := encodeAuth(o.authConfig)
		if err != nil {
			return fmt.Errorf("docker: encode auth: %w", err)
		}
		pullOpts.RegistryAuth = encoded
	}

	m.log.Info("pulling image", map[string]interface{}{"image": ref})

	reader, err := m.client.ImagePull(ctx, ref, pullOpts)
	if err != nil {
		return fmt.Errorf("docker: pull image %s: %w", ref, err)
	}
	defer reader.Close() //nolint:errcheck

	if o.onProgress != nil {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			var progress ImagePullProgress
			if err := json.Unmarshal(scanner.Bytes(), &progress); err == nil {
				o.onProgress(progress)
			}
		}
		return scanner.Err()
	}

	_, _ = io.Copy(io.Discard, reader)
	return nil
}

// ImageList returns all local images.
func (m *Manager) ImageList(ctx context.Context) ([]ImageInfo, error) {
	imgs, err := m.client.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: list images: %w", err)
	}
	result := make([]ImageInfo, 0, len(imgs))
	for _, img := range imgs {
		result = append(result, ImageInfo{
			ID:       img.ID,
			RepoTags: img.RepoTags,
			Size:     img.Size,
			Created:  time.Unix(img.Created, 0),
		})
	}
	return result, nil
}

// ImageRemove removes a local image.
func (m *Manager) ImageRemove(ctx context.Context, ref string, force bool) error {
	_, err := m.client.ImageRemove(ctx, ref, image.RemoveOptions{Force: force, PruneChildren: true})
	if err != nil {
		return fmt.Errorf("docker: remove image %s: %w", ref, err)
	}
	return nil
}

// ImageExists checks if an image exists locally.
func (m *Manager) ImageExists(ctx context.Context, ref string) (bool, error) {
	_, err := m.client.ImageInspect(ctx, ref)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("docker: inspect image %s: %w", ref, err)
	}
	return true, nil
}

// encodeAuth base64-encodes a registry AuthConfig for the Docker API.
func encodeAuth(auth *registry.AuthConfig) (string, error) {
	buf, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}
