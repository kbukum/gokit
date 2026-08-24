package embedded_test

import (
	"testing"
	"time"

	"github.com/kbukum/gokit/git"
	"github.com/kbukum/gokit/git/embedded"
)

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
