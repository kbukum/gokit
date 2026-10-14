package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/auth"
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
	return reg.Register(ProviderName, func(ctx context.Context, _ storage.Config, _ *logging.Logger) (storage.Storage, error) {
		return NewStorage(ctx, &c)
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
	Head(ctx context.Context, path string) (storage.FileInfo, error)
	Copy(ctx context.Context, srcPath, dstPath string) error
	SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error)
	Close() error
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

// NewStorageWithClient creates storage with an injected client for tests. Storage owns the client and releases it on Close.
func NewStorageWithClient(bucket, publicURL string, client objectClient) *Storage {
	return &Storage{bucket: bucket, publicURL: strings.TrimRight(publicURL, "/"), client: client}
}

// Close releases the owned GCS client. Consumers must drain operations first; a failed close may be retried.
func (s *Storage) Close(context.Context) error {
	return s.client.Close()
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
		if errors.Is(err, gcstorage.ErrObjectNotExist) {
			return nil, storage.NotFoundError(path)
		}
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

var errClientClosed = errors.New("storage: gcs client is closed")

// googleClient refuses use after Close because the SDK client panics once released.
type googleClient struct {
	bucket string
	cfg    *Config
	mu     sync.RWMutex
	client *gcstorage.Client
	closed bool
}

func newGoogleClient(ctx context.Context, cfg *Config) (*googleClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("storage: gcs config is nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	var opts []option.ClientOption
	if cfg.Endpoint != "" && cfg.Endpoint != DefaultEndpoint {
		opts = append(opts, option.WithEndpoint(cfg.Endpoint))
	}
	if cfg.CredentialsFile != "" || len(cfg.CredentialsJSON) > 0 {
		var creds *auth.Credentials
		var err error
		detect := &credentials.DetectOptions{Scopes: []string{gcstorage.ScopeFullControl}}
		if cfg.CredentialsFile != "" {
			creds, err = credentials.NewCredentialsFromFile(credentials.ServiceAccount, cfg.CredentialsFile, detect)
		} else {
			creds, err = credentials.NewCredentialsFromJSON(credentials.ServiceAccount, cfg.CredentialsJSON, detect)
		}
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

func (c *googleClient) bucketHandle() (*gcstorage.BucketHandle, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, errClientClosed
	}
	return c.client.Bucket(c.bucket), nil
}

func (c *googleClient) object(path string) (*gcstorage.ObjectHandle, error) {
	bucket, err := c.bucketHandle()
	if err != nil {
		return nil, err
	}
	return bucket.Object(path), nil
}

// Close releases the SDK client. Later operations fail; a failed release may be retried.
func (c *googleClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil
	}
	c.closed = true
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("storage: gcs close: %w", err)
	}
	c.client = nil
	return nil
}

func (c *googleClient) Put(ctx context.Context, path string, reader io.Reader) error {
	obj, err := c.object(path)
	if err != nil {
		return err
	}
	w := obj.NewWriter(ctx)
	if _, err := io.Copy(w, reader); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func (c *googleClient) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	obj, err := c.object(path)
	if err != nil {
		return nil, err
	}
	return obj.NewReader(ctx)
}

func (c *googleClient) Delete(ctx context.Context, path string) error {
	obj, err := c.object(path)
	if err != nil {
		return err
	}
	err = obj.Delete(ctx)
	if errors.Is(err, gcstorage.ErrObjectNotExist) {
		return nil
	}
	return err
}

func (c *googleClient) Exists(ctx context.Context, path string) (bool, error) {
	obj, err := c.object(path)
	if err != nil {
		return false, err
	}
	_, err = obj.Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gcstorage.ErrObjectNotExist) {
		return false, nil
	}
	return false, err
}

func (c *googleClient) List(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	bucket, err := c.bucketHandle()
	if err != nil {
		return nil, err
	}
	it := bucket.Objects(ctx, &gcstorage.Query{Prefix: prefix})
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
	_ storage.Closer            = (*Storage)(nil)
)
