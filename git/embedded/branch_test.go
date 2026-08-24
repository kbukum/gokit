package embedded_test

import (
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
