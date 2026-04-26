package docker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

	pullOpts := client.ImagePullOptions{}
	if platform != "" {
		parts := strings.SplitN(platform, "/", 2)
		if len(parts) == 2 {
			pullOpts.Platforms = []ocispec.Platform{{OS: parts[0], Architecture: parts[1]}}
		}
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
	defer reader.Close() //nolint:errcheck // best-effort close of pull progress stream

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
	listResult, err := m.client.ImageList(ctx, client.ImageListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: list images: %w", err)
	}
	imgs := listResult.Items
	result := make([]ImageInfo, 0, len(imgs))
	for i := range imgs {
		img := &imgs[i]
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
	_, err := m.client.ImageRemove(ctx, ref, client.ImageRemoveOptions{Force: force, PruneChildren: true})
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
// G117 (gosec) flags the marshaled "Password" field as a secret-shaped JSON
// key. That is intentional here — the Docker daemon expects an
// X-Registry-Auth header containing exactly this payload (registry password
// included). The encoded blob is sent only over the local Docker socket /
// authenticated daemon connection, never logged.
func encodeAuth(auth *registry.AuthConfig) (string, error) {
	buf, err := json.Marshal(auth) //nolint:gosec // G117: documented above; required by Docker registry-auth contract
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}
