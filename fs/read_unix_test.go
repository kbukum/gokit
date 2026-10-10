//go:build unix

package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kbukum/gokit/fs"
)

func TestReadFileLimitRejectsFIFOWithoutWaitingForWriter(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := fs.ReadFileLimit(path, 4096)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, fs.ErrNotRegularFile) {
			t.Fatalf("FIFO rejection: %v", err)
		}
	case <-time.After(time.Second):
		writer, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
		<-done
		t.Fatal("regular-file reader blocked opening a FIFO")
	}
}
