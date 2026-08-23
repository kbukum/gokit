package git_test

import (
	stderrors "errors"
	"os"
	"path/filepath"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/git"
	"github.com/kbukum/gokit/git/auth"
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

	head, err := repo.Exec("symbolic-ref", "HEAD")
	if err != nil {
		t.Fatalf("symbolic-ref HEAD error: %v", err)
	}
	if got := stringTrimSpace(string(head)); got != "refs/heads/"+git.DefaultBranch {
		t.Fatalf("HEAD = %q, want refs/heads/%s", got, git.DefaultBranch)
	}
}

func TestOpenWithAuthAppliesTransport(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	provider := &countingTokenProvider{token: "secret"}

	repo, err := git.OpenWithAuth(dir, auth.Token{Provider: provider})
	if err != nil {
		t.Fatalf("OpenWithAuth() error: %v", err)
	}

	err = repo.Fetch("missing")
	if err == nil {
		t.Fatal("Fetch(missing) expected error")
	}
	if provider.calls != 1 {
		t.Fatalf("token provider calls = %d, want 1", provider.calls)
	}
}

func TestDiscoverWithAuthAppliesTransport(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := &countingTokenProvider{token: "secret"}

	repo, err := git.DiscoverWithAuth(subdir, auth.Token{Provider: provider})
	if err != nil {
		t.Fatalf("DiscoverWithAuth() error: %v", err)
	}

	err = repo.Fetch("missing")
	if err == nil {
		t.Fatal("Fetch(missing) expected error")
	}
	if provider.calls != 1 {
		t.Fatalf("token provider calls = %d, want 1", provider.calls)
	}
}

func TestCreateSignedTagWithoutConfiguredKey(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	fakeGit := filepath.Join(t.TempDir(), "fake-git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nif [ \"$1\" = \"config\" ]; then exit 1; fi\necho unexpected git \"$@\" >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fake-git): %v", err)
	}
	repo, err := git.Open(dir, git.WithCLIPath(fakeGit))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	err = repo.CreateSignedTag("v1.0.0", "HEAD", "release", git.SignOptions{})
	if err == nil {
		t.Fatal("CreateSignedTag() expected error")
	}
	var appErr *goerrors.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("error = %T %v, want AppError", err, err)
	}
	if appErr.Details["gokit_git_error"] != "signing_key_missing" {
		t.Fatalf("gokit_git_error = %#v, want signing_key_missing", appErr.Details["gokit_git_error"])
	}
}

type countingTokenProvider struct {
	token string
	calls int
}

func (p *countingTokenProvider) Token() (string, error) {
	p.calls++
	return p.token, nil
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
