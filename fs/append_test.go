package fs_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestOpenAppendCreatesOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	f, err := fs.OpenAppend(path)
	if err != nil {
		t.Fatalf("OpenAppend: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if _, err := f.WriteString("line\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("created perms = %o, want 600", perm)
		}
	}
}

func TestOpenAppendPreservesExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("prior\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, err := fs.OpenAppend(path)
	if err != nil {
		t.Fatalf("OpenAppend: %v", err)
	}
	if _, err := f.WriteString("next\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "prior\nnext\n" {
		t.Fatalf("content = %q, want appended", string(data))
	}
}

func TestOpenAppendRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.log")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	f, err := fs.OpenAppend(link)
	if err == nil {
		_ = f.Close()
		t.Fatalf("expected symlink target to be rejected")
	}
}

func TestOpenAppendRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	f, err := fs.OpenAppend(dir)
	if err == nil {
		_ = f.Close()
		t.Fatalf("expected directory target to be rejected")
	}
}
