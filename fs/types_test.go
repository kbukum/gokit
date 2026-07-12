package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestMetadataAndReadDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	meta, err := fs.Metadata(file)
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if !meta.IsFile || meta.IsDir || meta.Len != 5 {
		t.Fatalf("unexpected meta: %+v", meta)
	}

	entries, err := fs.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	var sawFile, sawDir bool
	for _, e := range entries {
		if e.Name == "a.txt" && e.IsFile {
			sawFile = true
		}
		if e.Name == "sub" && e.IsDir {
			sawDir = true
		}
	}
	if !sawFile || !sawDir {
		t.Fatalf("entries missing expected file/dir: %+v", entries)
	}
}

func TestMetadataMissingPathErrors(t *testing.T) {
	t.Parallel()
	if _, err := fs.Metadata(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing path")
	}
}
