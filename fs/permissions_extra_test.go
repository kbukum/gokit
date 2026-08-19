package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetReadonlyTogglesWriteBit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetReadonly(path, true); err != nil {
		t.Fatal(err)
	}
	readonly, err := IsReadonly(path)
	if err != nil || !readonly {
		t.Fatalf("expected readonly, got %v, %v", readonly, err)
	}
	if err := SetReadonly(path, false); err != nil {
		t.Fatal(err)
	}
	readonly, err = IsReadonly(path)
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
	mode, err := Permissions(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Perm() != 0o640 {
		t.Fatalf("Permissions = %o, want 640", mode.Perm())
	}
}
