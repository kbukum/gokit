package fs_test

import (
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestCanWriteMissingPath(t *testing.T) {
	t.Parallel()
	if ok, err := fs.CanWrite(filepath.Join(t.TempDir(), "missing")); err != nil || ok {
		t.Fatalf("CanWrite missing: %t %v", ok, err)
	}
}
