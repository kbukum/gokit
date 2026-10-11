//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows

package fs_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kbukum/gokit/fs"
)

func TestFileLockProcessOwner(t *testing.T) {
	path := os.Getenv("GOKIT_LOCK_TEST_PATH")
	if path == "" {
		return
	}
	err := fs.WithFileLock(t.Context(), path, func() error {
		fmt.Fprintln(os.Stdout, "locked")
		_, err := bufio.NewReader(os.Stdin).ReadByte()
		if err != nil {
			return err
		}
		// Exit without unwinding the lock to prove the OS releases it on owner death.
		os.Exit(0)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFileLockCoordinatesProcessesAndSurvivesOwnerExit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bundle.lock")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFileLockProcessOwner$")
	cmd.Env = append(os.Environ(), "GOKIT_LOCK_TEST_PATH="+path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := false
	t.Cleanup(func() {
		if !reaped {
			cancel()
			_ = cmd.Wait()
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("lock owner not ready: %q, %v", line, err)
	}
	wait, stop := context.WithTimeout(ctx, time.Millisecond)
	err = fs.WithFileLock(wait, path, func() error {
		t.Fatal("another process held the lock")
		return nil
	})
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contention did not honor deadline: %v", err)
	}
	if _, err := stdin.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	reaped = true
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.WithFileLock(ctx, path, func() error { return nil }); err != nil {
		t.Fatalf("dead owner retained lock: %v", err)
	}
}
