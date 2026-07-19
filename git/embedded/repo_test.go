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

func TestInitAndInitBare(t *testing.T) {
	t.Parallel()

	repoDir := filepath.Join(t.TempDir(), "repo")
	repo, err := embedded.Init(repoDir, nil)
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		t.Fatalf(".git stat error: %v", err)
	}
	if repo.Root() != repoDir {
		absDir, _ := filepath.Abs(repoDir)
		if repo.Root() != absDir {
			t.Fatalf("Root() = %q, want %q", repo.Root(), absDir)
		}
	}

	bareDir := filepath.Join(t.TempDir(), "repo.git")
	bare, err := embedded.InitBare(bareDir, nil)
	if err != nil {
		t.Fatalf("InitBare() error: %v", err)
	}
	if bare.Root() != bareDir {
		absDir, _ := filepath.Abs(bareDir)
		if bare.Root() != absDir {
			t.Fatalf("Root() = %q, want %q", bare.Root(), absDir)
		}
	}
	if _, err := os.Stat(filepath.Join(bareDir, "HEAD")); err != nil {
		t.Fatalf("bare HEAD stat error: %v", err)
	}
}

func TestClone(t *testing.T) {
	t.Parallel()

	source := initTestRepo(t)
	remote := createRemote(t, source)
	cloneDir := filepath.Join(t.TempDir(), "clone")

	repo, err := embedded.Clone(remote, cloneDir, nil)
	if err != nil {
		t.Fatalf("Clone() error: %v", err)
	}
	if repo.Root() != cloneDir {
		t.Fatalf("Root() = %q, want %q", repo.Root(), cloneDir)
	}
	if _, err := repo.Head(); err != nil {
		t.Fatalf("Head() error: %v", err)
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

func TestDiscoverNotFound(t *testing.T) {
	t.Parallel()

	if _, err := embedded.Discover(t.TempDir(), nil); err == nil {
		t.Fatal("Discover() expected error outside repository")
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

func TestHeadDetached(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	head := revParse(t, dir, "HEAD")
	runGit(t, dir, "checkout", "--detach", head)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Head(); err == nil {
		t.Fatal("Head() expected detached HEAD error")
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
