package storage

import (
	"context"
	"io"
)

var (
	_ Storage        = (*Component)(nil)
	_ CreateProvider = (*Component)(nil)
	_ HeadProvider   = (*Component)(nil)
)

// Upload delegates to the started backend; lifecycle failures are explicit.
func (c *Component) Upload(ctx context.Context, path string, reader io.Reader) error {
	backend, err := c.Source()
	if err != nil {
		return err
	}
	return backend.Upload(ctx, path, reader)
}

func (c *Component) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	backend, err := c.Source()
	if err != nil {
		return nil, err
	}
	return backend.Download(ctx, path)
}

func (c *Component) Delete(ctx context.Context, path string) error {
	backend, err := c.Source()
	if err != nil {
		return err
	}
	return backend.Delete(ctx, path)
}

func (c *Component) Exists(ctx context.Context, path string) (bool, error) {
	backend, err := c.Source()
	if err != nil {
		return false, err
	}
	return backend.Exists(ctx, path)
}

func (c *Component) URL(ctx context.Context, path string) (string, error) {
	backend, err := c.Source()
	if err != nil {
		return "", err
	}
	return backend.URL(ctx, path)
}

func (c *Component) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	backend, err := c.Source()
	if err != nil {
		return nil, err
	}
	return backend.List(ctx, prefix)
}

// Create delegates to the started backend's atomic create; see [Create].
func (c *Component) Create(ctx context.Context, path string, reader io.Reader) error {
	backend, err := c.Source()
	if err != nil {
		return err
	}
	return Create(ctx, backend, path, reader)
}

// Head delegates to the started backend; see [Head].
func (c *Component) Head(ctx context.Context, path string) (FileInfo, error) {
	backend, err := c.Source()
	if err != nil {
		return FileInfo{}, err
	}
	return Head(ctx, backend, path)
}
