package embedded

import (
	"path/filepath"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

// Stage adds the provided paths to the repository index.
func (b *Backend) Stage(paths ...string) error {
	wt, err := b.repo.Worktree()
	if err != nil {
		return giterr.Internal(err)
	}
	for _, name := range paths {
		path, err := b.normalizeWorktreePath(name)
		if err != nil {
			return err
		}
		if _, err := wt.Add(path); err != nil {
			return giterr.Internal(err)
		}
	}
	return nil
}

// Unstage removes the provided paths from the repository index while preserving worktree changes.
func (b *Backend) Unstage(paths ...string) error {
	normalized := make([]string, 0, len(paths))
	for _, name := range paths {
		path, err := b.normalizeWorktreePath(name)
		if err != nil {
			return err
		}
		normalized = append(normalized, path)
	}
	if len(normalized) == 0 {
		return nil
	}

	wt, err := b.repo.Worktree()
	if err != nil {
		return giterr.Internal(err)
	}
	if err := wt.Reset(&gogit.ResetOptions{Mode: gogit.MixedReset, Files: normalized}); err != nil {
		return giterr.Internal(err)
	}
	return nil
}

// StagedEntries returns staged files from the repository index.
func (b *Backend) StagedEntries() ([]model.StatusEntry, error) {
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
		state, ok := stagedEntryState(fileStatus)
		if !ok {
			continue
		}
		entries = append(entries, model.StatusEntry{Path: path, State: state})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// Commit creates a new commit from the current index state.
func (b *Backend) Commit(message string, opts ...model.CommitOption) (model.Oid, error) {
	cfg := model.CommitOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.Sign {
		return model.Oid{}, giterr.SigningNotSupported()
	}

	wt, err := b.repo.Worktree()
	if err != nil {
		return model.Oid{}, giterr.Internal(err)
	}
	commitOpts := gogitCommitOptions(cfg)
	hash, err := wt.Commit(message, &commitOpts)
	if err != nil {
		return model.Oid{}, giterr.Internal(err)
	}
	return oidFromHash(hash), nil
}

func stagedEntryState(fs *gogit.FileStatus) (model.EntryState, bool) {
	if fs == nil {
		return model.Staged, false
	}
	if fs.Staging == gogit.UpdatedButUnmerged || fs.Worktree == gogit.UpdatedButUnmerged {
		return model.Conflicted, true
	}
	if fs.Staging == gogit.Unmodified || fs.Staging == gogit.Untracked {
		return model.Staged, false
	}
	return model.Staged, true
}

func (b *Backend) normalizeWorktreePath(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", giterr.InvalidPath(name)
	}
	clean := filepath.Clean(trimmed)
	if filepath.IsAbs(clean) {
		rel, err := filepath.Rel(b.root, clean)
		if err != nil {
			return "", giterr.Internal(err)
		}
		clean = rel
	}
	clean = filepath.ToSlash(clean)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", giterr.InvalidPath(name)
	}
	return clean, nil
}

func gogitCommitOptions(opts model.CommitOptions) gogit.CommitOptions {
	return gogit.CommitOptions{Author: signatureToObject(opts.Author), Committer: signatureToObject(opts.Committer), Amend: opts.Amend}
}

func signatureToObject(sig *model.Signature) *object.Signature {
	if sig == nil {
		return nil
	}
	return &object.Signature{Name: sig.Name, Email: sig.Email, When: sig.When}
}
