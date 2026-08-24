package embedded_test

import (
	"testing"
	"time"

	"github.com/kbukum/gokit/git"
	"github.com/kbukum/gokit/git/embedded"
)

func TestLog(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		setup   func(t *testing.T, dir string) git.LogOptions
		wantLen int
		wantMsg []string
	}{
		{
			name: "max count",
			setup: func(t *testing.T, dir string) git.LogOptions {
				base := time.Now().Add(2 * time.Hour)
				commitFileAt(t, dir, "a.txt", "a", "add a", base)
				commitFileAt(t, dir, "b.txt", "b", "add b", base.Add(time.Minute))
				return git.LogOptions{MaxCount: 2}
			},
			wantLen: 2,
			wantMsg: []string{"add b\n", "add a\n"},
		},
		{
			name: "path filter",
			setup: func(t *testing.T, dir string) git.LogOptions {
				commitFile(t, dir, "docs/guide.md", "guide", "update docs")
				commitFile(t, dir, "src/app.go", "package main", "update app")
				return git.LogOptions{PathFilter: "docs"}
			},
			wantLen: 1,
			wantMsg: []string{"update docs\n"},
		},
		{
			name: "since until",
			setup: func(t *testing.T, dir string) git.LogOptions {
				base := time.Now().Add(2 * time.Hour)
				commitFileAt(t, dir, "one.txt", "one", "commit one", base)
				commitFileAt(t, dir, "two.txt", "two", "commit two", base.Add(time.Hour))
				since := base.Add(-30 * time.Minute)
				until := base.Add(30 * time.Minute)
				return git.LogOptions{Since: &since, Until: &until}
			},
			wantLen: 1,
			wantMsg: []string{"commit one\n"},
		},
		{
			name: "author filter",
			setup: func(t *testing.T, dir string) git.LogOptions {
				commitFileAt(t, dir, "author.txt", "author", "author commit", time.Now().Add(2*time.Hour))
				return git.LogOptions{AuthorFilter: "test@test.com", MaxCount: 1}
			},
			wantLen: 1,
			wantMsg: []string{"author commit\n"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := initTestRepo(t)
			opts := tc.setup(t, dir)
			repo, err := embedded.Open(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			commits, err := repo.Log(opts)
			if err != nil {
				t.Fatalf("Log() error: %v", err)
			}
			if len(commits) != tc.wantLen {
				t.Fatalf("Log() returned %d commits, want %d", len(commits), tc.wantLen)
			}
			for i, want := range tc.wantMsg {
				if commits[i].Message != want {
					t.Errorf("Log()[%d].Message = %q, want %q", i, commits[i].Message, want)
				}
			}
		})
	}
}

func TestMergeBase(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	base := revParse(t, dir, "HEAD")
	mainBranch := currentBranch(t, dir)
	checkoutNewBranch(t, dir, "feature")
	commitFile(t, dir, "feature.txt", "feature", "feature change")
	checkoutBranch(t, dir, mainBranch)
	commitFile(t, dir, "main.txt", "main", "main change")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	oid, err := repo.MergeBase("HEAD", "feature")
	if err != nil {
		t.Fatalf("MergeBase() error: %v", err)
	}
	if oid.String() != base {
		t.Errorf("MergeBase() = %s, want %s", oid.String(), base)
	}
}

func TestMergeBaseAndIsAncestorRefErrors(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeBase("missing", "HEAD"); err == nil {
		t.Fatal("MergeBase(missing left) expected error")
	}
	if _, err := repo.MergeBase("HEAD", "missing"); err == nil {
		t.Fatal("MergeBase(missing right) expected error")
	}
	if _, err := repo.IsAncestor("missing", "HEAD"); err == nil {
		t.Fatal("IsAncestor(missing left) expected error")
	}
	if _, err := repo.IsAncestor("HEAD", "missing"); err == nil {
		t.Fatal("IsAncestor(missing right) expected error")
	}
}

func TestIsAncestor(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	base := revParse(t, dir, "HEAD")
	commitFile(t, dir, "next.txt", "next", "next change")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.IsAncestor(base, "HEAD")
	if err != nil {
		t.Fatalf("IsAncestor() error: %v", err)
	}
	if !got {
		t.Fatal("expected base to be ancestor of HEAD")
	}
	got, err = repo.IsAncestor("HEAD", base)
	if err != nil {
		t.Fatalf("IsAncestor() error: %v", err)
	}
	if got {
		t.Fatal("expected HEAD not to be ancestor of base")
	}
}
