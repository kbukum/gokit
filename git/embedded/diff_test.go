package embedded_test

import (
	"os"
	"path/filepath"
	"testing"

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
