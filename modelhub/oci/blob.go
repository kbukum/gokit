package oci

import (
	"context"
	"io"

	"github.com/kbukum/gokit/modelhub/gguf"
	"github.com/kbukum/gokit/modelhub/internal/fetch"
)

// Blob returns a [gguf.Source] that reads a byte prefix of layer with a range request, capped at the layer size.
// The blob content is not digest-verified, since only a prefix is read; treat what it yields as untrusted.
func (c *Client) Blob(repo string, layer Layer) gguf.Source {
	return blobSource{client: c, repo: repo, layer: layer}
}

type blobSource struct {
	client *Client
	repo   string
	layer  Layer
}

// OpenPrefix opens up to n bytes of the blob.
func (b blobSource) OpenPrefix(ctx context.Context, n int64) (io.ReadCloser, error) {
	if err := validRepository(b.repo); err != nil {
		return nil, err
	}
	if err := validDigest(b.layer.Digest); err != nil {
		return nil, err
	}
	if b.layer.Size > 0 {
		n = min(n, b.layer.Size)
	}
	rc, _, err := authorized(ctx, b.client, b.repo, func(headers map[string]string) (io.ReadCloser, *fetch.Response, error) {
		return b.client.fetch.Open(ctx, fetch.Request{URL: b.client.blobURL(b.repo, b.layer.Digest), Headers: headers}, n)
	})
	return rc, err
}
