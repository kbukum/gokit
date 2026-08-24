package embedded

import (
	"errors"
	stdpath "path"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

// Log returns commits reachable from HEAD that match the provided filters.
func (b *Backend) Log(opts model.LogOptions) ([]model.Commit, error) {
	logOpts := &gogit.LogOptions{Order: gogit.LogOrderCommitterTime, Since: opts.Since, Until: opts.Until}
	if filter := normalizeGitPath(opts.PathFilter); filter != "" {
		logOpts.PathFilter = func(name string) bool { return matchesPathFilter(filter, name) }
	}

	iter, err := b.repo.Log(logOpts)
	if err != nil {
		return nil, giterr.Internal(err)
	}
	defer iter.Close()

	commits := make([]model.Commit, 0)
	authorFilter := strings.ToLower(strings.TrimSpace(opts.AuthorFilter))
	err = iter.ForEach(func(commit *object.Commit) error {
		if authorFilter != "" && !matchesAuthorFilter(commit, authorFilter) {
			return nil
		}
		commits = append(commits, commitFromObject(commit))
		if opts.MaxCount > 0 && len(commits) >= opts.MaxCount {
			return storer.ErrStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, storer.ErrStop) {
		return nil, giterr.Internal(err)
	}
	return commits, nil
}

// MergeBase returns a merge base for a and b.
func (b *Backend) MergeBase(a, other string) (model.Oid, error) {
	left, err := b.commitForRef(a)
	if err != nil {
		return model.Oid{}, err
	}
	right, err := b.commitForRef(other)
	if err != nil {
		return model.Oid{}, err
	}

	bases, err := left.MergeBase(right)
	if err != nil {
		return model.Oid{}, giterr.Internal(err)
	}
	if len(bases) == 0 {
		return model.Oid{}, giterr.RefNotFound(a + "..." + other)
	}

	sort.Slice(bases, func(i, j int) bool { return bases[i].Committer.When.After(bases[j].Committer.When) })
	return oidFromHash(bases[0].Hash), nil
}

// IsAncestor reports whether a is an ancestor of b.
func (b *Backend) IsAncestor(a, other string) (bool, error) {
	ancestor, err := b.commitForRef(a)
	if err != nil {
		return false, err
	}
	descendant, err := b.commitForRef(other)
	if err != nil {
		return false, err
	}

	ok, err := ancestor.IsAncestor(descendant)
	if err != nil {
		return false, giterr.Internal(err)
	}
	return ok, nil
}

func normalizeGitPath(name string) string {
	clean := stdpath.Clean(strings.TrimSpace(name))
	switch clean {
	case "", ".", "/":
		return ""
	default:
		return clean
	}
}

func matchesPathFilter(filter, name string) bool {
	cleanName := normalizeGitPath(name)
	return cleanName == filter || strings.HasPrefix(cleanName, filter+"/")
}

func matchesAuthorFilter(commit *object.Commit, filter string) bool {
	return strings.Contains(strings.ToLower(commit.Author.Name), filter) ||
		strings.Contains(strings.ToLower(commit.Author.Email), filter) ||
		strings.Contains(strings.ToLower(commit.Committer.Name), filter) ||
		strings.Contains(strings.ToLower(commit.Committer.Email), filter)
}
