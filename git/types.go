package git

import "github.com/kbukum/gokit/git/internal/model"

type (
	Oid          = model.Oid
	TreeHash     = model.TreeHash
	Reference    = model.Reference
	Signature    = model.Signature
	Commit       = model.Commit
	FileStatus   = model.FileStatus
	DiffEntry    = model.DiffEntry
	DiffStats    = model.DiffStats
	EntryState   = model.EntryState
	StatusEntry  = model.StatusEntry
	EntryKind    = model.EntryKind
	TreeEntry    = model.TreeEntry
	Branch       = model.Branch
	Tag          = model.Tag
	Remote       = model.Remote
	BlameLine    = model.BlameLine
	BranchFilter = model.BranchFilter
	GrepMatch    = model.GrepMatch
	ResetMode    = model.ResetMode
	StashEntry   = model.StashEntry
	MergeResult  = model.MergeResult
	RebaseResult = model.RebaseResult
)

const (
	FileAdded       = model.FileAdded
	FileModified    = model.FileModified
	FileDeleted     = model.FileDeleted
	FileRenamed     = model.FileRenamed
	FileCopied      = model.FileCopied
	FileUntracked   = model.FileUntracked
	FileIgnored     = model.FileIgnored
	FileTypeChanged = model.FileTypeChanged
	FileConflicted  = model.FileConflicted
)

const (
	Staged     = model.Staged
	Unstaged   = model.Unstaged
	Untracked  = model.Untracked
	Conflicted = model.Conflicted
)

const (
	EntryKindBlob      = model.EntryKindBlob
	EntryKindTree      = model.EntryKindTree
	EntryKindSubmodule = model.EntryKindSubmodule
)

const (
	LocalBranches  = model.LocalBranches
	RemoteBranches = model.RemoteBranches
	AllBranches    = model.AllBranches
)

const (
	ResetMixed = model.ResetMixed
	ResetSoft  = model.ResetSoft
	ResetHard  = model.ResetHard
)
