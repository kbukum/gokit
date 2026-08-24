package embedded_test

import (
	stderrors "errors"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/git"
	"github.com/kbukum/gokit/git/embedded"
)

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

func TestFileAtBounded(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	const content = "hello bounded world"
	commitFile(t, dir, "bounded.txt", content, "add bounded")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.FileAtBounded("HEAD", "bounded.txt", int64(len(content)))
	if err != nil {
		t.Fatalf("FileAtBounded() error: %v", err)
	}
	if string(got) != content {
		t.Fatalf("FileAtBounded() = %q, want %q", got, content)
	}

	_, err = repo.FileAtBounded("HEAD", "bounded.txt", int64(len(content)-1))
	if err == nil {
		t.Fatal("FileAtBounded(oversized) expected error")
	}
	var appErr *goerrors.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("error = %T %v, want AppError", err, err)
	}
	if appErr.Details["gokit_git_error"] != "file_too_large" {
		t.Fatalf("gokit_git_error = %#v, want file_too_large", appErr.Details["gokit_git_error"])
	}
	if appErr.Details["size"] != int64(len(content)) {
		t.Fatalf("size = %#v, want %d", appErr.Details["size"], len(content))
	}
}

func TestFileAtNotFound(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.FileAt("HEAD", "nonexistent.txt"); err == nil {
		t.Fatal("FileAt() expected error")
	}
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
