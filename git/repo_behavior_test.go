package git_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/git"
)

func TestRepoEmbeddedOperations(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	head, err := repo.ResolveRef("HEAD")
	if err != nil {
		t.Fatalf("ResolveRef() error: %v", err)
	}
	if head.IsZero() {
		t.Fatal("ResolveRef() returned zero OID")
	}

	if dirty, err := repo.IsDirty(); err != nil || dirty {
		t.Fatalf("IsDirty() = %v, %v; want clean", dirty, err)
	}

	createTag(t, dir, "base")
	commitFile(t, dir, "docs/guide.md", "guide\n", "add guide")
	commitFile(t, dir, "README.md", "# updated\n", "update readme")

	diff, err := repo.Diff("base", "HEAD")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if len(diff) == 0 {
		t.Fatal("Diff() returned no entries")
	}
	stats, err := repo.DiffStats("base", "HEAD")
	if err != nil {
		t.Fatalf("DiffStats() error: %v", err)
	}
	if stats.FilesChanged == 0 || stats.Additions == 0 {
		t.Fatalf("DiffStats() = %+v, want changed files and additions", stats)
	}
	if content, err := repo.FileAt("HEAD", "docs/guide.md"); err != nil || string(content) != "guide\n" {
		t.Fatalf("FileAt() = %q, %v; want guide", content, err)
	}
	if tree, err := repo.TreeHash("HEAD", "docs"); err != nil || tree.IsZero() {
		t.Fatalf("TreeHash() = %s, %v; want non-zero", tree.String(), err)
	}
	entries, err := repo.ListEntries("HEAD", "docs")
	if err != nil {
		t.Fatalf("ListEntries() error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "guide.md" {
		t.Fatalf("ListEntries() = %+v, want guide.md", entries)
	}
	logs, err := repo.Log(git.LogOptions{MaxCount: 2, PathFilter: "README.md"})
	if err != nil {
		t.Fatalf("Log() error: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("Log() len = %d, want 2", len(logs))
	}
	mergeBase, err := repo.MergeBase("base", "HEAD")
	if err != nil {
		t.Fatalf("MergeBase() error: %v", err)
	}
	if mergeBase.String() != revParse(t, dir, "base") {
		t.Fatalf("MergeBase() = %s, want base", mergeBase)
	}
	ancestor, err := repo.IsAncestor("base", "HEAD")
	if err != nil || !ancestor {
		t.Fatalf("IsAncestor() = %v, %v; want true", ancestor, err)
	}
	blame, err := repo.Blame("HEAD", "README.md", git.WithLineRange(1, 1))
	if err != nil {
		t.Fatalf("Blame() error: %v", err)
	}
	if len(blame) != 1 || blame[0].Line != 1 {
		t.Fatalf("Blame() = %+v, want first line", blame)
	}

	makeUntracked(t, dir, "notes.txt")
	if err := repo.Stage("notes.txt"); err != nil {
		t.Fatalf("Stage() error: %v", err)
	}
	staged, err := repo.StagedEntries()
	if err != nil {
		t.Fatalf("StagedEntries() error: %v", err)
	}
	if len(staged) != 1 || staged[0].Path != "notes.txt" {
		t.Fatalf("StagedEntries() = %+v, want notes.txt", staged)
	}
	if err := repo.Unstage("notes.txt"); err != nil {
		t.Fatalf("Unstage() error: %v", err)
	}
	status, err := repo.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	foundUntracked := false
	for _, entry := range status {
		foundUntracked = foundUntracked || entry.Path == "notes.txt" && entry.State == git.Untracked
	}
	if !foundUntracked {
		t.Fatalf("Status() = %+v, want untracked notes.txt", status)
	}
	if err := repo.Stage("notes.txt"); err != nil {
		t.Fatalf("Stage(notes) error: %v", err)
	}
	when := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	oid, err := repo.Commit("add notes", git.WithCommitAuthor(git.Signature{Name: "Test User", Email: "test@test.com", When: when}))
	if err != nil {
		t.Fatalf("Commit() error: %v", err)
	}
	if oid.IsZero() {
		t.Fatal("Commit() returned zero OID")
	}
}

func TestRepoRefRemoteAndConfigOperations(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	remoteDir := createRemote(t, dir)
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.CreateBranch("release", "HEAD"); err != nil {
		t.Fatalf("CreateBranch() error: %v", err)
	}
	branches, err := repo.ListBranches(git.AllBranches)
	if err != nil {
		t.Fatalf("ListBranches() error: %v", err)
	}
	if !hasBranch(branches, "release") {
		t.Fatalf("ListBranches() missing release: %+v", branches)
	}
	if err := repo.DeleteBranch("release"); err != nil {
		t.Fatalf("DeleteBranch() error: %v", err)
	}

	if err := repo.CreateTag("v9.0.0", "HEAD", "release"); err != nil {
		t.Fatalf("CreateTag() error: %v", err)
	}
	tags, err := repo.ListTags()
	if err != nil {
		t.Fatalf("ListTags() error: %v", err)
	}
	if !hasTag(tags, "v9.0.0") {
		t.Fatalf("ListTags() missing v9.0.0: %+v", tags)
	}
	if err := repo.DeleteTag("v9.0.0"); err != nil {
		t.Fatalf("DeleteTag() error: %v", err)
	}

	remotes, err := repo.ListRemotes()
	if err != nil {
		t.Fatalf("ListRemotes() error: %v", err)
	}
	if len(remotes) != 1 || remotes[0].Name != "origin" || remotes[0].URL != remoteDir {
		t.Fatalf("ListRemotes() = %+v, want origin %q", remotes, remoteDir)
	}
	branch := currentBranch(t, dir)
	upstream, err := repo.TrackingBranch(branch)
	if err != nil {
		t.Fatalf("TrackingBranch() error: %v", err)
	}
	if upstream != "origin/"+branch {
		t.Fatalf("TrackingBranch() = %q, want origin/%s", upstream, branch)
	}

	if err := repo.ConfigSet("tool.test", "value"); err != nil {
		t.Fatalf("ConfigSet() error: %v", err)
	}
	if got, err := repo.ConfigGet("tool.test"); err != nil || got != "value" {
		t.Fatalf("ConfigGet() = %q, %v; want value", got, err)
	}
	values, err := repo.ConfigGetAll("remote.origin.fetch")
	if err != nil {
		t.Fatalf("ConfigGetAll() error: %v", err)
	}
	if len(values) == 0 {
		t.Fatal("ConfigGetAll() returned no fetch refspecs")
	}

	commitFile(t, dir, "local.txt", "local\n", "local change")
	if err := repo.Push("origin", git.WithPushRefspecs("refs/heads/"+branch+":refs/heads/"+branch)); err != nil {
		t.Fatalf("Push() error: %v", err)
	}
	if err := repo.Fetch("origin", git.WithFetchRefspecs("+refs/heads/"+branch+":refs/remotes/origin/"+branch)); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
}

func TestRepoCLIOperations(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "tag", "-a", "v1.0.0", "-m", "v1.0.0")

	if got, err := repo.Describe("HEAD"); err != nil || got != "v1.0.0" {
		t.Fatalf("Describe() = %q, %v; want v1.0.0", got, err)
	}
	if oid, err := repo.RevParse("HEAD"); err != nil || oid.IsZero() {
		t.Fatalf("RevParse() = %s, %v; want non-zero", oid.String(), err)
	}
	matches, err := repo.Grep("test repo", "README.md")
	if err != nil {
		t.Fatalf("Grep() error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("Grep() len = %d, want 1", len(matches))
	}
	if out, err := repo.Show("HEAD:README.md"); err != nil || !strings.Contains(string(out), "test repo") {
		t.Fatalf("Show() = %q, %v; want README", out, err)
	}

	writeFile(t, dir, "scratch.txt", "scratch\n")
	cleaned, err := repo.Clean()
	if err != nil {
		t.Fatalf("Clean(dry-run) error: %v", err)
	}
	if len(cleaned) != 1 || cleaned[0] != "scratch.txt" {
		t.Fatalf("Clean(dry-run) = %v, want scratch.txt", cleaned)
	}
	if _, err := os.Stat(filepath.Join(dir, "scratch.txt")); err != nil {
		t.Fatalf("dry-run clean removed file: %v", err)
	}
	cleaned, err = repo.Clean(git.WithCleanForce(true))
	if err != nil {
		t.Fatalf("Clean(force) error: %v", err)
	}
	if len(cleaned) != 1 || cleaned[0] != "scratch.txt" {
		t.Fatalf("Clean(force) = %v, want scratch.txt", cleaned)
	}

	if err := repo.GC(); err != nil {
		t.Fatalf("GC() error: %v", err)
	}
	if err := repo.Prune(); err != nil {
		t.Fatalf("Prune() error: %v", err)
	}
	if err := repo.Fsck(); err != nil {
		t.Fatalf("Fsck() error: %v", err)
	}

	commitFile(t, dir, "reset.txt", "one\n", "add reset")
	first := revParse(t, dir, "HEAD")
	commitFile(t, dir, "reset.txt", "two\n", "update reset")
	if err := repo.Reset(first, git.ResetMixed, "reset.txt"); err != nil {
		t.Fatalf("Reset(path) error: %v", err)
	}
	if got := statusShort(t, dir, "reset.txt"); got == "" {
		t.Fatal("Reset(path) left no working tree change")
	}
	if err := repo.Checkout("HEAD", "reset.txt"); err != nil {
		t.Fatalf("Checkout(path) error: %v", err)
	}

	writeFile(t, dir, "README.md", "stashed\n")
	if err := repo.StashPush("save readme"); err != nil {
		t.Fatalf("StashPush() error: %v", err)
	}
	stashes, err := repo.StashList()
	if err != nil {
		t.Fatalf("StashList() error: %v", err)
	}
	if len(stashes) != 1 || stashes[0].Message != "save readme" {
		t.Fatalf("StashList() = %+v, want save readme", stashes)
	}
	if err := repo.StashPop(0); err != nil {
		t.Fatalf("StashPop() error: %v", err)
	}
}

func TestRepoCLISequencingOperations(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	baseBranch := currentBranch(t, dir)

	checkoutNewBranch(t, dir, "feature")
	commitFile(t, dir, "feature.txt", "feature\n", "feature change")
	checkoutBranch(t, dir, baseBranch)
	commitFile(t, dir, "main.txt", "main\n", "main change")

	if err := repo.Merge("feature"); err != nil {
		t.Fatalf("Merge() error: %v", err)
	}
	if err := repo.MergeAbort(); err == nil {
		t.Fatal("MergeAbort() expected error outside merge")
	}

	checkoutBranch(t, dir, "feature")
	if err := repo.Rebase(baseBranch); err != nil {
		t.Fatalf("Rebase() error: %v", err)
	}
	if err := repo.RebaseContinue(); err == nil {
		t.Fatal("RebaseContinue() expected error outside rebase")
	}
	if err := repo.RebaseAbort(); err == nil {
		t.Fatal("RebaseAbort() expected error outside rebase")
	}

	checkoutBranch(t, dir, "feature")
	commitFile(t, dir, "cherry.txt", "cherry\n", "cherry change")
	cherryCommit := revParse(t, dir, "HEAD")
	checkoutBranch(t, dir, baseBranch)
	if err := repo.CherryPick(cherryCommit); err != nil {
		t.Fatalf("CherryPick() error: %v", err)
	}
	if err := repo.CherryPickContinue(); err == nil {
		t.Fatal("CherryPickContinue() expected error outside cherry-pick")
	}
	if err := repo.CherryPickAbort(); err == nil {
		t.Fatal("CherryPickAbort() expected error outside cherry-pick")
	}
	if err := repo.Checkout("feature"); err != nil {
		t.Fatalf("Checkout(branch) error: %v", err)
	}
}

func hasBranch(branches []git.Branch, name string) bool {
	for _, branch := range branches {
		if branch.Name == name {
			return true
		}
	}
	return false
}

func hasTag(tags []git.Tag, name string) bool {
	for _, tag := range tags {
		if tag.Name == name {
			return true
		}
	}
	return false
}
