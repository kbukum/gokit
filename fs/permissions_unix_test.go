//go:build unix

package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestCanReadAndCanWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if ok, err := fs.CanRead(file); err != nil || !ok {
		t.Fatalf("CanRead file: %t %v", ok, err)
	}
	if ok, err := fs.CanWrite(file); err != nil || !ok {
		t.Fatalf("CanWrite file: %t %v", ok, err)
	}
	if ok, err := fs.CanWrite(dir); err != nil || !ok {
		t.Fatalf("CanWrite dir: %t %v", ok, err)
	}
	if ok, err := fs.CanRead(filepath.Join(dir, "missing")); err != nil || ok {
		t.Fatalf("CanRead missing: %t %v", ok, err)
	}
}

func TestModeAndExecutable(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if exec, err := fs.IsExecutable(file); err != nil || exec {
		t.Fatalf("expected non-executable: %t %v", exec, err)
	}
	if err := fs.SetMode(file, 0o755); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	if exec, err := fs.IsExecutable(file); err != nil || !exec {
		t.Fatalf("expected executable after chmod: %t %v", exec, err)
	}
	if mode, err := fs.Mode(file); err != nil || mode.Perm() != 0o755 {
		t.Fatalf("mode: %v %v", mode, err)
	}
}

func TestIsReadonly(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o444); err != nil {
		t.Fatalf("write: %v", err)
	}
	if ro, err := fs.IsReadonly(file); err != nil || !ro {
		t.Fatalf("expected readonly: %t %v", ro, err)
	}
	if err := fs.SetMode(file, 0o644); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	if ro, err := fs.IsReadonly(file); err != nil || ro {
		t.Fatalf("expected writable: %t %v", ro, err)
	}
}
