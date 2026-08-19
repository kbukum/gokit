package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFsChangeBatchCoalescing(t *testing.T) {
	t.Parallel()
	batch := newFsChangeBatch(map[string]struct{}{
		"/repo/b.go": {},
		"/repo/a.go": {},
	}, true)
	if !batch.RescanRequested() {
		t.Fatal("expected rescan flag")
	}
	paths := batch.Paths()
	if len(paths) != 2 || paths[0] != "/repo/a.go" || paths[1] != "/repo/b.go" {
		t.Fatalf("paths not sorted/deduped: %v", paths)
	}
	if !batch.Any(func(p string) bool { return p == "/repo/a.go" }) {
		t.Fatal("Any should find a.go")
	}
}

func TestFsChangeBatchRescanOnlyNotEmpty(t *testing.T) {
	t.Parallel()
	batch := newFsChangeBatch(map[string]struct{}{}, true)
	if batch.IsEmpty() {
		t.Fatal("rescan-only batch should not be empty")
	}
	if batch.Len() != 0 {
		t.Fatalf("Len = %d, want 0", batch.Len())
	}
}

func TestWatchEmptyRootsErrors(t *testing.T) {
	t.Parallel()
	if _, err := NewFsWatcher(50*time.Millisecond).Watch(context.Background(), nil); err == nil {
		t.Fatal("expected empty-roots error")
	}
}

func TestWatchMissingRootErrors(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := NewFsWatcher(50*time.Millisecond).Watch(context.Background(), []string{missing}); err == nil {
		t.Fatal("expected missing-root error")
	}
}

func TestWatchDeliversBatchOnWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changes, err := NewFsWatcher(50*time.Millisecond).Watch(ctx, []string{root})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "touched.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case batch := <-changes:
		if batch.IsEmpty() {
			t.Fatal("expected a non-empty change batch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for change batch")
	}
}
