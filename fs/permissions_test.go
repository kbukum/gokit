package fs_test

import (
	"os"
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

func TestSetReadonlyTogglesWriteBit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fs.SetReadonly(path, true); err != nil {
		t.Fatal(err)
	}
	readonly, err := fs.IsReadonly(path)
	if err != nil || !readonly {
		t.Fatalf("expected readonly, got %v, %v", readonly, err)
	}
	if err := fs.SetReadonly(path, false); err != nil {
		t.Fatal(err)
	}
	readonly, err = fs.IsReadonly(path)
	if err != nil || readonly {
		t.Fatalf("expected writable, got %v, %v", readonly, err)
	}
}

func TestPermissions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	mode, err := fs.Permissions(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Perm() != 0o640 {
		t.Fatalf("Permissions = %o, want 640", mode.Perm())
	}
}
