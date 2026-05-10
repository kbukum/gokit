package embedded_test

import (
	"testing"
	"time"

	"github.com/kbukum/gokit/git"
	"github.com/kbukum/gokit/git/embedded"
)

func TestStage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		paths     []string
		wantPaths []string
	}{
		{name: "tracked file", setup: func(t *testing.T, dir string) { makeDirty(t, dir, "README.md") }, paths: []string{"README.md"}, wantPaths: []string{"README.md"}},
		{name: "multiple files", setup: func(t *testing.T, dir string) { makeDirty(t, dir, "README.md"); makeUntracked(t, dir, "notes.txt") }, paths: []string{"README.md", "notes.txt"}, wantPaths: []string{"README.md", "notes.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := initTestRepo(t)
			tc.setup(t, dir)
			repo, err := embedded.Open(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.Stage(tc.paths...); err != nil {
				t.Fatalf("Stage() error: %v", err)
			}
			entries, err := repo.StagedEntries()
			if err != nil {
				t.Fatalf("StagedEntries() error: %v", err)
			}
			assertStatusEntries(t, entries, tc.wantPaths, git.Staged)
		})
	}
}

func TestUnstage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		path      string
		wantState git.EntryState
	}{
		{name: "tracked file", setup: func(t *testing.T, dir string) { makeDirty(t, dir, "README.md") }, path: "README.md", wantState: git.Unstaged},
		{name: "new file", setup: func(t *testing.T, dir string) { makeUntracked(t, dir, "notes.txt") }, path: "notes.txt", wantState: git.Untracked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := initTestRepo(t)
			tc.setup(t, dir)
			repo, err := embedded.Open(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.Stage(tc.path); err != nil {
				t.Fatalf("Stage() error: %v", err)
			}
			if err := repo.Unstage(tc.path); err != nil {
				t.Fatalf("Unstage() error: %v", err)
			}
			staged, err := repo.StagedEntries()
			if err != nil {
				t.Fatalf("StagedEntries() error: %v", err)
			}
			if len(staged) != 0 {
				t.Fatalf("StagedEntries() returned %d entries, want 0", len(staged))
			}
			status, err := repo.Status()
			if err != nil {
				t.Fatalf("Status() error: %v", err)
			}
			assertStatusEntry(t, status, tc.path, tc.wantState)
		})
	}
}

func TestStagedEntries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		stage     []string
		wantPaths []string
	}{
		{name: "only staged entries", setup: func(t *testing.T, dir string) { makeDirty(t, dir, "README.md"); makeUntracked(t, dir, "notes.txt") }, stage: []string{"README.md"}, wantPaths: []string{"README.md"}},
		{name: "sorted output", setup: func(t *testing.T, dir string) {
			makeUntracked(t, dir, "z-last.txt")
			makeUntracked(t, dir, "a-first.txt")
			makeDirty(t, dir, "README.md")
		}, stage: []string{"z-last.txt", "a-first.txt"}, wantPaths: []string{"a-first.txt", "z-last.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := initTestRepo(t)
			tc.setup(t, dir)
			repo, err := embedded.Open(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.Stage(tc.stage...); err != nil {
				t.Fatalf("Stage() error: %v", err)
			}
			entries, err := repo.StagedEntries()
			if err != nil {
				t.Fatalf("StagedEntries() error: %v", err)
			}
			assertStatusEntries(t, entries, tc.wantPaths, git.Staged)
		})
	}
}

func TestCommit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		message      string
		setup        func(t *testing.T, dir string)
		opts         []git.CommitOption
		wantAuthor   string
		wantAuthorTS string
	}{
		{name: "message only", message: "add notes", setup: func(t *testing.T, dir string) { makeUntracked(t, dir, "notes.txt") }, wantAuthor: "Test User <test@test.com>"},
		{name: "with author option", message: "authored commit", setup: func(t *testing.T, dir string) { makeUntracked(t, dir, "author.txt") }, opts: []git.CommitOption{git.WithCommitAuthor(git.Signature{Name: "Custom Author", Email: "author@example.com", When: time.Date(2024, time.January, 2, 3, 4, 5, 0, time.FixedZone("+0200", 2*60*60))})}, wantAuthor: "Custom Author <author@example.com>", wantAuthorTS: "2024-01-02T03:04:05+02:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := initTestRepo(t)
			tc.setup(t, dir)
			repo, err := embedded.Open(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			status, err := repo.Status()
			if err != nil {
				t.Fatalf("Status() error: %v", err)
			}
			if len(status) != 1 {
				t.Fatalf("Status() returned %d entries, want 1", len(status))
			}
			if err := repo.Stage(status[0].Path); err != nil {
				t.Fatalf("Stage() error: %v", err)
			}
			oid, err := repo.Commit(tc.message, tc.opts...)
			if err != nil {
				t.Fatalf("Commit() error: %v", err)
			}
			if oid.IsZero() {
				t.Fatal("Commit() returned zero OID")
			}
			head := revParse(t, dir, "HEAD")
			if oid.String() != head {
				t.Fatalf("Commit() OID = %s, want HEAD %s", oid.String(), head)
			}
			if got := stringTrimSpace(runGit(t, dir, "log", "-1", "--format=%s")); got != tc.message {
				t.Fatalf("message = %q, want %q", got, tc.message)
			}
			if got := stringTrimSpace(runGit(t, dir, "log", "-1", "--format=%an <%ae>")); got != tc.wantAuthor {
				t.Fatalf("author = %q, want %q", got, tc.wantAuthor)
			}
			if tc.wantAuthorTS != "" {
				if got := stringTrimSpace(runGit(t, dir, "log", "-1", "--format=%aI")); got != tc.wantAuthorTS {
					t.Fatalf("author timestamp = %q, want %q", got, tc.wantAuthorTS)
				}
			}
		})
	}
}

func assertStatusEntries(t *testing.T, entries []git.StatusEntry, wantPaths []string, wantState git.EntryState) {
	t.Helper()
	if len(entries) != len(wantPaths) {
		t.Fatalf("status entry count = %d, want %d", len(entries), len(wantPaths))
	}
	for i, wantPath := range wantPaths {
		if entries[i].Path != wantPath {
			t.Fatalf("entries[%d].Path = %q, want %q", i, entries[i].Path, wantPath)
		}
		if entries[i].State != wantState {
			t.Fatalf("entries[%d].State = %v, want %v", i, entries[i].State, wantState)
		}
	}
}

func assertStatusEntry(t *testing.T, entries []git.StatusEntry, wantPath string, wantState git.EntryState) {
	t.Helper()
	for _, entry := range entries {
		if entry.Path == wantPath {
			if entry.State != wantState {
				t.Fatalf("status %q state = %v, want %v", wantPath, entry.State, wantState)
			}
			return
		}
	}
	t.Fatalf("status missing %q", wantPath)
}
