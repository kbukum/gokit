package embedded_test

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kbukum/gokit/util"
)

func TestFixtureCommitsDoNotStartMaintenance(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	tracePath := filepath.Join(t.TempDir(), "git-trace.json")
	writeFile(t, dir, "next.txt", "next")
	runGit(t, dir, "add", "next.txt")
	runGitEnv(t, dir, map[string]string{"GIT_TRACE2_EVENT": tracePath}, "commit", "-m", "next commit")

	trace, err := os.Open(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := trace.Close(); err != nil {
			t.Error(err)
		}
	}()
	decoder := json.NewDecoder(trace)
	for {
		var event struct {
			Event string   `json:"event"`
			Args  []string `json:"argv"`
		}
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if event.Event == "child_start" && slices.Contains(event.Args, "--auto") &&
			(slices.Contains(event.Args, "maintenance") || slices.Contains(event.Args, "gc")) {
			t.Errorf("fixture commit started automatic maintenance: %v", event.Args)
		}
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	// Detached maintenance must not outlive the fixture and race TempDir cleanup.
	runGit(t, dir, "config", "maintenance.auto", "false")
	runGit(t, dir, "config", "gc.auto", "0")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test User")
	writeFile(t, dir, "README.md", "# test repo")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial commit")
	return dir
}

func writeFile(t *testing.T, repoDir, path, content string) {
	t.Helper()
	full := filepath.Join(repoDir, path)
	if err := util.WriteFile(full, []byte(content)); err != nil {
		t.Fatal(err)
	}
}

func commitFile(t *testing.T, repoDir, path, content, message string) {
	t.Helper()
	writeFile(t, repoDir, path, content)
	runGit(t, repoDir, "add", path)
	runGit(t, repoDir, "commit", "-m", message)
}

func commitFileAt(t *testing.T, repoDir, path, content, message string, when time.Time) {
	t.Helper()
	writeFile(t, repoDir, path, content)
	runGit(t, repoDir, "add", path)
	ts := when.Format(time.RFC3339)
	runGitEnv(t, repoDir, map[string]string{"GIT_AUTHOR_DATE": ts, "GIT_COMMITTER_DATE": ts}, "commit", "-m", message)
}

func createBranch(t *testing.T, repoDir, name string) { t.Helper(); runGit(t, repoDir, "branch", name) }

func checkoutBranch(t *testing.T, repoDir, name string) {
	t.Helper()
	runGit(t, repoDir, "checkout", name)
}

func checkoutNewBranch(t *testing.T, repoDir, name string) {
	t.Helper()
	runGit(t, repoDir, "checkout", "-b", name)
}
func createTag(t *testing.T, repoDir, name string) { t.Helper(); runGit(t, repoDir, "tag", name) }
func makeDirty(t *testing.T, repoDir, path string) {
	t.Helper()
	writeFile(t, repoDir, path, "dirty content\n")
}

func makeUntracked(t *testing.T, repoDir, path string) {
	t.Helper()
	writeFile(t, repoDir, path, "untracked\n")
}

func createRemote(t *testing.T, repoDir string) string {
	t.Helper()
	remoteDir := t.TempDir()
	runGit(t, remoteDir, "init", "--bare")
	branch := stringTrimSpace(runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD"))
	runGit(t, repoDir, "--git-dir="+remoteDir, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runGit(t, repoDir, "remote", "add", "origin", remoteDir)
	runGit(t, repoDir, "push", "-u", "origin", "HEAD:refs/heads/"+branch)
	runGit(t, repoDir, "fetch", "origin")
	return remoteDir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s\n%s", args, err, out)
	}
	return string(out)
}

func runGitEnv(t *testing.T, dir string, env map[string]string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s\n%s", args, err, out)
	}
	return string(out)
}

func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	return stringTrimSpace(runGit(t, dir, "rev-parse", rev))
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	return stringTrimSpace(runGit(t, dir, "branch", "--show-current"))
}

func stringTrimSpace(s string) string {
	for s != "" && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	for s != "" && (s[0] == '\n' || s[0] == '\r' || s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}
