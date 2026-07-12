package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestConfineExistingPathAcceptsInside(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inside := filepath.Join(root, "sub", "file.txt")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(inside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := fs.ConfineExistingPath(root, "sub/file.txt")
	if err != nil {
		t.Fatalf("confine: %v", err)
	}
	wantResolved, _ := filepath.EvalSymlinks(inside)
	if got != wantResolved {
		t.Fatalf("got %q want %q", got, wantResolved)
	}
}

func TestConfineExistingPathRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("s"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	_, err := fs.ConfineExistingPath(root, "escape")
	var appErr interface{ Error() string }
	if !errors.As(err, &appErr) {
		t.Fatalf("expected error, got nil")
	}
}

func TestConfinePathAllowsMissingSuffix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got, err := fs.ConfinePath(root, "new/dir/file.txt")
	if err != nil {
		t.Fatalf("confine: %v", err)
	}
	rootResolved, _ := filepath.EvalSymlinks(root)
	want := filepath.Join(rootResolved, "new", "dir", "file.txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestConfinePathRejectsAbsoluteEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	_, err := fs.ConfinePath(root, filepath.Join(outside, "x.txt"))
	if err == nil {
		t.Fatal("expected escape to be rejected")
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
