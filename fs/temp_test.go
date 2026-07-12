package fs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestTempFilePersistAndRemove(t *testing.T) {
	t.Parallel()
	tf, err := fs.NewTempFile()
	if err != nil {
		t.Fatalf("new temp file: %v", err)
	}
	if _, err := tf.File().WriteString("hello"); err != nil {
		t.Fatalf("write: %v", err)
	}
	target := filepath.Join(t.TempDir(), "persisted.txt")
	if _, err := tf.Persist(target); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "hello" {
		t.Fatalf("read persisted: %q %v", data, err)
	}
	// Remove is a no-op after persist and must not delete the persisted file.
	if err := tf.Remove(); err != nil {
		t.Fatalf("remove after persist: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("persisted file missing: %v", err)
	}
}

func TestTempFileRemove(t *testing.T) {
	t.Parallel()
	tf, err := fs.NewTempFile()
	if err != nil {
		t.Fatalf("new temp file: %v", err)
	}
	path := tf.Path()
	if err := tf.Remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file not removed: %v", err)
	}
}

func TestTempDirWriteAndChild(t *testing.T) {
	t.Parallel()
	dir, err := fs.NewTempDir()
	if err != nil {
		t.Fatalf("new temp dir: %v", err)
	}
	defer func() { _ = dir.Remove() }()

	written, err := dir.WriteFile("nested/file.txt", []byte("data"))
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
	if data, err := os.ReadFile(written); err != nil || string(data) != "data" {
		t.Fatalf("read: %q %v", data, err)
	}
	if _, err := dir.Child("../escape"); err == nil {
		t.Fatal("child should reject traversal")
	}
}

func TestTempDirRemove(t *testing.T) {
	t.Parallel()
	dir, err := fs.NewTempDir()
	if err != nil {
		t.Fatalf("new temp dir: %v", err)
	}
	path := dir.Path()
	if err := dir.Remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp dir not removed: %v", err)
	}
}

func TestSiblingTempPath(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "config.json")
	got := fs.SiblingTempPath(dest, "svc/prefix", ".tmp")
	if filepath.Dir(got) != filepath.Dir(dest) {
		t.Fatalf("sibling not next to dest: %q", got)
	}
	name := filepath.Base(got)
	if strings.ContainsAny(name, "/\\") {
		t.Fatalf("name must be a single component: %q", name)
	}
	if !strings.HasSuffix(name, ".tmp") {
		t.Fatalf("suffix not applied: %q", name)
	}
	// Unsafe prefix characters must be sanitized away.
	if strings.Contains(name, "svc/prefix") {
		t.Fatalf("prefix not sanitized: %q", name)
	}
	// Two calls must not collide.
	other := fs.SiblingTempPath(dest, "svc-prefix", ".tmp")
	if other == got {
		t.Fatal("sibling temp paths collided")
	}
}
