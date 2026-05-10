package embedded_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/git/embedded"
)

func TestOpen(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if repo.Root() != dir {
		absDir, _ := filepath.Abs(dir)
		if repo.Root() != absDir {
			t.Errorf("Root() = %q, want %q", repo.Root(), absDir)
		}
	}
}

func TestOpenNonExistent(t *testing.T) {
	t.Parallel()
	if _, err := embedded.Open("/nonexistent/path", nil); err == nil {
		t.Fatal("Open() expected error")
	}
}

func TestDiscover(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	subdir := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := embedded.Discover(subdir, nil)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if repo.Root() == subdir {
		t.Error("Discover() root should be repo root")
	}
}

func TestHead(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := repo.Head()
	if err != nil {
		t.Fatalf("Head() error: %v", err)
	}
	if ref.Target.IsZero() {
		t.Error("Head() target is zero OID")
	}
}

func TestResolveRef(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	oid, err := repo.ResolveRef("feature")
	if err != nil {
		t.Fatalf("ResolveRef() error: %v", err)
	}
	if oid.IsZero() {
		t.Error("ResolveRef() returned zero OID")
	}
}

func TestResolveRefNotFound(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveRef("nonexistent-branch"); err == nil {
		t.Fatal("ResolveRef() expected error")
	}
}

func TestIsDirty(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	dirty, err := repo.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty() error: %v", err)
	}
	if dirty {
		t.Error("IsDirty() = true for clean repo")
	}
	makeDirty(t, dir, "README.md")
	dirty, err = repo.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty() error: %v", err)
	}
	if !dirty {
		t.Error("IsDirty() = false for dirty repo")
	}
}
