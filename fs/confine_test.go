package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
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
	assertConfineRejected(t, err)
}

func TestConfinePathRejectsLexicalTraversalEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := fs.ConfinePath(root, "../escape.txt")
	assertConfineRejected(t, err)
}

func TestConfinePathRejectsSymlinkParentEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := fs.ConfinePath(root, "link/new.txt")
	assertConfineRejected(t, err)
}

func TestConfineExistingPathRejectsPrefixSiblingEscape(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	sibling := filepath.Join(parent, "root-evil")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(sibling, "secret.txt")
	if err := os.WriteFile(secret, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := fs.ConfineExistingPath(root, secret)
	assertConfineRejected(t, err)
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

func assertConfineRejected(t *testing.T, err error) {
	t.Helper()
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("error = %T, want *errors.AppError", err)
	}
	if appErr.Code != goerrors.ErrCodeInvalidInput {
		t.Fatalf("code = %s, want %s", appErr.Code, goerrors.ErrCodeInvalidInput)
	}
}
