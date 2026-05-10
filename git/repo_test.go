package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/git"
)

func TestOpenDiscoverAndExec(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)

	repo, err := git.Open(dir)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if repo.Root() != dir {
		absDir, _ := filepath.Abs(dir)
		if repo.Root() != absDir {
			t.Fatalf("Root() = %q, want %q", repo.Root(), absDir)
		}
	}

	subdir := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	discovered, err := git.Discover(subdir)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if discovered.Root() != repo.Root() {
		t.Fatalf("Discover().Root() = %q, want %q", discovered.Root(), repo.Root())
	}

	out, err := repo.Exec("rev-parse", "--is-inside-work-tree")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if got := stringTrimSpace(string(out)); got != "true" {
		t.Fatalf("Exec() = %q, want true", got)
	}
}

func TestClone(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	remoteDir := createRemote(t, dir)
	cloneDir := filepath.Join(t.TempDir(), "clone")

	repo, err := git.Clone(remoteDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone() error: %v", err)
	}
	if repo.Root() != cloneDir {
		t.Fatalf("Clone().Root() = %q, want %q", repo.Root(), cloneDir)
	}
	if _, err := repo.Head(); err != nil {
		t.Fatalf("Head() after clone error: %v", err)
	}
}

func TestInit(t *testing.T) {
	t.Parallel()
	repoDir := filepath.Join(t.TempDir(), "repo")

	repo, err := git.Init(repoDir)
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	absDir, _ := filepath.Abs(repoDir)
	if repo.Root() != absDir {
		t.Fatalf("Root() = %q, want %q", repo.Root(), absDir)
	}

	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		t.Fatalf(".git stat error: %v", err)
	}

	out, err := repo.Exec("rev-parse", "--is-inside-work-tree")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if got := stringTrimSpace(string(out)); got != "true" {
		t.Fatalf("Exec() = %q, want true", got)
	}
}

func TestInitBare(t *testing.T) {
	t.Parallel()
	repoDir := filepath.Join(t.TempDir(), "repo.git")

	repo, err := git.InitBare(repoDir)
	if err != nil {
		t.Fatalf("InitBare() error: %v", err)
	}

	absDir, _ := filepath.Abs(repoDir)
	if repo.Root() != absDir {
		t.Fatalf("Root() = %q, want %q", repo.Root(), absDir)
	}

	if _, err := os.Stat(filepath.Join(repoDir, "HEAD")); err != nil {
		t.Fatalf("HEAD stat error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "objects")); err != nil {
		t.Fatalf("objects stat error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git stat error = %v, want not exist", err)
	}
}
