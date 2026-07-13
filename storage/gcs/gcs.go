package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/auth/credentials"
	gcstorage "cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/storage"
)

// Register registers a configured GCS storage provider into the given registry.
func Register(reg *storage.FactoryRegistry, cfg Config) error {
	if reg == nil {
		return fmt.Errorf("gcs: storage registry is nil")
	}
	c := cfg
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return err
	}
	return reg.Register(ProviderName, func(_ storage.Config, _ *logging.Logger) (storage.Storage, error) {
		return NewStorage(context.Background(), &c)
	})
}

// ProviderName is the canonical provider name for Google Cloud Storage.
const ProviderName = storage.ProviderGCS

type objectClient interface {
	Put(ctx context.Context, path string, reader io.Reader) error
	Get(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) (bool, error)
	List(ctx context.Context, prefix string) ([]storage.FileInfo, error)
	SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error)
}

// Storage implements storage.Storage using Google Cloud Storage.
type Storage struct {
	bucket    string
	publicURL string
	client    objectClient
}

// NewStorage creates a Google Cloud Storage client from the given config.
func NewStorage(ctx context.Context, cfg *Config) (*Storage, error) {
	client, err := newGoogleClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return NewStorageWithClient(cfg.Bucket, cfg.PublicURL, client), nil
}

// NewStorageWithClient creates storage with an injected client for tests.
func NewStorageWithClient(bucket, publicURL string, client objectClient) *Storage {
	return &Storage{bucket: bucket, publicURL: strings.TrimRight(publicURL, "/"), client: client}
}

// Upload writes data from reader to GCS.
func (s *Storage) Upload(ctx context.Context, path string, reader io.Reader) error {
	if err := s.client.Put(ctx, path, reader); err != nil {
		return fmt.Errorf("storage: gcs upload: %w", err)
	}
	return nil
}

// Download returns a reader for the GCS object at path.
func (s *Storage) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	body, err := s.client.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("storage: gcs download: %w", err)
	}
	return body, nil
}

// Delete removes a GCS object. Missing objects are ignored by the SDK wrapper.
func (s *Storage) Delete(ctx context.Context, path string) error {
	if err := s.client.Delete(ctx, path); err != nil {
		return fmt.Errorf("storage: gcs delete: %w", err)
	}
	return nil
}

// Exists checks whether a GCS object exists.
func (s *Storage) Exists(ctx context.Context, path string) (bool, error) {
	exists, err := s.client.Exists(ctx, path)
	if err != nil {
		return false, fmt.Errorf("storage: gcs exists: %w", err)
	}
	return exists, nil
}

// URL returns a public URL for the object.
func (s *Storage) URL(_ context.Context, path string) (string, error) {
	if s.publicURL != "" {
		return s.publicURL + "/" + url.PathEscape(path), nil
	}
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.bucket, url.PathEscape(path)), nil
}

// List returns metadata for objects whose name starts with prefix.
func (s *Storage) List(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	files, err := s.client.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("storage: gcs list: %w", err)
	}
	return files, nil
}

// SignedURL returns a signed GET URL valid for expiry.
func (s *Storage) SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	signed, err := s.client.SignedURL(ctx, path, expiry)
	if err != nil {
		return "", fmt.Errorf("storage: gcs signed url: %w", err)
	}
	return signed, nil
}

type googleClient struct {
	bucket string
	client *gcstorage.Client
	cfg    *Config
}

func newGoogleClient(ctx context.Context, cfg *Config) (*googleClient, error) {
	var opts []option.ClientOption
	if cfg.Endpoint != "" && cfg.Endpoint != DefaultEndpoint {
		opts = append(opts, option.WithEndpoint(cfg.Endpoint))
	}
	if cfg.CredentialsFile != "" || len(cfg.CredentialsJSON) > 0 {
		creds, err := credentials.DetectDefault(&credentials.DetectOptions{
			Scopes:          []string{gcstorage.ScopeFullControl},
			CredentialsFile: cfg.CredentialsFile,
			CredentialsJSON: cfg.CredentialsJSON,
		})
		if err != nil {
			return nil, fmt.Errorf("storage: load gcs credentials: %w", err)
		}
		opts = append(opts, option.WithAuthCredentials(creds))
	}
	client, err := gcstorage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("storage: create gcs client: %w", err)
	}
	return &googleClient{bucket: cfg.Bucket, client: client, cfg: cfg}, nil
}

func (c *googleClient) object(path string) *gcstorage.ObjectHandle {
	return c.client.Bucket(c.bucket).Object(path)
}

func (c *googleClient) Put(ctx context.Context, path string, reader io.Reader) error {
	w := c.object(path).NewWriter(ctx)
	if _, err := io.Copy(w, reader); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func (c *googleClient) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	return c.object(path).NewReader(ctx)
}

func (c *googleClient) Delete(ctx context.Context, path string) error {
	err := c.object(path).Delete(ctx)
	if errors.Is(err, gcstorage.ErrObjectNotExist) {
		return nil
	}
	return err
}

func (c *googleClient) Exists(ctx context.Context, path string) (bool, error) {
	_, err := c.object(path).Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gcstorage.ErrObjectNotExist) {
		return false, nil
	}
	return false, err
}

func (c *googleClient) List(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	it := c.client.Bucket(c.bucket).Objects(ctx, &gcstorage.Query{Prefix: prefix})
	var files []storage.FileInfo
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		files = append(files, storage.FileInfo{Path: attrs.Name, Size: attrs.Size, LastModified: attrs.Updated, ContentType: attrs.ContentType})
	}
	return files, nil
}

func (c *googleClient) SignedURL(_ context.Context, path string, expiry time.Duration) (string, error) {
	if c.cfg.GoogleAccessID == "" || len(c.cfg.PrivateKey) == 0 {
		return "", fmt.Errorf("gcs: signed URLs require google_access_id and private_key")
	}
	return gcstorage.SignedURL(c.bucket, path, &gcstorage.SignedURLOptions{GoogleAccessID: c.cfg.GoogleAccessID, PrivateKey: c.cfg.PrivateKey, Method: "GET", Expires: time.Now().Add(expiry)})
}

var (
	_ storage.Storage           = (*Storage)(nil)
	_ storage.SignedURLProvider = (*Storage)(nil)
)
