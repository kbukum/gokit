package embedded

import (
	"errors"
	"fmt"
	stdpath "path"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
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

// TreeHash returns the OID of the tree at the given path and revision.
func (b *Backend) TreeHash(revision, path string) (model.TreeHash, error) {
	tree, err := b.resolveTree(revision, path)
	if err != nil {
		return model.Oid{}, err
	}
	return oidFromHash(tree.Hash), nil
}

// FileAt returns the content of a file at the given revision and path.
func (b *Backend) FileAt(revision, path string) ([]byte, error) {
	commit, err := b.commitForRef(revision)
	if err != nil {
		return nil, err
	}

	path = strings.ReplaceAll(path, "\\", "/")
	file, err := commit.File(path)
	if err != nil {
		return nil, giterr.RefNotFound(fmt.Sprintf("%s:%s", revision, path))
	}

	content, err := file.Contents()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	return []byte(content), nil
}

// ListEntries returns the entries in a tree at the given revision and path.
func (b *Backend) ListEntries(revision, path string) ([]model.TreeEntry, error) {
	tree, err := b.resolveTree(revision, path)
	if err != nil {
		return nil, err
	}

	entries := make([]model.TreeEntry, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		entries = append(entries, model.TreeEntry{
			Name:     entry.Name,
			OID:      oidFromHash(entry.Hash),
			Kind:     entryKindFromMode(entry.Mode),
			Filemode: uint32(entry.Mode),
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

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

// Blame returns line-level attribution for a file at a revision.
func (b *Backend) Blame(revision, path string, opts ...model.BlameOption) ([]model.BlameLine, error) {
	cfg := model.BlameOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.EndLine > 0 && cfg.StartLine > cfg.EndLine {
		return nil, giterr.InvalidLineRange(cfg.StartLine, cfg.EndLine)
	}

	commit, err := b.commitForRef(revision)
	if err != nil {
		return nil, err
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if _, fileErr := commit.File(path); fileErr != nil {
		return nil, giterr.RefNotFound(fmt.Sprintf("%s:%s", revision, path))
	}

	result, err := gogit.Blame(commit, path)
	if err != nil {
		return nil, giterr.Internal(err)
	}

	start, end, err := blameRange(len(result.Lines), cfg)
	if err != nil {
		return nil, err
	}

	lines := make([]model.BlameLine, 0, end-start)
	for idx := start; idx < end; idx++ {
		line := result.Lines[idx]
		lines = append(lines, model.BlameLine{
			Line:      idx + 1,
			CommitOID: oidFromHash(line.Hash),
			Author:    model.Signature{Name: line.AuthorName, Email: line.Author, When: line.Date},
			Content:   line.Text,
		})
	}
	return lines, nil
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

func (b *Backend) commitForRef(ref string) (*object.Commit, error) {
	h, err := b.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, giterr.RefNotFound(ref)
	}
	commit, err := b.repo.CommitObject(*h)
	if err != nil {
		return nil, giterr.Internal(err)
	}
	return commit, nil
}

func (b *Backend) resolveTree(revision, path string) (*object.Tree, error) {
	commit, err := b.commitForRef(revision)
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if path == "" || path == "." || path == "/" {
		return tree, nil
	}
	subtree, err := tree.Tree(path)
	if err != nil {
		return nil, giterr.RefNotFound(fmt.Sprintf("%s:%s", revision, path))
	}
	return subtree, nil
}

func entryKindFromMode(m filemode.FileMode) model.EntryKind {
	switch m {
	case filemode.Dir:
		return model.EntryKindTree
	case filemode.Submodule:
		return model.EntryKindSubmodule
	default:
		return model.EntryKindBlob
	}
}

func commitFromObject(commit *object.Commit) model.Commit {
	parents := make([]model.Oid, 0, len(commit.ParentHashes))
	for _, parent := range commit.ParentHashes {
		parents = append(parents, oidFromHash(parent))
	}
	return model.Commit{
		OID:       oidFromHash(commit.Hash),
		Author:    signatureFromObject(commit.Author),
		Committer: signatureFromObject(commit.Committer),
		Message:   commit.Message,
		Parents:   parents,
	}
}

func signatureFromObject(sig object.Signature) model.Signature {
	return model.Signature{Name: sig.Name, Email: sig.Email, When: sig.When}
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

func blameRange(total int, opts model.BlameOptions) (start, end int, err error) {
	if opts.StartLine < 0 || opts.EndLine < 0 {
		return 0, 0, giterr.InvalidLineRange(opts.StartLine, opts.EndLine)
	}
	start = 1
	if opts.StartLine > 0 {
		start = opts.StartLine
	}
	end = total
	if opts.EndLine > 0 {
		end = opts.EndLine
	}
	if start < 1 || start > end {
		return 0, 0, giterr.InvalidLineRange(opts.StartLine, opts.EndLine)
	}
	if start > total {
		return total, total, nil
	}
	if end > total {
		end = total
	}
	return start - 1, end, nil
}
