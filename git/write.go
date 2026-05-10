package git

// IndexManager provides staging area operations.
type IndexManager interface {
	// Stage adds the provided paths to the repository index.
	Stage(paths ...string) error
	// Unstage removes paths from the index while preserving worktree changes.
	Unstage(paths ...string) error
	// StagedEntries returns staged files from the repository index.
	StagedEntries() ([]StatusEntry, error)
}

// Committer provides commit creation.
type Committer interface {
	// Commit creates a new commit from the current index state.
	Commit(message string, opts ...CommitOption) (Oid, error)
}

// Merger provides merge operations.
type Merger interface {
	// Merge merges the provided revision into the current HEAD.
	Merge(revision string) error
	// MergeAbort aborts an in-progress merge.
	MergeAbort() error
}

// Rebaser provides rebase operations.
type Rebaser interface {
	// Rebase rebases the current branch onto the provided revision.
	Rebase(onto string) error
	// RebaseContinue continues an in-progress rebase.
	RebaseContinue() error
	// RebaseAbort aborts an in-progress rebase.
	RebaseAbort() error
}

// CherryPicker provides cherry-pick operations.
type CherryPicker interface {
	// CherryPick applies the given revision onto the current HEAD.
	CherryPick(revision string) error
	// CherryPickContinue continues an in-progress cherry-pick.
	CherryPickContinue() error
	// CherryPickAbort aborts an in-progress cherry-pick.
	CherryPickAbort() error
}

// Resetter provides reset operations.
type Resetter interface {
	// Reset resets the repository to target using the requested mode.
	Reset(target string, mode ResetMode, paths ...string) error
}

// Checker provides checkout operations.
type Checker interface {
	// Checkout updates HEAD, index, or paths to the requested target.
	Checkout(target string, paths ...string) error
}

// Stasher provides stash operations.
type Stasher interface {
	// StashPush saves worktree and index state with an optional message.
	StashPush(message string) error
	// StashPop applies and drops the stash entry at index.
	StashPop(index int) error
	// StashList lists available stash entries.
	StashList() ([]StashEntry, error)
}
