package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestWriteAtomicCreatesFile(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "sub", "out.txt")
	if err := fs.WriteAtomic(dest, []byte("v1"), "test"); err != nil {
		t.Fatalf("write atomic: %v", err)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "v1" {
		t.Fatalf("read: %q %v", data, err)
	}
	// No leftover temp files in the destination directory.
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected only the destination file, got %d entries", len(entries))
	}
}

func TestWriteAtomicReplaceOverwrites(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "out.txt")
	if err := fs.WriteAtomic(dest, []byte("v1"), "test"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := fs.WriteAtomicReplace(dest, []byte("v2"), "test"); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "v2" {
		t.Fatalf("read: %q %v", data, err)
	}
}
