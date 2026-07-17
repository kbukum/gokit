package collect

import (
	"testing"

	"github.com/kbukum/gokit/dataset/manifest"
	"github.com/kbukum/gokit/dataset/stage"
)

// TestNullProgressNoOps documents that NullProgress implements Progress as
// no-ops that never panic.
func TestNullProgressNoOps(t *testing.T) {
	t.Parallel()
	var p Progress = NullProgress{}
	p.OnSourceStart(0, "s", 1, true)
	p.OnSourceProgress(0, 1)
	p.OnSourceDone(0, "s", manifest.SourceStats{})
	p.OnSourceCached(0, "s", manifest.SourceStats{})
	p.OnSourceError(0, "s", nil)
	p.OnPublishDone("t", stage.PublishResult{})
}
