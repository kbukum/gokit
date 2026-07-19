package embedded_test

import (
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/git"
	"github.com/kbukum/gokit/git/embedded"
)

func TestListBranches(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	mainBranch := currentBranch(t, dir)
	createBranch(t, dir, "feature")
	createRemote(t, dir)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		filter    git.BranchFilter
		wantNames []string
	}{
		{name: "local branches", filter: git.LocalBranches, wantNames: []string{"feature", mainBranch}},
		{name: "remote branches", filter: git.RemoteBranches, wantNames: []string{"origin/" + mainBranch}},
		{name: "all branches", filter: git.AllBranches, wantNames: []string{"feature", mainBranch, "origin/" + mainBranch}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			branches, err := repo.ListBranches(tc.filter)
			if err != nil {
				t.Fatalf("ListBranches() error: %v", err)
			}
			names := make(map[string]git.Branch, len(branches))
			for _, branch := range branches {
				names[branch.Name] = branch
			}
			for _, want := range tc.wantNames {
				if _, ok := names[want]; !ok {
					t.Errorf("ListBranches() missing %q", want)
				}
			}
			if tc.filter == git.LocalBranches {
				if branch := names[mainBranch]; branch.Upstream != "origin/"+mainBranch {
					t.Errorf("upstream = %q, want origin/%s", branch.Upstream, mainBranch)
				}
			}
		})
	}
}

func TestBranchCRUD(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.CreateBranch("release", "HEAD"); err != nil {
		t.Fatalf("CreateBranch() error: %v", err)
	}
	branches, err := repo.ListBranches(git.LocalBranches)
	if err != nil {
		t.Fatalf("ListBranches() error: %v", err)
	}
	found := false
	for _, branch := range branches {
		if branch.Name == "release" {
			found = true
		}
	}
	if !found {
		t.Fatal("CreateBranch() branch not listed")
	}
	if err := repo.DeleteBranch("release"); err != nil {
		t.Fatalf("DeleteBranch() error: %v", err)
	}
	branches, err = repo.ListBranches(git.LocalBranches)
	if err != nil {
		t.Fatalf("ListBranches() after delete error: %v", err)
	}
	for _, branch := range branches {
		if branch.Name == "release" {
			t.Fatal("DeleteBranch() did not remove release branch")
		}
	}
}

func TestBranchErrors(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	current := currentBranch(t, dir)

	if err := repo.CreateBranch("bad name", "HEAD"); err == nil {
		t.Fatal("CreateBranch(invalid) expected error")
	}
	if err := repo.CreateBranch("missing-target", "missing"); err == nil {
		t.Fatal("CreateBranch(missing target) expected error")
	}
	if err := repo.CreateBranch("release", "HEAD"); err != nil {
		t.Fatalf("CreateBranch() error: %v", err)
	}
	if err := repo.CreateBranch("release", "HEAD"); err == nil {
		t.Fatal("CreateBranch(duplicate) expected error")
	}
	if err := repo.DeleteBranch("missing"); err == nil {
		t.Fatal("DeleteBranch(missing) expected error")
	}
	if err := repo.DeleteBranch(current); err == nil {
		t.Fatal("DeleteBranch(checked out) expected error")
	}
}

func TestTagCRUD(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createTag(t, dir, "v1.0.0")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.CreateTag("v2.0.0", "HEAD", "release v2"); err != nil {
		t.Fatalf("CreateTag() error: %v", err)
	}
	tags, err := repo.ListTags()
	if err != nil {
		t.Fatalf("ListTags() error: %v", err)
	}
	foundLightweight := false
	foundAnnotated := false
	for _, tag := range tags {
		switch tag.Name {
		case "v1.0.0":
			foundLightweight = true
			if tag.Message != "" {
				t.Errorf("lightweight tag message = %q, want empty", tag.Message)
			}
		case "v2.0.0":
			foundAnnotated = true
			if tag.Message != "release v2\n" {
				t.Errorf("annotated tag message = %q", tag.Message)
			}
			if tag.Tagger == nil {
				t.Error("annotated tag missing tagger")
			}
		}
	}
	if !foundLightweight || !foundAnnotated {
		t.Fatal("ListTags() missing expected tags")
	}
	if err := repo.DeleteTag("v1.0.0"); err != nil {
		t.Fatalf("DeleteTag() error: %v", err)
	}
	tags, err = repo.ListTags()
	if err != nil {
		t.Fatalf("ListTags() after delete error: %v", err)
	}
	for _, tag := range tags {
		if tag.Name == "v1.0.0" {
			t.Fatal("DeleteTag() did not remove v1.0.0")
		}
	}
}

func TestTagErrors(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTag("v1.0.0", "missing", ""); err == nil {
		t.Fatal("CreateTag(missing target) expected error")
	}
	if err := repo.CreateTag("v1.0.0", "HEAD", ""); err != nil {
		t.Fatalf("CreateTag() error: %v", err)
	}
	if err := repo.CreateTag("v1.0.0", "HEAD", ""); err == nil {
		t.Fatal("CreateTag(duplicate) expected error")
	}
	if err := repo.DeleteTag("missing"); err == nil {
		t.Fatal("DeleteTag(missing) expected error")
	}
}

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

func TestTrackingBranch(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createRemote(t, dir)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	branch := currentBranch(t, dir)
	got, err := repo.TrackingBranch(branch)
	if err != nil {
		t.Fatalf("TrackingBranch() error: %v", err)
	}
	if got != "origin/"+branch {
		t.Fatalf("TrackingBranch() = %q, want origin/%s", got, branch)
	}
}

func TestTrackingBranchMissingAndUnconfigured(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	createBranch(t, dir, "local-only")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.TrackingBranch("local-only")
	if err != nil {
		t.Fatalf("TrackingBranch(local-only) error: %v", err)
	}
	if got != "" {
		t.Fatalf("TrackingBranch(local-only) = %q, want empty", got)
	}
	if _, err := repo.TrackingBranch("missing"); err == nil {
		t.Fatal("TrackingBranch(missing) expected error")
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

	if err := repo.Fetch("origin", git.WithFetchPrune(true)); err != nil {
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
	if err := repo.Fetch("missing"); err == nil {
		t.Fatal("Fetch(missing) expected error")
	}
	if err := repo.Push("missing"); err == nil {
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
	if err := repo.Push("origin", git.WithPushRefspecs(refspec)); err != nil {
		t.Fatalf("Push() error: %v", err)
	}
	got := stringTrimSpace(runGit(t, dir, "-c", "safe.bareRepository=all", "--git-dir", remoteDir, "rev-parse", "refs/heads/"+mainBranch))
	want := revParse(t, dir, "HEAD")
	if got != want {
		t.Fatalf("remote HEAD = %s, want %s", got, want)
	}
}

func TestConfigGetAndGetAll(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createRemote(t, dir)
	runGit(t, dir, "config", "--add", "remote.origin.fetch", "+refs/tags/*:refs/tags/*")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.ConfigGet("remote.origin.fetch")
	if err != nil {
		t.Fatalf("ConfigGet() error: %v", err)
	}
	if got != "+refs/tags/*:refs/tags/*" {
		t.Fatalf("ConfigGet() = %q", got)
	}
	gotAll, err := repo.ConfigGetAll("remote.origin.fetch")
	if err != nil {
		t.Fatalf("ConfigGetAll() error: %v", err)
	}
	if len(gotAll) != 2 {
		t.Fatalf("ConfigGetAll() returned %d values, want 2", len(gotAll))
	}
}

func TestConfigErrors(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"missing", "remote..url"} {
		if _, err := repo.ConfigGet(key); err == nil {
			t.Fatalf("ConfigGet(%q) expected error", key)
		}
		if _, err := repo.ConfigGetAll(key); err == nil {
			t.Fatalf("ConfigGetAll(%q) expected error", key)
		}
		if err := repo.ConfigSet(key, "value"); err == nil {
			t.Fatalf("ConfigSet(%q) expected error", key)
		}
	}
	if _, err := repo.ConfigGet("tool.missing"); err == nil {
		t.Fatal("ConfigGet(missing value) expected error")
	}
}

func TestConfigSet(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ConfigSet("tooling.editor", "vim"); err != nil {
		t.Fatalf("ConfigSet() error: %v", err)
	}
	got, err := repo.ConfigGet("tooling.editor")
	if err != nil {
		t.Fatalf("ConfigGet() error: %v", err)
	}
	if got != "vim" {
		t.Fatalf("ConfigGet() = %q, want vim", got)
	}
	reopened, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = reopened.ConfigGet("tooling.editor")
	if err != nil {
		t.Fatalf("ConfigGet() after reopen error: %v", err)
	}
	if got != "vim" {
		t.Fatalf("ConfigGet() after reopen = %q, want vim", got)
	}
}
