//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows

package fs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/fs"
)

func TestWithFileLockSerializesAndReleases(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bundle.lock")
	dataPath := path + ".count"
	if err := os.WriteFile(dataPath, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var active atomic.Int32
	results := make(chan error, 16)
	for range cap(results) {
		wg.Go(func() {
			results <- fs.WithFileLock(t.Context(), path, func() error {
				if active.Add(1) != 1 {
					return errors.New("overlapping lock holders")
				}
				defer active.Add(-1)
				data, err := os.ReadFile(dataPath)
				if err != nil {
					return err
				}
				count, err := strconv.Atoi(string(data))
				if err != nil {
					return err
				}
				return os.WriteFile(dataPath, []byte(strconv.Itoa(count+1)), 0o600)
			})
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(dataPath)
	if err != nil || string(data) != strconv.Itoa(cap(results)) {
		t.Fatalf("lost updates: %q, %v", data, err)
	}
	cause := errors.New("operation failed")
	if err := fs.WithFileLock(t.Context(), path, func() error { return cause }); !errors.Is(err, cause) {
		t.Fatalf("operation cause lost: %v", err)
	}
	if err := fs.WithFileLock(t.Context(), path, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestWithFileLockCancellationAndInvalidFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.lock")
	err := fs.WithFileLock(t.Context(), path, func() error {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := fs.WithFileLock(ctx, path, func() error {
			t.Fatal("canceled caller entered")
			return nil
		}); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
		ctx, cancel = context.WithTimeout(t.Context(), time.Millisecond)
		defer cancel()
		if err := fs.WithFileLock(ctx, path, func() error {
			t.Fatal("contending caller entered")
			return nil
		}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline lost: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{link, dir, ""} {
		if err := fs.WithFileLock(t.Context(), invalid, func() error {
			t.Fatal("invalid lock admitted")
			return nil
		}); err == nil {
			t.Fatal("invalid lock accepted")
		}
	}
	if err := fs.WithFileLock(t.Context(), path, nil); err == nil {
		t.Fatal("nil operation accepted")
	}
}
