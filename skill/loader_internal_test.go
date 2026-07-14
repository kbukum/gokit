package skill

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRejectSymlinkSegmentsFailsClosedOnEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside", "x.txt")
	if err := rejectSymlinkSegments(root, outside); !errors.Is(err, ErrInvalidPackFile) {
		t.Fatalf("want ErrInvalidPackFile for path escaping root, got %v", err)
	}
}
