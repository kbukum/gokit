package embedded

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	ggconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

// ListBranches lists repository branches matching filter.
func (b *Backend) ListBranches(filter model.BranchFilter) ([]model.Branch, error) {
	cfg, err := b.repo.Config()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	iter, err := b.repo.References()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	defer iter.Close()

	branches := make([]model.Branch, 0)
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		if !matchesBranchFilter(filter, ref.Name()) {
			return nil
		}
		branch := model.Branch{Name: ref.Name().Short(), Target: oidFromHash(ref.Hash())}
		if ref.Name().IsBranch() {
			branch.Upstream = branchUpstream(cfg, ref.Name().Short())
		}
		branches = append(branches, branch)
		return nil
	})
	if err != nil && !errors.Is(err, storer.ErrStop) {
		return nil, giterr.Internal(err)
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].Name < branches[j].Name })
	return branches, nil
}

// CreateBranch creates a local branch at target.
func (b *Backend) CreateBranch(name, target string) error {
	refName := plumbing.NewBranchReferenceName(name)
	if err := refName.Validate(); err != nil {
		return giterr.InvalidArg("name", fmt.Sprintf("%s: %s", name, err.Error()))
	}
	if _, err := b.repo.Reference(refName, false); err == nil {
		return giterr.AlreadyExists("branch", name)
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return giterr.Internal(err)
	}

	hash, err := b.repo.ResolveRevision(plumbing.Revision(target))
	if err != nil {
		return giterr.RefNotFound(target)
	}
	if err := b.repo.Storer.SetReference(plumbing.NewHashReference(refName, *hash)); err != nil {
		return giterr.Internal(err)
	}
	return nil
}

// DeleteBranch deletes a local branch.
func (b *Backend) DeleteBranch(name string) error {
	refName := plumbing.NewBranchReferenceName(name)
	if _, err := b.repo.Reference(refName, false); err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return giterr.RefNotFound(name)
		}
		return giterr.Internal(err)
	}
	head, err := b.repo.Head()
	if err == nil && head.Name() == refName {
		return giterr.CheckedOutBranch(name)
	}
	if err := b.repo.Storer.RemoveReference(refName); err != nil {
		return giterr.Internal(err)
	}
	if err := b.repo.DeleteBranch(name); err != nil && !errors.Is(err, gogit.ErrBranchNotFound) {
		return giterr.Internal(err)
	}
	return nil
}

// TrackingBranch returns the configured upstream for a local branch.
func (b *Backend) TrackingBranch(branch string) (string, error) {
	refName := plumbing.NewBranchReferenceName(branch)
	if _, err := b.repo.Reference(refName, false); err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", giterr.RefNotFound(branch)
		}
		return "", giterr.Internal(err)
	}
	cfg, err := b.repo.Config()
	if err != nil {
		return "", giterr.Internal(err)
	}
	return branchUpstream(cfg, branch), nil
}

func matchesBranchFilter(filter model.BranchFilter, name plumbing.ReferenceName) bool {
	switch filter {
	case model.RemoteBranches:
		return name.IsRemote()
	case model.AllBranches:
		return name.IsBranch() || name.IsRemote()
	default:
		return name.IsBranch()
	}
}

func branchUpstream(cfg *ggconfig.Config, name string) string {
	branchCfg, ok := cfg.Branches[name]
	if !ok || branchCfg.Remote == "" || branchCfg.Merge == "" {
		return ""
	}
	upstream := strings.TrimPrefix(branchCfg.Merge.String(), "refs/heads/")
	if branchCfg.Remote == "." {
		return upstream
	}
	return branchCfg.Remote + "/" + upstream
}
