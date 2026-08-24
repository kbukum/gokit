package embedded

import (
	"fmt"
	"sort"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

// Diff returns file changes between two refs.
func (b *Backend) Diff(from, to string) ([]model.DiffEntry, error) {
	fromTree, err := b.treeForRef(from)
	if err != nil {
		return nil, err
	}
	toTree, err := b.treeForRef(to)
	if err != nil {
		return nil, err
	}

	changes, err := object.DiffTree(fromTree, toTree)
	if err != nil {
		return nil, giterr.Internal(err)
	}

	entries := make([]model.DiffEntry, 0, len(changes))
	for _, change := range changes {
		entry, err := diffEntryFromChange(change)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// DiffStats returns aggregated statistics for changes between two refs.
func (b *Backend) DiffStats(from, to string) (model.DiffStats, error) {
	fromTree, err := b.treeForRef(from)
	if err != nil {
		return model.DiffStats{}, err
	}
	toTree, err := b.treeForRef(to)
	if err != nil {
		return model.DiffStats{}, err
	}

	changes, err := object.DiffTree(fromTree, toTree)
	if err != nil {
		return model.DiffStats{}, giterr.Internal(err)
	}

	patch, err := changes.Patch()
	if err != nil {
		return model.DiffStats{}, giterr.Internal(err)
	}

	var stats model.DiffStats
	for _, fileStat := range patch.Stats() {
		stats.Additions += fileStat.Addition
		stats.Deletions += fileStat.Deletion
		stats.FilesChanged++
	}
	return stats, nil
}

// Status returns the working tree status.
func (b *Backend) Status() ([]model.StatusEntry, error) {
	wt, err := b.repo.Worktree()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	status, err := wt.Status()
	if err != nil {
		return nil, giterr.Internal(err)
	}

	entries := make([]model.StatusEntry, 0, len(status))
	for path, fileStatus := range status {
		entries = append(entries, model.StatusEntry{Path: path, State: entryStateFromStatus(fileStatus)})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (b *Backend) treeForRef(ref string) (*object.Tree, error) {
	commit, err := b.commitForRef(ref)
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	return tree, nil
}

func diffEntryFromChange(c *object.Change) (model.DiffEntry, error) {
	action, err := c.Action()
	if err != nil {
		return model.DiffEntry{}, giterr.Internal(err)
	}

	entry := model.DiffEntry{}
	switch action {
	case merkletrie.Insert:
		entry.Path = c.To.Name
		entry.Status = model.FileAdded
		entry.NewOID = oidFromHash(c.To.TreeEntry.Hash)
	case merkletrie.Delete:
		entry.Path = c.From.Name
		entry.Status = model.FileDeleted
		entry.OldOID = oidFromHash(c.From.TreeEntry.Hash)
	case merkletrie.Modify:
		entry.Path = c.To.Name
		entry.OldOID = oidFromHash(c.From.TreeEntry.Hash)
		entry.NewOID = oidFromHash(c.To.TreeEntry.Hash)
		if c.From.Name != "" && c.To.Name != "" && c.From.Name != c.To.Name {
			entry.OldPath = c.From.Name
			entry.Status = model.FileRenamed
			break
		}
		entry.Status = model.FileModified
	default:
		return model.DiffEntry{}, giterr.Internal(fmt.Errorf("unsupported diff action: %v", action))
	}
	return entry, nil
}

func entryStateFromStatus(fs *gogit.FileStatus) model.EntryState {
	if fs == nil {
		return model.Staged
	}
	if fs.Staging == gogit.UpdatedButUnmerged || fs.Worktree == gogit.UpdatedButUnmerged {
		return model.Conflicted
	}
	if fs.Staging == gogit.Untracked || fs.Worktree == gogit.Untracked {
		return model.Untracked
	}
	if fs.Staging != gogit.Unmodified {
		return model.Staged
	}
	if fs.Worktree != gogit.Unmodified {
		return model.Unstaged
	}
	return model.Staged
}
