package embedded_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kbukum/gokit/util"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
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
	runGit(t, repoDir, "remote", "add", "origin", remoteDir)
	runGit(t, repoDir, "push", "-u", "origin", "HEAD:refs/heads/main")
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
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	for len(s) > 0 && (s[0] == '\n' || s[0] == '\r' || s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}
