package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHardLink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	original := filepath.Join(dir, "original.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(original, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := HardLink(original, link); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(link)
	if err != nil || string(content) != "hello" {
		t.Fatalf("hard link content = %q, %v", content, err)
	}
}

func TestHardLinkMissingSourceErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := HardLink(filepath.Join(dir, "missing"), filepath.Join(dir, "link")); err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestSymlinkFileAndReadLink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	original := filepath.Join(dir, "original.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(original, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SymlinkFile(original, link); err != nil {
		t.Fatal(err)
	}
	target, err := ReadLink(link)
	if err != nil || target != original {
		t.Fatalf("ReadLink = %q, %v, want %q", target, err, original)
	}
}

func TestSymlinkDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	original := filepath.Join(dir, "sub")
	link := filepath.Join(dir, "link")
	if err := os.MkdirAll(original, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SymlinkDir(original, link); err != nil {
		t.Fatal(err)
	}
	target, err := ReadLink(link)
	if err != nil || target != original {
		t.Fatalf("ReadLink = %q, %v", target, err)
	}
}

func TestReadLinkMissingErrors(t *testing.T) {
	t.Parallel()
	if _, err := ReadLink(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error for missing link")
	}
}
