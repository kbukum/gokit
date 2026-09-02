package storage

import (
	"errors"
	"testing"
)

func TestNotFoundErrorIsErrNotFound(t *testing.T) {
	t.Parallel()

	err := NotFoundError("dir/a.txt")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("NotFoundError should wrap ErrNotFound, got %v", err)
	}
	if got := err.Error(); got == "" || got == ErrNotFound.Error() {
		t.Errorf("NotFoundError should include the path, got %q", got)
	}
}

func TestCheckDistinctPaths(t *testing.T) {
	t.Parallel()

	if err := CheckDistinctPaths("Copy", "a.txt", "a.txt"); !errors.Is(err, ErrSameObject) {
		t.Fatalf("equal paths should yield ErrSameObject, got %v", err)
	}
	if err := CheckDistinctPaths("Copy", "a.txt", "b.txt"); err != nil {
		t.Fatalf("distinct paths should be accepted, got %v", err)
	}
}
