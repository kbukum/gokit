package embedded

import (
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

// Backend implements the git interfaces using go-git.
type Backend struct {
	repo *gogit.Repository
	root string
}

// Init creates a new git repository at the given path.
func Init(path string) (*Backend, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, giterr.RepoNotFound(path)
	}
	repo, err := gogit.PlainInit(absPath, false)
	if err != nil {
		return nil, giterr.Internal(err)
	}
	return &Backend{repo: repo, root: absPath}, nil
}

// InitBare creates a new bare git repository at the given path.
func InitBare(path string) (*Backend, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, giterr.RepoNotFound(path)
	}
	repo, err := gogit.PlainInit(absPath, true)
	if err != nil {
		return nil, giterr.Internal(err)
	}
	return &Backend{repo: repo, root: absPath}, nil
}

// Open opens a git repository at the given path.
func Open(path string, _ *model.OpenOptions) (*Backend, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, giterr.RepoNotFound(path)
	}

	repo, err := gogit.PlainOpen(absPath)
	if err != nil {
		return nil, giterr.RepoNotFound(absPath)
	}

	root, err := findRepoRoot(absPath)
	if err != nil {
		return nil, giterr.RepoNotFound(absPath)
	}

	return &Backend{repo: repo, root: root}, nil
}

// Discover finds a git repository by walking up from the given path.
func Discover(path string, _ *model.OpenOptions) (*Backend, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, giterr.RepoNotFound(path)
	}

	repo, err := gogit.PlainOpenWithOptions(absPath, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, giterr.RepoNotFound(absPath)
	}

	root, err := findRepoRoot(absPath)
	if err != nil {
		return nil, giterr.RepoNotFound(absPath)
	}

	return &Backend{repo: repo, root: root}, nil
}

// Clone clones a repository into path.
func Clone(url, path string, cfg *model.OpenOptions) (*Backend, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, giterr.RepoNotFound(path)
	}

	authMethod, err := transportAuthMethod(nil)
	if cfg != nil {
		authMethod, err = transportAuthMethod(cfg.Transport)
	}
	if err != nil {
		return nil, err
	}

	repo, err := gogit.PlainClone(absPath, false, &gogit.CloneOptions{URL: url, Auth: authMethod})
	if err != nil {
		return nil, giterr.Network(err)
	}

	return &Backend{repo: repo, root: absPath}, nil
}

// Root returns the absolute path to the repository root.
func (b *Backend) Root() string { return b.root }

// Head returns the reference that HEAD points to.
func (b *Backend) Head() (model.Reference, error) {
	head, err := b.repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return model.Reference{}, giterr.Internal(err)
	}
	if head.Type() != plumbing.SymbolicReference {
		return model.Reference{}, giterr.DetachedHead()
	}

	ref, err := b.repo.Reference(head.Target(), true)
	if err != nil {
		return model.Reference{}, giterr.Internal(err)
	}

	return referenceFromPlumbing(ref), nil
}

// ResolveRef resolves a ref name to an OID.
func (b *Backend) ResolveRef(refname string) (model.Oid, error) {
	h, err := b.repo.ResolveRevision(plumbing.Revision(refname))
	if err != nil {
		return model.Oid{}, giterr.RefNotFound(refname)
	}
	return oidFromHash(*h), nil
}

// IsDirty reports whether the working tree has uncommitted changes.
func (b *Backend) IsDirty() (bool, error) {
	wt, err := b.repo.Worktree()
	if err != nil {
		return false, giterr.Internal(err)
	}
	status, err := wt.Status()
	if err != nil {
		return false, giterr.Internal(err)
	}
	return !status.IsClean(), nil
}

func findRepoRoot(path string) (string, error) {
	start := path
	info, err := os.Stat(start)
	switch {
	case err == nil && !info.IsDir():
		start = filepath.Dir(start)
	case err != nil && os.IsNotExist(err):
		start = filepath.Dir(start)
	case err != nil:
		return "", err
	}

	for {
		if isGitDir(start) {
			return start, nil
		}
		parent := filepath.Dir(start)
		if parent == start {
			return "", os.ErrNotExist
		}
		start = parent
	}
}

func isGitDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && (info.IsDir() || !info.IsDir())
}

func oidFromHash(h plumbing.Hash) model.Oid {
	var o model.Oid
	copy(o[:], h[:])
	return o
}

func referenceFromPlumbing(ref *plumbing.Reference) model.Reference {
	name := ref.Name()
	return model.Reference{
		Name:     name.String(),
		Target:   oidFromHash(ref.Hash()),
		IsBranch: name.IsBranch(),
		IsTag:    name.IsTag(),
	}
}
