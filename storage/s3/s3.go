package s3

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/storage"
)

// Register registers a configured S3 storage provider into the given registry. Pass an optional Config to override defaults.
func Register(reg *storage.FactoryRegistry, configs ...Config) error {
	if reg == nil {
		return fmt.Errorf("s3: storage registry is nil")
	}
	if len(configs) > 1 {
		return fmt.Errorf("s3: at most one config may be provided, got %d", len(configs))
	}
	c := Config{}
	if len(configs) > 0 {
		c = configs[0]
	}
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return err
	}
	return reg.Register(storage.ProviderS3, func(_ storage.Config, _ *logging.Logger) (storage.Storage, error) {
		return NewStorage(context.Background(), &c)
	})
}

// Storage implements storage.Storage using Amazon S3 (or S3-compatible services).
type Storage struct {
	client *awss3.Client
	bucket string
}

// NewStorage creates a new S3 storage client from the given config.
func NewStorage(ctx context.Context, cfg *Config) (*Storage, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("storage: load aws config: %w", err)
	}

	var s3Opts []func(*awss3.Options)
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *awss3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = cfg.ForcePathStyle
		})
	} else if cfg.ForcePathStyle {
		s3Opts = append(s3Opts, func(o *awss3.Options) {
			o.UsePathStyle = true
		})
	}

	client := awss3.NewFromConfig(awsCfg, s3Opts...)
	return &Storage{client: client, bucket: cfg.Bucket}, nil
}

// Upload writes data from reader to S3.
func (s *Storage) Upload(ctx context.Context, path string, reader io.Reader) error {
	_, err := s.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
		Body:   reader,
	})
	if err != nil {
		return fmt.Errorf("storage: s3 upload: %w", err)
	}
	return nil
}

// Download returns a reader for the S3 object at the given path.
func (s *Storage) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: s3 download: %w", err)
	}
	return out.Body, nil
}

// Delete removes an S3 object. Returns nil if the object does not exist.
func (s *Storage) Delete(ctx context.Context, path string) error {
	_, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return fmt.Errorf("storage: s3 delete: %w", err)
	}
	return nil
}

// Exists checks whether an S3 object exists.
//
// Any non-nil error from HeadObject is treated as "object does not exist" — the AWS SDK surfaces NotFound, NoSuchBucket, AccessDenied, etc. as distinct error types and Exists callers only care about the boolean. Real I/O failures surface on the next operation (Upload/Download/Delete).
func (s *Storage) Exists(ctx context.Context, path string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return false, nil //nolint:nilerr // see godoc: not-found semantics
	}
	return true, nil
}

// URL returns a public URL for the S3 object.
func (s *Storage) URL(_ context.Context, path string) (string, error) {
	endpoint := s.resolveEndpoint()
	return fmt.Sprintf("%s/%s/%s", endpoint, s.bucket, path), nil
}

// List returns metadata for all objects whose key starts with prefix.
func (s *Storage) List(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	input := &awss3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	}

	var files []storage.FileInfo
	for {
		out, err := s.client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("storage: s3 list: %w", err)
		}
		for _, obj := range out.Contents {
			fi := storage.FileInfo{
				Path: aws.ToString(obj.Key),
				Size: aws.ToInt64(obj.Size),
			}
			if obj.LastModified != nil {
				fi.LastModified = *obj.LastModified
			}
			files = append(files, fi)
		}
		if !aws.ToBool(out.IsTruncated) {
			break
		}
		input.ContinuationToken = out.NextContinuationToken
	}
	return files, nil
}

// SignedURL generates a pre-signed GET URL valid for the specified duration.
func (s *Storage) SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	presignClient := awss3.NewPresignClient(s.client)
	req, err := presignClient.PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	}, awss3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("storage: s3 presign: %w", err)
	}
	return req.URL, nil
}

func (s *Storage) resolveEndpoint() string {
	opts := s.client.Options()
	if opts.BaseEndpoint != nil && *opts.BaseEndpoint != "" {
		return *opts.BaseEndpoint
	}
	return fmt.Sprintf("https://s3.%s.amazonaws.com", opts.Region)
}

// compile-time check
var (
	_ storage.Storage           = (*Storage)(nil)
	_ storage.SignedURLProvider = (*Storage)(nil)
)
