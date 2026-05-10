package testutil

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kbukum/gokit/git"
	"github.com/kbukum/gokit/util"
)

// Builder creates test repositories with specific states.
type Builder struct {
	t    *testing.T
	root string
	repo *git.Repo
}

// NewBuilder creates a builder backed by a temp directory.
func NewBuilder(t *testing.T) *Builder {
	t.Helper()

	root := t.TempDir()
	repo, err := git.Init(root)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	if err := configureRepo(repo); err != nil {
		t.Fatalf("configure repository failed: %v", err)
	}

	return &Builder{t: t, root: root, repo: repo}
}

// WithFile creates or overwrites a file in the working tree.
func (b *Builder) WithFile(path, content string) *Builder {
	b.t.Helper()

	fullPath := filepath.Join(b.root, path)
	if err := util.WriteFile(fullPath, []byte(content)); err != nil {
		b.t.Fatalf("write file %q failed: %v", path, err)
	}
	return b
}

// WithCommit stages all changes and creates a commit.
func (b *Builder) WithCommit(message string) *Builder {
	b.t.Helper()

	if err := b.repo.Stage("."); err != nil {
		b.t.Fatalf("Stage(.) failed: %v", err)
	}

	sig := git.Signature{
		Name:  testUserName,
		Email: testUserEmail,
		When:  time.Now(),
	}
	if _, err := b.repo.Commit(message, git.WithCommitAuthor(sig), git.WithCommitCommitter(sig)); err != nil {
		b.t.Fatalf("Commit(%q) failed: %v", message, err)
	}
	return b
}

// WithBranch creates a new branch at HEAD.
func (b *Builder) WithBranch(name string) *Builder {
	b.t.Helper()

	if err := b.repo.CreateBranch(name, "HEAD"); err != nil {
		b.t.Fatalf("CreateBranch(%q) failed: %v", name, err)
	}
	return b
}

// WithCheckout switches to the named branch.
func (b *Builder) WithCheckout(branch string) *Builder {
	b.t.Helper()

	if err := b.repo.Checkout(branch); err != nil {
		b.t.Fatalf("Checkout(%q) failed: %v", branch, err)
	}
	return b
}

// WithTag creates a tag at HEAD.
func (b *Builder) WithTag(name, message string) *Builder {
	b.t.Helper()

	if err := b.repo.CreateTag(name, "HEAD", message); err != nil {
		b.t.Fatalf("CreateTag(%q) failed: %v", name, err)
	}
	return b
}

// WithRemote adds a remote.
func (b *Builder) WithRemote(name, url string) *Builder {
	b.t.Helper()

	if _, err := b.repo.Exec("remote", "add", name, url); err != nil {
		b.t.Fatalf("remote add %q failed: %v", name, err)
	}
	return b
}

// Repo returns the constructed repository.
func (b *Builder) Repo() *git.Repo { return b.repo }

// Root returns the repository root path.
func (b *Builder) Root() string { return b.root }
