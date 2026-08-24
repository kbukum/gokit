package embedded_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/git"
	"github.com/kbukum/gokit/git/embedded"
)

func TestListRemotes(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createRemote(t, dir)
	backupDir := t.TempDir()
	runGit(t, backupDir, "init", "--bare")
	runGit(t, dir, "remote", "add", "backup", backupDir)
	runGit(t, dir, "config", "--add", "remote.backup.push", "refs/heads/*:refs/heads/*")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := repo.ListRemotes()
	if err != nil {
		t.Fatalf("ListRemotes() error: %v", err)
	}
	if len(remotes) != 2 {
		t.Fatalf("ListRemotes() returned %d remotes, want 2", len(remotes))
	}
	if remotes[0].Name != "backup" || remotes[1].Name != "origin" {
		t.Fatalf("unexpected remote order: %#v", remotes)
	}
	if got := remotes[0].PushSpecs; len(got) != 1 || got[0] != "refs/heads/*:refs/heads/*" {
		t.Fatalf("backup push specs = %v", got)
	}
}

func TestFetch(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	remoteDir := createRemote(t, dir)
	mainBranch := currentBranch(t, dir)
	cloneRoot := t.TempDir()
	cloneDir := filepath.Join(cloneRoot, "clone")
	runGit(t, cloneRoot, "clone", remoteDir, cloneDir)
	runGit(t, cloneDir, "config", "user.email", "test@test.com")
	runGit(t, cloneDir, "config", "user.name", "Test User")
	commitFile(t, cloneDir, "remote.txt", "remote change", "remote change")
	runGit(t, cloneDir, "push", "origin", "HEAD:refs/heads/"+mainBranch)
	want := stringTrimSpace(runGit(t, dir, "-c", "safe.bareRepository=all", "--git-dir", remoteDir, "rev-parse", "refs/heads/"+mainBranch))
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Fetch(context.Background(), "origin", git.WithFetchPrune(true)); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	got, err := repo.ResolveRef("origin/" + mainBranch)
	if err != nil {
		t.Fatalf("ResolveRef() error: %v", err)
	}
	if got.String() != want {
		t.Fatalf("origin/%s = %s, want %s", mainBranch, got.String(), want)
	}
}

func TestFetchAndPushMissingRemote(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Fetch(context.Background(), "missing"); err == nil {
		t.Fatal("Fetch(missing) expected error")
	}
	if err := repo.Push(context.Background(), "missing"); err == nil {
		t.Fatal("Push(missing) expected error")
	}
}

func TestPush(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	remoteDir := createRemote(t, dir)
	mainBranch := currentBranch(t, dir)
	commitFile(t, dir, "local.txt", "local change", "local change")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	refspec := "refs/heads/" + mainBranch + ":refs/heads/" + mainBranch
	if err := repo.Push(context.Background(), "origin", git.WithPushRefspecs(refspec)); err != nil {
		t.Fatalf("Push() error: %v", err)
	}
	got := stringTrimSpace(runGit(t, dir, "-c", "safe.bareRepository=all", "--git-dir", remoteDir, "rev-parse", "refs/heads/"+mainBranch))
	want := revParse(t, dir, "HEAD")
	if got != want {
		t.Fatalf("remote HEAD = %s, want %s", got, want)
	}
}
