package cli

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/git/internal/model"
	"github.com/kbukum/gokit/process"
	"github.com/kbukum/gokit/util"
)

func TestExec(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	backend := New(dir, nil)

	out, err := backend.Exec("rev-parse", "--is-inside-work-tree")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "true" {
		t.Fatalf("Exec() = %q, want %q", got, "true")
	}
}

func TestExecReportsGitFailure(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	backend := New(dir, nil)

	if _, err := backend.Exec("show", "does-not-exist"); err == nil {
		t.Fatal("Exec() expected error")
	} else if !strings.Contains(err.Error(), "unknown revision or path not in the working tree") {
		t.Fatalf("Exec() error = %v, want stderr from git", err)
	}
}

func TestInspectorOperations(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	backend := New(dir, nil)
	runGit(t, dir, "tag", "-a", "v1.0.0", "-m", "v1.0.0")

	t.Run("RevParse", func(t *testing.T) {
		t.Parallel()

		got, err := backend.RevParse("HEAD")
		if err != nil {
			t.Fatalf("RevParse() error: %v", err)
		}
		want := mustParseOID(t, strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD")))
		if got != want {
			t.Fatalf("RevParse() = %s, want %s", got.String(), want.String())
		}
	})

	t.Run("Describe", func(t *testing.T) {
		t.Parallel()

		got, err := backend.Describe("HEAD")
		if err != nil {
			t.Fatalf("Describe() error: %v", err)
		}
		if got != "v1.0.0" {
			t.Fatalf("Describe() = %q, want %q", got, "v1.0.0")
		}
	})

	t.Run("Grep", func(t *testing.T) {
		t.Parallel()

		matches, err := backend.Grep("test repo", "README.md")
		if err != nil {
			t.Fatalf("Grep() error: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("Grep() len = %d, want 1", len(matches))
		}
		if matches[0].Path != "README.md" || matches[0].Line != 1 || matches[0].Content != "# test repo" {
			t.Fatalf("Grep() match = %+v", matches[0])
		}
	})

	t.Run("GrepNoMatches", func(t *testing.T) {
		t.Parallel()

		matches, err := backend.Grep("missing-pattern", "README.md")
		if err != nil {
			t.Fatalf("Grep() error: %v", err)
		}
		if len(matches) != 0 {
			t.Fatalf("Grep() len = %d, want 0", len(matches))
		}
	})

	t.Run("Show", func(t *testing.T) {
		t.Parallel()

		out, err := backend.Show("HEAD:README.md")
		if err != nil {
			t.Fatalf("Show() error: %v", err)
		}
		if got := string(out); got != "# test repo\n" {
			t.Fatalf("Show() = %q, want %q", got, "# test repo\n")
		}
	})
}

func TestMerge(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	backend := New(dir, nil)
	baseBranch := currentBranch(t, dir)

	runGit(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "feature.txt", "feature\n")
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feature change")

	runGit(t, dir, "checkout", baseBranch)
	writeFile(t, dir, "main.txt", "main\n")
	runGit(t, dir, "add", "main.txt")
	runGit(t, dir, "commit", "-m", "main change")

	if err := backend.Merge("feature"); err != nil {
		t.Fatalf("Merge() error: %v", err)
	}
	if got := currentBranch(t, dir); got != baseBranch {
		t.Fatalf("current branch = %q, want %q", got, baseBranch)
	}
	if _, err := os.Stat(filepath.Join(dir, "feature.txt")); err != nil {
		t.Fatalf("feature.txt missing after merge: %v", err)
	}
	if _, err := backend.Exec("rev-parse", "HEAD^2"); err != nil {
		t.Fatalf("expected merge commit, got error: %v", err)
	}
}

func TestRebase(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	backend := New(dir, nil)
	baseBranch := currentBranch(t, dir)

	runGit(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "feature.txt", "feature\n")
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feature change")

	runGit(t, dir, "checkout", baseBranch)
	writeFile(t, dir, "main.txt", "main\n")
	runGit(t, dir, "add", "main.txt")
	runGit(t, dir, "commit", "-m", "main change")

	runGit(t, dir, "checkout", "feature")
	if err := backend.Rebase(baseBranch); err != nil {
		t.Fatalf("Rebase() error: %v", err)
	}

	if got := strings.TrimSpace(runGit(t, dir, "log", "--format=%s", "-2")); !strings.Contains(got, "feature change") || !strings.Contains(got, "main change") {
		t.Fatalf("unexpected log after rebase: %q", got)
	}
}

func TestCherryPick(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	backend := New(dir, nil)
	baseBranch := currentBranch(t, dir)

	runGit(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "picked.txt", "picked\n")
	runGit(t, dir, "add", "picked.txt")
	runGit(t, dir, "commit", "-m", "picked change")
	revision := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))

	runGit(t, dir, "checkout", baseBranch)
	if err := backend.CherryPick(revision); err != nil {
		t.Fatalf("CherryPick() error: %v", err)
	}

	if got := readFile(t, dir, "picked.txt"); got != "picked\n" {
		t.Fatalf("picked.txt = %q, want %q", got, "picked\n")
	}
}

func TestResetModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mode       model.ResetMode
		wantStatus string
		wantFile   string
	}{
		{name: "soft", mode: model.ResetSoft, wantStatus: "M  file.txt", wantFile: "two\n"},
		{name: "mixed", mode: model.ResetMixed, wantStatus: " M file.txt", wantFile: "two\n"},
		{name: "hard", mode: model.ResetHard, wantStatus: "", wantFile: "one\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := initTestRepo(t)
			backend := New(dir, nil)

			writeFile(t, dir, "file.txt", "one\n")
			runGit(t, dir, "add", "file.txt")
			runGit(t, dir, "commit", "-m", "add file")
			initial := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))

			writeFile(t, dir, "file.txt", "two\n")
			runGit(t, dir, "add", "file.txt")
			runGit(t, dir, "commit", "-m", "update file")

			if err := backend.Reset("HEAD~1", tt.mode); err != nil {
				t.Fatalf("Reset() error: %v", err)
			}

			if got := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD")); got != initial {
				t.Fatalf("HEAD = %q, want %q", got, initial)
			}
			if got := statusShort(t, dir, "file.txt"); got != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got, tt.wantStatus)
			}
			if got := readFile(t, dir, "file.txt"); got != tt.wantFile {
				t.Fatalf("file.txt = %q, want %q", got, tt.wantFile)
			}
		})
	}
}

func TestCheckout(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	backend := New(dir, nil)
	baseBranch := currentBranch(t, dir)

	runGit(t, dir, "checkout", "-b", "feature")
	runGit(t, dir, "checkout", baseBranch)

	if err := backend.Checkout("feature"); err != nil {
		t.Fatalf("Checkout() error: %v", err)
	}
	if got := currentBranch(t, dir); got != "feature" {
		t.Fatalf("current branch = %q, want %q", got, "feature")
	}
}

func TestStash(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	backend := New(dir, nil)

	writeFile(t, dir, "README.md", "dirty\n")
	if err := backend.StashPush("save work"); err != nil {
		t.Fatalf("StashPush() error: %v", err)
	}
	if got := statusShort(t, dir); got != "" {
		t.Fatalf("status after stash push = %q, want clean tree", got)
	}

	entries, err := backend.StashList()
	if err != nil {
		t.Fatalf("StashList() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("StashList() len = %d, want 1", len(entries))
	}
	if entries[0].Index != 0 || entries[0].Name != "stash@{0}" || entries[0].Message != "save work" || entries[0].Commit.IsZero() {
		t.Fatalf("unexpected stash entry: %+v", entries[0])
	}

	if err := backend.StashPop(0); err != nil {
		t.Fatalf("StashPop() error: %v", err)
	}
	if got := readFile(t, dir, "README.md"); got != "dirty\n" {
		t.Fatalf("README.md = %q, want %q", got, "dirty\n")
	}
	if entries, err = backend.StashList(); err != nil {
		t.Fatalf("StashList() after pop error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("StashList() after pop len = %d, want 0", len(entries))
	}
}

func TestMaintenance(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	backend := New(dir, nil)
	writeFile(t, dir, "junk.txt", "junk\n")

	if err := backend.GC(); err != nil {
		t.Fatalf("GC() error: %v", err)
	}
	if err := backend.Prune(); err != nil {
		t.Fatalf("Prune() error: %v", err)
	}
	if err := backend.Fsck(); err != nil {
		t.Fatalf("Fsck() error: %v", err)
	}
	cleaned, err := backend.Clean(model.WithCleanForce(true))
	if err != nil {
		t.Fatalf("Clean() error: %v", err)
	}
	if len(cleaned) != 1 || cleaned[0] != "junk.txt" {
		t.Fatalf("Clean() = %v, want [junk.txt]", cleaned)
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	writeFile(t, dir, "README.md", "# test repo\n")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial commit")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	result, err := process.Run(context.Background(), process.Command{
		Binary: "git",
		Args:   args,
		Dir:    dir,
	})
	if err != nil {
		t.Fatalf("git %v failed: %v\nstdout:\n%s\nstderr:\n%s", args, err, result.Stdout, result.Stderr)
	}
	return string(result.Stdout)
}

func writeFile(t *testing.T, repoDir, name, content string) {
	t.Helper()

	full := filepath.Join(repoDir, name)
	if err := util.WriteFile(full, []byte(content)); err != nil {
		t.Fatalf("WriteFile(%q): %v", full, err)
	}
}

func readFile(t *testing.T, repoDir, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(repoDir, name))
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", name, err)
	}
	return string(data)
}

func currentBranch(t *testing.T, repoDir string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, repoDir, "branch", "--show-current"))
}

func statusShort(t *testing.T, repoDir string, paths ...string) string {
	t.Helper()

	args := make([]string, 0, 2+len(paths))
	args = append(args, "status", "--short")
	args = append(args, paths...)
	return strings.TrimRight(runGit(t, repoDir, args...), "\r\n")
}

func mustParseOID(t *testing.T, text string) model.Oid {
	t.Helper()

	var oid model.Oid
	decoded, err := hex.DecodeString(text)
	if err != nil {
		t.Fatalf("hex.DecodeString(%q): %v", text, err)
	}
	copy(oid[:], decoded)
	return oid
}
