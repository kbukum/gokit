package embedded_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kbukum/gokit/git"
	"github.com/kbukum/gokit/git/embedded"
)

func TestDiff(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createTag(t, dir, "v1")
	commitFile(t, dir, "new.txt", "hello", "add new file")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.Diff("v1", "HEAD")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("Diff() returned no entries")
	}
	found := false
	for _, e := range entries {
		if e.Path == "new.txt" && e.Status == git.FileAdded {
			found = true
		}
	}
	if !found {
		t.Error("Diff() missing expected added file")
	}
}

func TestDiffModified(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createTag(t, dir, "v1")
	commitFile(t, dir, "README.md", "updated content", "update readme")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	func TestDiffDeleted(t *testing.T) {
		t.Parallel()
		dir := initTestRepo(t)
		commitFile(t, dir, "remove.txt", "remove", "add removable")
		createTag(t, dir, "before-delete")
		if err := os.Remove(filepath.Join(dir, "remove.txt")); err != nil {
			t.Fatal(err)
		}
		runGit(t, dir, "add", "remove.txt")
		runGit(t, dir, "commit", "-m", "remove file")
		repo, err := embedded.Open(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := repo.Diff("before-delete", "HEAD")
		if err != nil {
			t.Fatalf("Diff() error: %v", err)
		}
		if len(entries) != 1 || entries[0].Path != "remove.txt" || entries[0].Status != git.FileDeleted {
			t.Fatalf("Diff() = %+v, want deleted remove.txt", entries)
		}
	}
	entries, err := repo.Diff("v1", "HEAD")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Path == "README.md" && e.Status == git.FileModified {
			found = true
		}
	}
	if !found {
		t.Error("Diff() missing expected modified file")
	}
}

func TestDiffStats(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createTag(t, dir, "v1")
	commitFile(t, dir, "a.txt", "line1\nline2\n", "add a")
	commitFile(t, dir, "b.txt", "line1\n", "add b")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := repo.DiffStats("v1", "HEAD")
	if err != nil {
		t.Fatalf("DiffStats() error: %v", err)
	}
	if stats.FilesChanged < 2 {
		t.Errorf("FilesChanged = %d, want >= 2", stats.FilesChanged)
	}
	if stats.Additions < 3 {
		t.Errorf("Additions = %d, want >= 3", stats.Additions)
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	makeUntracked(t, dir, "untracked.txt")
	makeDirty(t, dir, "README.md")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("Status() returned %d entries, want >= 2", len(entries))
	}
	hasUntracked := false
	hasModified := false
	for _, e := range entries {
		if e.Path == "untracked.txt" && e.State == git.Untracked {
			hasUntracked = true
		}
		if e.Path == "README.md" && e.State == git.Unstaged {
			hasModified = true
		}
	}
	if !hasUntracked {
		t.Error("Status() missing untracked file")
	}
	if !hasModified {
		t.Error("Status() missing modified file")
	}
}

func TestStatusClean(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Status() returned %d entries, want 0", len(entries))
	}
}

func TestFileAt(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	commitFile(t, dir, "hello.txt", "hello world", "add hello")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	content, err := repo.FileAt("HEAD", "hello.txt")
	if err != nil {
		t.Fatalf("FileAt() error: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("FileAt() = %q, want %q", content, "hello world")
	}
}

func TestFileAtNotFound(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	func TestTreeReadErrors(t *testing.T) {
		t.Parallel()
		dir := initTestRepo(t)
		repo, err := embedded.Open(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.TreeHash("missing", ""); err == nil {
			t.Fatal("TreeHash(missing ref) expected error")
		}
		if _, err := repo.TreeHash("HEAD", "missing-dir"); err == nil {
			t.Fatal("TreeHash(missing path) expected error")
		}
		if _, err := repo.ListEntries("HEAD", "missing-dir"); err == nil {
			t.Fatal("ListEntries(missing path) expected error")
		}
		if _, err := repo.FileAt("missing", "README.md"); err == nil {
			t.Fatal("FileAt(missing ref) expected error")
		}
	}
	if _, err := repo.FileAt("HEAD", "nonexistent.txt"); err == nil {
		t.Fatal("FileAt() expected error")
	}
}

func TestListEntries(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "a", "add a")
	commitFile(t, dir, "sub/b.txt", "b", "add b")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.ListEntries("HEAD", "")
	if err != nil {
		t.Fatalf("ListEntries() error: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("ListEntries() returned %d entries, want >= 2", len(entries))
	}
	hasBlob := false
	hasTree := false
	for _, e := range entries {
		if e.Kind == git.EntryKindBlob {
			hasBlob = true
		}
		if e.Kind == git.EntryKindTree {
			hasTree = true
		}
	}
	if !hasBlob {
		t.Error("ListEntries() missing blob entry")
	}
	if !hasTree {
		t.Error("ListEntries() missing tree entry")
	}
}

func TestListEntriesSubdir(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	commitFile(t, dir, "sub/file.txt", "content", "add sub/file")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.ListEntries("HEAD", "sub")
	if err != nil {
		t.Fatalf("ListEntries(sub) error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListEntries(sub) returned %d entries, want 1", len(entries))
	}
	if entries[0].Name != "file.txt" {
		t.Errorf("ListEntries(sub)[0].Name = %q, want file.txt", entries[0].Name)
	}
}

func TestTreeHash(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := repo.TreeHash("HEAD", "")
	if err != nil {
		t.Fatalf("TreeHash() error: %v", err)
	}
	if hash.IsZero() {
		t.Error("TreeHash() returned zero OID")
	}
}

func TestTreeHashChanges(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createTag(t, dir, "v1")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	hash1, err := repo.TreeHash("v1", "")
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, dir, "new.txt", "content", "add file")
	repo, err = embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := repo.TreeHash("HEAD", "")
	if err != nil {
		t.Fatal(err)
	}
	if hash1 == hash2 {
		t.Error("TreeHash() should differ after adding a file")
	}
}

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
	oid, err := repo.MergeBase("HEAD", "feature")
	if err != nil {
		t.Fatalf("MergeBase() error: %v", err)
	}
	if oid.String() != base {
		t.Errorf("MergeBase() = %s, want %s", oid.String(), base)
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

func TestBlame(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	base := time.Now().Add(2 * time.Hour)
	commitFileAt(t, dir, "notes.txt", "one\ntwo\nthree\n", "add notes", base)
	first := revParse(t, dir, "HEAD")
	commitFileAt(t, dir, "notes.txt", "ONE\ntwo\nthree\n", "update first line", base.Add(time.Minute))
	second := revParse(t, dir, "HEAD")
	commitFileAt(t, dir, "notes.txt", "ONE\ntwo\nTHREE\n", "update third line", base.Add(2*time.Minute))
	third := revParse(t, dir, "HEAD")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	func TestBlameErrorsAndOutOfRange(t *testing.T) {
		t.Parallel()

		dir := initTestRepo(t)
		commitFile(t, dir, "notes.txt", "one\ntwo\n", "add notes")
		repo, err := embedded.Open(dir, nil)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := repo.Blame("HEAD", "notes.txt", git.WithLineRange(3, 4)); err != nil {
			t.Fatalf("Blame(out of range) error: %v", err)
		}
		lines, err := repo.Blame("HEAD", "notes.txt", git.WithLineRange(0, -1))
		if err == nil {
			t.Fatalf("Blame(invalid range) = %+v, want error", lines)
		}
		if _, err := repo.Blame("HEAD", "missing.txt"); err == nil {
			t.Fatal("Blame(missing file) expected error")
		}
		if _, err := repo.Blame("missing", "notes.txt"); err == nil {
			t.Fatal("Blame(missing ref) expected error")
		}
		if _, err := repo.Blame("HEAD", "notes.txt", git.WithLineRange(2, 1)); err == nil {
			t.Fatal("Blame(start > end) expected error")
		}
	}
	lines, err := repo.Blame("HEAD", "notes.txt")
	if err != nil {
		t.Fatalf("Blame() error: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("Blame() returned %d lines, want 3", len(lines))
	}
	if lines[0].CommitOID.String() != second || lines[1].CommitOID.String() != first || lines[2].CommitOID.String() != third {
		t.Fatalf("unexpected blame commit IDs: %#v", lines)
	}
	ranged, err := repo.Blame("HEAD", "notes.txt", git.WithLineRange(2, 3), git.WithIgnoreWhitespace(true))
	if err != nil {
		t.Fatalf("Blame(line range) error: %v", err)
	}
	if len(ranged) != 2 || ranged[0].Line != 2 || ranged[1].Line != 3 {
		t.Fatalf("unexpected ranged blame: %#v", ranged)
	}
}
