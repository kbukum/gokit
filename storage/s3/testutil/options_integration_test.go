//go:build integration

package testutil

import (
	"os"
	"testing"
)

// ImageEnv selects an alternate immutable fixture image for the real suite, such as a consumer's source-built artifact.
const ImageEnv = "GOKIT_S3_FIXTURE_IMAGE"

// IntegrationOptions returns the fixture options selected by ImageEnv; unset runs the pinned S3Image.
func IntegrationOptions(t testing.TB) []Option {
	t.Helper()
	ref, set := os.LookupEnv(ImageEnv)
	if !set {
		return nil
	}
	return []Option{WithImage(ref)}
}
