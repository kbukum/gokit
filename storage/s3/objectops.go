package s3

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/kbukum/gokit/storage"
)

// Head returns metadata for a single S3 object via HeadObject. Metadata is
// populated from the object's user metadata and Checksum from an explicit,
// algorithm-tagged S3 content checksum when present. The object's ETag is
// deliberately not used as a checksum: it is not a guaranteed content hash (for
// example single-part SSE-KMS/SSE-C objects carry a non-MD5 ETag).
func (s *Storage) Head(ctx context.Context, path string) (storage.FileInfo, error) {
	out, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(path),
		ChecksumMode: s3types.ChecksumModeEnabled,
	})
	if err != nil {
		if isNotFound(err) {
			return storage.FileInfo{}, storage.NotFoundError(path)
		}
		return storage.FileInfo{}, fmt.Errorf("storage: s3 head: %w", err)
	}
	fi := storage.FileInfo{
		Path:        path,
		Size:        aws.ToInt64(out.ContentLength),
		ContentType: aws.ToString(out.ContentType),
		Checksum:    headChecksum(out),
	}
	if out.LastModified != nil {
		fi.LastModified = *out.LastModified
	}
	if len(out.Metadata) > 0 {
		fi.Metadata = make(map[string]string, len(out.Metadata))
		for k, v := range out.Metadata {
			fi.Metadata[k] = v
		}
	}
	return fi, nil
}

// headChecksum renders an explicit S3 content checksum as an algorithm-tagged
// value (e.g. "sha256:<base64>"), preferring the strongest available algorithm.
// It returns "" when the object carries no explicit content checksum.
func headChecksum(out *awss3.HeadObjectOutput) string {
	switch {
	case out.ChecksumSHA256 != nil:
		return "sha256:" + aws.ToString(out.ChecksumSHA256)
	case out.ChecksumSHA1 != nil:
		return "sha1:" + aws.ToString(out.ChecksumSHA1)
	case out.ChecksumCRC32C != nil:
		return "crc32c:" + aws.ToString(out.ChecksumCRC32C)
	case out.ChecksumCRC32 != nil:
		return "crc32:" + aws.ToString(out.ChecksumCRC32)
	default:
		return ""
	}
}

// isNotFound reports whether err is S3's object-not-found condition. It matches
// the typed HeadObject NotFound and GetObject/CopyObject NoSuchKey errors, and
// falls back to any 404 response so Download and Copy translate a missing key
// even when the SDK surfaces it as a generic response error.
func isNotFound(err error) bool {
	var nf *s3types.NotFound
	var nsk *s3types.NoSuchKey
	if errors.As(err, &nf) || errors.As(err, &nsk) {
		return true
	}
	var re *awshttp.ResponseError
	return errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound
}

// Copy duplicates an object within the bucket using CopyObject.
func (s *Storage) Copy(ctx context.Context, srcPath, dstPath string) error {
	if err := storage.CheckDistinctPaths("Copy", srcPath, dstPath); err != nil {
		return err
	}
	if err := s.copyObject(ctx, srcPath, dstPath); err != nil {
		if isNotFound(err) {
			return storage.NotFoundError(srcPath)
		}
		return fmt.Errorf("storage: s3 copy: %w", err)
	}
	return nil
}

// Rename moves an object within the bucket by copying then deleting the source.
func (s *Storage) Rename(ctx context.Context, srcPath, dstPath string) error {
	if err := storage.CheckDistinctPaths("Rename", srcPath, dstPath); err != nil {
		return err
	}
	if err := s.copyObject(ctx, srcPath, dstPath); err != nil {
		if isNotFound(err) {
			return storage.NotFoundError(srcPath)
		}
		return fmt.Errorf("storage: s3 rename copy: %w", err)
	}
	if _, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(srcPath),
	}); err != nil {
		return fmt.Errorf("storage: s3 rename delete: %w", err)
	}
	return nil
}

// copyObject issues a server-side copy from srcPath to dstPath within the bucket.
func (s *Storage) copyObject(ctx context.Context, srcPath, dstPath string) error {
	_, err := s.client.CopyObject(ctx, &awss3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(dstPath),
		CopySource: aws.String(copySource(s.bucket, srcPath)),
	})
	return err
}

// copySource builds a URL-encoded x-amz-copy-source value, escaping each path
// segment while preserving the separators between the bucket and key.
func copySource(bucket, key string) string {
	segments := append([]string{bucket}, strings.Split(key, "/")...)
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}
