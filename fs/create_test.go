package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestWriteAtomicCreateNeverReplaces(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "secret")
	var wg sync.WaitGroup
	results := make(chan error, 16)
	for range cap(results) {
		wg.Go(func() { results <- fs.WriteAtomicCreate(dest, []byte("complete"), ".test") })
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("successful creates = %d, want 1", winners)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "complete" {
		t.Fatalf("published file is incomplete: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("secret permissions: %v, %v", info, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files remain: %v, %v", entries, err)
	}
}

func TestWriteAtomicCreateFailureAndSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, dest := range []string{link, dir} {
		if err := fs.WriteAtomicCreate(dest, []byte("replacement"), ".test"); !errors.Is(err, os.ErrExist) {
			t.Fatalf("existing destination: %v", err)
		}
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "original" {
		t.Fatalf("target changed: %v", err)
	}
}
