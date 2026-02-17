package s3

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/storage"
)

func init() {
	storage.RegisterFactory(storage.ProviderS3, func(cfg storage.Config, providerCfg any, log *logger.Logger) (storage.Storage, error) {
		c := &Config{}
		if providerCfg != nil {
			pc, ok := providerCfg.(*Config)
			if !ok {
				return nil, fmt.Errorf("s3: expected *s3.Config, got %T", providerCfg)
			}
			c = pc
		}
		c.ApplyDefaults()
		if err := c.Validate(); err != nil {
			return nil, err
		}
		return NewStorage(context.Background(), c)
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
			o.UsePathStyle = cfg.ForcePathStyle || true
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
func (s *Storage) Exists(ctx context.Context, path string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return false, nil
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

func (s *Storage) resolveEndpoint() string {
	opts := s.client.Options()
	if opts.BaseEndpoint != nil && *opts.BaseEndpoint != "" {
		return *opts.BaseEndpoint
	}
	return fmt.Sprintf("https://s3.%s.amazonaws.com", opts.Region)
}

// compile-time check
var _ storage.Storage = (*Storage)(nil)
