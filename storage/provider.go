package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/kbukum/gokit/provider"
)

// --- Request / Response types for storage operations ---

// UploadRequest describes a storage upload operation.
type UploadRequest struct {
	Path   string
	Reader io.Reader
}

// DownloadRequest describes a storage download operation.
type DownloadRequest struct {
	Path string
}

// DownloadResponse holds the result of a download operation.
type DownloadResponse struct {
	Body io.ReadCloser
}

// DeleteRequest describes a storage delete operation.
type DeleteRequest struct {
	Path string
}

// ExistsRequest describes a storage existence check.
type ExistsRequest struct {
	Path string
}

// ExistsResponse holds the result of an existence check.
type ExistsResponse struct {
	Exists bool
}

// --- Provider adapters ---

// UploadProvider wraps Storage.Upload as a RequestResponse provider.
type UploadProvider struct {
	name    string
	storage Storage
}

// NewUploadProvider creates a RequestResponse provider for uploads.
func NewUploadProvider(name string, s Storage) *UploadProvider {
	return &UploadProvider{name: name, storage: s}
}

func (p *UploadProvider) Name() string                         { return p.name }
func (p *UploadProvider) IsAvailable(_ context.Context) bool   { return p.storage != nil }

func (p *UploadProvider) Execute(ctx context.Context, req UploadRequest) (struct{}, error) {
	if err := p.storage.Upload(ctx, req.Path, req.Reader); err != nil {
		return struct{}{}, fmt.Errorf("storage upload provider: %w", err)
	}
	return struct{}{}, nil
}

// DownloadProvider wraps Storage.Download as a RequestResponse provider.
type DownloadProvider struct {
	name    string
	storage Storage
}

// NewDownloadProvider creates a RequestResponse provider for downloads.
func NewDownloadProvider(name string, s Storage) *DownloadProvider {
	return &DownloadProvider{name: name, storage: s}
}

func (p *DownloadProvider) Name() string                         { return p.name }
func (p *DownloadProvider) IsAvailable(_ context.Context) bool   { return p.storage != nil }

func (p *DownloadProvider) Execute(ctx context.Context, req DownloadRequest) (*DownloadResponse, error) {
	body, err := p.storage.Download(ctx, req.Path)
	if err != nil {
		return nil, fmt.Errorf("storage download provider: %w", err)
	}
	return &DownloadResponse{Body: body}, nil
}

// DeleteProvider wraps Storage.Delete as a RequestResponse provider.
type DeleteProvider struct {
	name    string
	storage Storage
}

// NewDeleteProvider creates a RequestResponse provider for deletes.
func NewDeleteProvider(name string, s Storage) *DeleteProvider {
	return &DeleteProvider{name: name, storage: s}
}

func (p *DeleteProvider) Name() string                         { return p.name }
func (p *DeleteProvider) IsAvailable(_ context.Context) bool   { return p.storage != nil }

func (p *DeleteProvider) Execute(ctx context.Context, req DeleteRequest) (struct{}, error) {
	if err := p.storage.Delete(ctx, req.Path); err != nil {
		return struct{}{}, fmt.Errorf("storage delete provider: %w", err)
	}
	return struct{}{}, nil
}

// ExistsProvider wraps Storage.Exists as a RequestResponse provider.
type ExistsProvider struct {
	name    string
	storage Storage
}

// NewExistsProvider creates a RequestResponse provider for existence checks.
func NewExistsProvider(name string, s Storage) *ExistsProvider {
	return &ExistsProvider{name: name, storage: s}
}

func (p *ExistsProvider) Name() string                         { return p.name }
func (p *ExistsProvider) IsAvailable(_ context.Context) bool   { return p.storage != nil }

func (p *ExistsProvider) Execute(ctx context.Context, req ExistsRequest) (*ExistsResponse, error) {
	exists, err := p.storage.Exists(ctx, req.Path)
	if err != nil {
		return nil, fmt.Errorf("storage exists provider: %w", err)
	}
	return &ExistsResponse{Exists: exists}, nil
}

// compile-time checks
var _ provider.RequestResponse[UploadRequest, struct{}] = (*UploadProvider)(nil)
var _ provider.RequestResponse[DownloadRequest, *DownloadResponse] = (*DownloadProvider)(nil)
var _ provider.RequestResponse[DeleteRequest, struct{}] = (*DeleteProvider)(nil)
var _ provider.RequestResponse[ExistsRequest, *ExistsResponse] = (*ExistsProvider)(nil)
