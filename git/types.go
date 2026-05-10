package git

import "github.com/kbukum/gokit/git/internal/model"

type Oid = model.Oid
type TreeHash = model.TreeHash
type Reference = model.Reference
type Signature = model.Signature
type Commit = model.Commit
type FileStatus = model.FileStatus
type DiffEntry = model.DiffEntry
type DiffStats = model.DiffStats
type EntryState = model.EntryState
type StatusEntry = model.StatusEntry
type EntryKind = model.EntryKind
type TreeEntry = model.TreeEntry
type Branch = model.Branch
type Tag = model.Tag
type Remote = model.Remote
type BlameLine = model.BlameLine
type BranchFilter = model.BranchFilter
type GrepMatch = model.GrepMatch
type ResetMode = model.ResetMode
type StashEntry = model.StashEntry
type MergeResult = model.MergeResult
type RebaseResult = model.RebaseResult

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
