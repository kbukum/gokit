package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestReadFileLimitReadsWithinBound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFileLimit(path, 16)
	if err != nil {
		t.Fatalf("read within bound: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("data=%q", data)
	}
}

func TestReadFileLimitRejectsOversized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFileLimit(path, 8); !errors.Is(err, fs.ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge, got %v", err)
	}
}

func TestReadFileLimitRejectsDirectory(t *testing.T) {
	if _, err := fs.ReadFileLimit(t.TempDir(), 16); !errors.Is(err, fs.ErrNotRegularFile) {
		t.Fatalf("want ErrNotRegularFile, got %v", err)
	}
}

func TestReadFileLimitMissingFile(t *testing.T) {
	if _, err := fs.ReadFileLimit(filepath.Join(t.TempDir(), "missing"), 16); err == nil {
		t.Fatal("expected error for a missing file")
	}
}
