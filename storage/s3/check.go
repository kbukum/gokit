package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// Check verifies access to the configured bucket without mutating it.
func (s *Storage) Check(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("storage: s3 bucket readiness: %w", err)
	}
	return nil
}
