package embedded_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/git/embedded"
)

// canceledContext returns a context that is already canceled, so a remote call
// must observe cancellation rather than performing the transfer.
func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestFetchCanceledContextPropagates(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	createRemote(t, dir)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = repo.Fetch(canceledContext(t), "origin")
	if err == nil {
		t.Fatal("Fetch(canceled) expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch(canceled) error = %v, want errors.Is(context.Canceled)", err)
	}
	var appErr *goerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("Fetch(canceled) error = %T, want typed *goerrors.AppError", err)
	}
}

func TestPushCanceledContextPropagates(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	createRemote(t, dir)
	commitFile(t, dir, "local.txt", "local change", "local change")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = repo.Push(canceledContext(t), "origin")
	if err == nil {
		t.Fatal("Push(canceled) expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Push(canceled) error = %v, want errors.Is(context.Canceled)", err)
	}
	var appErr *goerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("Push(canceled) error = %T, want typed *goerrors.AppError", err)
	}
}

func TestCloneCanceledContextPropagates(t *testing.T) {
	t.Parallel()

	source := initTestRepo(t)
	remote := createRemote(t, source)
	cloneDir := filepath.Join(t.TempDir(), "clone")

	_, err := embedded.Clone(canceledContext(t), remote, cloneDir, nil)
	if err == nil {
		t.Fatal("Clone(canceled) expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Clone(canceled) error = %v, want errors.Is(context.Canceled)", err)
	}
}

// TestFetchExpiredDeadlinePropagates proves a timed-out remote call surfaces a
// deadline error the caller can detect, not a swallowed success.
func TestFetchExpiredDeadlinePropagates(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	createRemote(t, dir)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	err = repo.Fetch(ctx, "origin")
	if err == nil {
		t.Fatal("Fetch(expired deadline) expected error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch(expired deadline) error = %v, want deadline/cancel", err)
	}
}
