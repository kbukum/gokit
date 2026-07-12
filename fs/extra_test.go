package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestAbsolute(t *testing.T) {
	t.Parallel()
	abs, err := fs.Absolute("relative/path")
	if err != nil {
		t.Fatalf("absolute: %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("expected absolute path, got %q", abs)
	}
	already := filepath.Join(string(filepath.Separator), "etc", "hosts")
	if got, err := fs.Absolute(already); err != nil || got != already {
		t.Fatalf("absolute passthrough: %q %v", got, err)
	}
}

func TestCanonicalize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	got, err := fs.Canonicalize(link)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	want, _ := filepath.EvalSymlinks(target)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCanonicalizeMissingErrors(t *testing.T) {
	t.Parallel()
	if _, err := fs.Canonicalize(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestConfineExistingPathMissingErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := fs.ConfineExistingPath(root, "does/not/exist.txt"); err == nil {
		t.Fatal("expected error for missing confined path")
	}
}

func TestConfineRootMustBeDirectory(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := fs.ConfinePath(file, "child.txt"); err == nil {
		t.Fatal("expected error when root is not a directory")
	}
}

func TestCanWriteMissingPath(t *testing.T) {
	t.Parallel()
	if ok, err := fs.CanWrite(filepath.Join(t.TempDir(), "missing")); err != nil || ok {
		t.Fatalf("CanWrite missing: %t %v", ok, err)
	}
}
