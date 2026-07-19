package model

import "time"

// Oid represents a Git object ID (SHA-1 hash).
type Oid [20]byte

// String returns the hex-encoded OID.
func (o Oid) String() string {
	const hex = "0123456789abcdef"
	var buf [40]byte
	for i, b := range o {
		buf[i*2] = hex[b>>4]
		buf[i*2+1] = hex[b&0x0f]
	}
	return string(buf[:])
}

// IsZero reports whether o is the zero OID.
func (o Oid) IsZero() bool { return o == Oid{} }

// TreeHash is the OID of a tree object for content-addressed comparison.
type TreeHash = Oid

// Reference represents a git ref (branch, tag, or HEAD).
type Reference struct {
	Name     string
	Target   Oid
	IsBranch bool
	IsTag    bool
}

// Signature holds author or committer identity.
type Signature struct {
	Name  string
	Email string
	When  time.Time
}

// Commit holds metadata for a git commit object.
type Commit struct {
	OID       Oid
	Author    Signature
	Committer Signature
	Message   string
	Parents   []Oid
}

// FileStatus describes how a file changed in a diff.
type FileStatus int

const (
	FileAdded FileStatus = iota
	FileModified
	FileDeleted
	FileRenamed
	FileCopied
	FileUntracked
	FileIgnored
	FileTypeChanged
	FileConflicted
)

func (s FileStatus) String() string {
	switch s {
	case FileAdded:
		return "added"
	case FileModified:
		return "modified"
	case FileDeleted:
		return "deleted"
	case FileRenamed:
		return "renamed"
	case FileCopied:
		return "copied"
	case FileUntracked:
		return "untracked"
	case FileIgnored:
		return "ignored"
	case FileTypeChanged:
		return "typechanged"
	case FileConflicted:
		return "conflicted"
	default:
		return "unknown"
	}
}

// DiffEntry represents a single file change between two refs.
type DiffEntry struct {
	Path    string
	OldPath string
	OldOID  Oid
	NewOID  Oid
	Status  FileStatus
}

// DiffStats aggregates diff statistics.
type DiffStats struct {
	Additions    int
	Deletions    int
	FilesChanged int
}

// EntryState describes a file's state in the working tree or index.
type EntryState int

const (
	Staged EntryState = iota
	Unstaged
	Untracked
	Conflicted
)

func (s EntryState) String() string {
	switch s {
	case Staged:
		return "staged"
	case Unstaged:
		return "unstaged"
	case Untracked:
		return "untracked"
	case Conflicted:
		return "conflicted"
	default:
		return "unknown"
	}
}

// StatusEntry represents a file's status in the working tree.
type StatusEntry struct {
	Path  string
	State EntryState
}

// EntryKind describes the type of a tree entry.
type EntryKind int

const (
	EntryKindBlob EntryKind = iota
	EntryKindTree
	EntryKindSubmodule
)

func (k EntryKind) String() string {
	switch k {
	case EntryKindBlob:
		return "blob"
	case EntryKindTree:
		return "tree"
	case EntryKindSubmodule:
		return "submodule"
	default:
		return "unknown"
	}
}

// TreeEntry represents an entry within a git tree object.
type TreeEntry struct {
	Name     string
	OID      Oid
	Kind     EntryKind
	Filemode uint32
}

// Branch holds branch metadata.
type Branch struct {
	Name     string
	Target   Oid
	Upstream string
}

// Tag holds tag metadata.
type Tag struct {
	Name    string
	Target  Oid
	Tagger  *Signature
	Message string
}

// Remote holds remote repository metadata.
type Remote struct {
	Name       string
	URL        string
	FetchSpecs []string
	PushSpecs  []string
}

// BlameLine holds line-level attribution.
type BlameLine struct {
	Line      int
	CommitOID Oid
	Author    Signature
	Content   string
}

// BranchFilter controls which branches to list.
type BranchFilter int

const (
	LocalBranches BranchFilter = iota
	RemoteBranches
	AllBranches
)

// GrepMatch represents a single grep match.
type GrepMatch struct {
	Path    string
	Line    int
	Content string
}

// ResetMode describes reset behavior.
type ResetMode int

const (
	ResetMixed ResetMode = iota
	ResetSoft
	ResetHard
)

func (m ResetMode) String() string {
	switch m {
	case ResetMixed:
		return "mixed"
	case ResetSoft:
		return "soft"
	case ResetHard:
		return "hard"
	default:
		return "unknown"
	}
}

// StashEntry represents a stash reference.
type StashEntry struct {
	Index   int
	Name    string
	Message string
	Commit  Oid
}

// MergeResult summarizes a merge operation.
type MergeResult struct {
	Merged      bool
	Head        Oid
	FastForward bool
	Conflicts   []string
}

// RebaseResult summarizes a rebase operation.
type RebaseResult struct {
	Complete  bool
	Head      Oid
	Applied   int
	Conflicts []string
}
