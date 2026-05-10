package model

import (
	"time"

	gitauth "github.com/kbukum/gokit/git/auth"
)

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

// OpenOptions controls repository construction and backend selection.
type OpenOptions struct {
	PreferCLI bool
	CLIPath   string
	Transport gitauth.Transport
	Signing   gitauth.Signing
	ExtraArgs []string
}

// Option configures repository construction behavior.
type Option func(*OpenOptions)

// WithPreferCLI requests CLI-backed operations where available.
func WithPreferCLI(prefer bool) Option { return func(opts *OpenOptions) { opts.PreferCLI = prefer } }

// WithCLIPath sets the git executable path used by the CLI backend.
func WithCLIPath(path string) Option { return func(opts *OpenOptions) { opts.CLIPath = path } }

// WithTransport sets the transport auth configuration used by clone/fetch/push operations.
func WithTransport(transport gitauth.Transport) Option {
	return func(opts *OpenOptions) { opts.Transport = transport }
}

// WithSigning sets the signing configuration used by write operations when supported.
func WithSigning(signing gitauth.Signing) Option {
	return func(opts *OpenOptions) { opts.Signing = signing }
}

// WithExtraArgs appends raw CLI args for backends that support them.
func WithExtraArgs(args ...string) Option {
	return func(opts *OpenOptions) { opts.ExtraArgs = append(opts.ExtraArgs, args...) }
}

// ApplyOptions materializes constructor options.
func ApplyOptions(opts ...Option) *OpenOptions {
	cfg := &OpenOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// DiffOptions controls diff generation.
type DiffOptions struct {
	ContextLines int
	NameOnly     bool
	ExtraArgs    []string
}

// DiffOption configures diff generation.
type DiffOption func(*DiffOptions)

// WithDiffContext sets the requested number of context lines.
func WithDiffContext(lines int) DiffOption {
	return func(opts *DiffOptions) { opts.ContextLines = lines }
}

// WithDiffNameOnly requests name-only diff output when supported.
func WithDiffNameOnly(nameOnly bool) DiffOption {
	return func(opts *DiffOptions) { opts.NameOnly = nameOnly }
}

// WithDiffExtraArgs appends backend-specific raw diff args.
func WithDiffExtraArgs(args ...string) DiffOption {
	return func(opts *DiffOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// LogOptions controls log traversal.
type LogOptions struct {
	MaxCount     int
	PathFilter   string
	AuthorFilter string
	Since        *time.Time
	Until        *time.Time
	ExtraArgs    []string
}

// WithLogExtraArgs appends backend-specific raw log args.
func WithLogExtraArgs(args ...string) func(*LogOptions) {
	return func(opts *LogOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// BlameOptions controls blame output.
type BlameOptions struct {
	StartLine        int
	EndLine          int
	IgnoreWhitespace bool
	ExtraArgs        []string
}

// BlameOption configures blame behavior.
type BlameOption func(*BlameOptions)

// WithLineRange limits blame results to the inclusive range [start, end].
func WithLineRange(start, end int) BlameOption {
	return func(opts *BlameOptions) {
		opts.StartLine = start
		opts.EndLine = end
	}
}

// WithIgnoreWhitespace requests whitespace-insensitive blame when supported.
func WithIgnoreWhitespace(ignore bool) BlameOption {
	return func(opts *BlameOptions) { opts.IgnoreWhitespace = ignore }
}

// WithBlameExtraArgs appends backend-specific raw blame args.
func WithBlameExtraArgs(args ...string) BlameOption {
	return func(opts *BlameOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// CommitOptions controls commit creation.
type CommitOptions struct {
	Author    *Signature
	Committer *Signature
	Sign      bool
	Amend     bool
	ExtraArgs []string
}

// CommitOption configures commit behavior.
type CommitOption func(*CommitOptions)

// WithCommitAuthor sets the author signature for a commit.
func WithCommitAuthor(sig Signature) CommitOption {
	return func(opts *CommitOptions) {
		copySig := sig
		opts.Author = &copySig
	}
}

// WithCommitCommitter sets the committer signature for a commit.
func WithCommitCommitter(sig Signature) CommitOption {
	return func(opts *CommitOptions) {
		copySig := sig
		opts.Committer = &copySig
	}
}

// WithCommitSign requests commit signing.
func WithCommitSign(sign bool) CommitOption { return func(opts *CommitOptions) { opts.Sign = sign } }

// WithCommitAmend requests amending the current HEAD commit.
func WithCommitAmend(amend bool) CommitOption {
	return func(opts *CommitOptions) { opts.Amend = amend }
}

// WithCommitExtraArgs appends backend-specific raw commit args.
func WithCommitExtraArgs(args ...string) CommitOption {
	return func(opts *CommitOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// FetchOptions controls fetch behavior.
type FetchOptions struct {
	Prune     bool
	Depth     int
	Refspecs  []string
	ExtraArgs []string
}

// FetchOption configures fetch behavior.
type FetchOption func(*FetchOptions)

// WithFetchPrune sets whether fetch prunes stale remote-tracking refs.
func WithFetchPrune(prune bool) FetchOption { return func(opts *FetchOptions) { opts.Prune = prune } }

// WithFetchDepth sets fetch depth.
func WithFetchDepth(depth int) FetchOption { return func(opts *FetchOptions) { opts.Depth = depth } }

// WithFetchRefspecs sets fetch refspecs.
func WithFetchRefspecs(refspecs ...string) FetchOption {
	return func(opts *FetchOptions) { opts.Refspecs = append([]string(nil), refspecs...) }
}

// WithFetchExtraArgs appends backend-specific raw fetch args.
func WithFetchExtraArgs(args ...string) FetchOption {
	return func(opts *FetchOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// PushOptions controls push behavior.
type PushOptions struct {
	Force     bool
	Refspecs  []string
	ExtraArgs []string
}

// PushOption configures push behavior.
type PushOption func(*PushOptions)

// WithPushForce sets whether push is forced.
func WithPushForce(force bool) PushOption { return func(opts *PushOptions) { opts.Force = force } }

// WithPushRefspecs sets push refspecs.
func WithPushRefspecs(refspecs ...string) PushOption {
	return func(opts *PushOptions) { opts.Refspecs = append([]string(nil), refspecs...) }
}

// WithPushExtraArgs appends backend-specific raw push args.
func WithPushExtraArgs(args ...string) PushOption {
	return func(opts *PushOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// DescribeOptions controls git describe output.
type DescribeOptions struct {
	Match             string
	AnnotatedTagsOnly bool
	Long              bool
	Abbreviated       bool
	Always            bool
	ExtraArgs         []string
}

// DescribeOption configures describe behavior.
type DescribeOption func(*DescribeOptions)

// WithDescribeMatch restricts matching tags.
func WithDescribeMatch(match string) DescribeOption {
	return func(opts *DescribeOptions) { opts.Match = match }
}

// WithDescribeAnnotatedTagsOnly limits describe to annotated tags.
func WithDescribeAnnotatedTagsOnly(only bool) DescribeOption {
	return func(opts *DescribeOptions) { opts.AnnotatedTagsOnly = only }
}

// WithDescribeLong requests long describe output.
func WithDescribeLong(long bool) DescribeOption {
	return func(opts *DescribeOptions) { opts.Long = long }
}

// WithDescribeAbbreviated controls abbreviated fallback output.
func WithDescribeAbbreviated(abbreviated bool) DescribeOption {
	return func(opts *DescribeOptions) { opts.Abbreviated = abbreviated }
}

// WithDescribeAlways requests fallback to an object name.
func WithDescribeAlways(always bool) DescribeOption {
	return func(opts *DescribeOptions) { opts.Always = always }
}

// WithDescribeExtraArgs appends backend-specific raw describe args.
func WithDescribeExtraArgs(args ...string) DescribeOption {
	return func(opts *DescribeOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// GrepOptions controls git grep behavior.
type GrepOptions struct {
	Pathspecs   []string
	IgnoreCase  bool
	LineNumbers bool
	ExtraArgs   []string
}

// GrepOption configures grep behavior.
type GrepOption func(*GrepOptions)

// WithGrepPathspecs restricts grep to the provided paths.
func WithGrepPathspecs(pathspecs ...string) GrepOption {
	return func(opts *GrepOptions) { opts.Pathspecs = append([]string(nil), pathspecs...) }
}

// WithGrepIgnoreCase toggles case-insensitive matching.
func WithGrepIgnoreCase(ignore bool) GrepOption {
	return func(opts *GrepOptions) { opts.IgnoreCase = ignore }
}

// WithGrepLineNumbers toggles line-number output when supported.
func WithGrepLineNumbers(enabled bool) GrepOption {
	return func(opts *GrepOptions) { opts.LineNumbers = enabled }
}

// WithGrepExtraArgs appends backend-specific raw grep args.
func WithGrepExtraArgs(args ...string) GrepOption {
	return func(opts *GrepOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// MergeOptions controls merge behavior.
type MergeOptions struct {
	Commit        bool
	FFOnly        bool
	NoFastForward bool
	Squash        bool
	Message       string
	ExtraArgs     []string
}

// MergeOption configures merge behavior.
type MergeOption func(*MergeOptions)

// WithMergeCommit toggles automatic merge commit creation.
func WithMergeCommit(commit bool) MergeOption {
	return func(opts *MergeOptions) { opts.Commit = commit }
}

// WithMergeFFOnly requests fast-forward-only merges.
func WithMergeFFOnly(ffOnly bool) MergeOption {
	return func(opts *MergeOptions) { opts.FFOnly = ffOnly }
}

// WithMergeNoFastForward requests a merge commit even when fast-forward is possible.
func WithMergeNoFastForward(noFF bool) MergeOption {
	return func(opts *MergeOptions) { opts.NoFastForward = noFF }
}

// WithMergeSquash requests squash merge behavior.
func WithMergeSquash(squash bool) MergeOption {
	return func(opts *MergeOptions) { opts.Squash = squash }
}

// WithMergeMessage sets the merge commit message.
func WithMergeMessage(message string) MergeOption {
	return func(opts *MergeOptions) { opts.Message = message }
}

// WithMergeExtraArgs appends backend-specific raw merge args.
func WithMergeExtraArgs(args ...string) MergeOption {
	return func(opts *MergeOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// RebaseOptions controls rebase behavior.
type RebaseOptions struct {
	Interactive bool
	Autosquash  bool
	ExtraArgs   []string
}

// RebaseOption configures rebase behavior.
type RebaseOption func(*RebaseOptions)

// WithRebaseInteractive toggles interactive rebase mode.
func WithRebaseInteractive(interactive bool) RebaseOption {
	return func(opts *RebaseOptions) { opts.Interactive = interactive }
}

// WithRebaseAutosquash toggles autosquash behavior.
func WithRebaseAutosquash(autosquash bool) RebaseOption {
	return func(opts *RebaseOptions) { opts.Autosquash = autosquash }
}

// WithRebaseExtraArgs appends backend-specific raw rebase args.
func WithRebaseExtraArgs(args ...string) RebaseOption {
	return func(opts *RebaseOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// CherryPickOptions controls cherry-pick behavior.
type CherryPickOptions struct {
	Mainline  int
	NoCommit  bool
	ExtraArgs []string
}

// CherryPickOption configures cherry-pick behavior.
type CherryPickOption func(*CherryPickOptions)

// WithCherryPickMainline selects the mainline parent for merge commits.
func WithCherryPickMainline(mainline int) CherryPickOption {
	return func(opts *CherryPickOptions) { opts.Mainline = mainline }
}

// WithCherryPickNoCommit applies changes without creating a commit.
func WithCherryPickNoCommit(noCommit bool) CherryPickOption {
	return func(opts *CherryPickOptions) { opts.NoCommit = noCommit }
}

// WithCherryPickExtraArgs appends backend-specific raw cherry-pick args.
func WithCherryPickExtraArgs(args ...string) CherryPickOption {
	return func(opts *CherryPickOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// CheckoutOptions controls checkout behavior.
type CheckoutOptions struct {
	CreateBranch string
	Force        bool
	Detach       bool
	ExtraArgs    []string
}

// CheckoutOption configures checkout behavior.
type CheckoutOption func(*CheckoutOptions)

// WithCheckoutCreateBranch creates a new branch while checking out.
func WithCheckoutCreateBranch(name string) CheckoutOption {
	return func(opts *CheckoutOptions) { opts.CreateBranch = name }
}

// WithCheckoutForce forces checkout.
func WithCheckoutForce(force bool) CheckoutOption {
	return func(opts *CheckoutOptions) { opts.Force = force }
}

// WithCheckoutDetach detaches HEAD at the target revision.
func WithCheckoutDetach(detach bool) CheckoutOption {
	return func(opts *CheckoutOptions) { opts.Detach = detach }
}

// WithCheckoutExtraArgs appends backend-specific raw checkout args.
func WithCheckoutExtraArgs(args ...string) CheckoutOption {
	return func(opts *CheckoutOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}

// CleanOptions controls git clean behavior.
type CleanOptions struct {
	Directories bool
	Ignored     bool
	Force       bool
	ExtraArgs   []string
}

// CleanOption configures clean behavior.
type CleanOption func(*CleanOptions)

// WithCleanDirectories includes untracked directories in clean operations.
func WithCleanDirectories(directories bool) CleanOption {
	return func(opts *CleanOptions) { opts.Directories = directories }
}

// WithCleanIgnored includes ignored files in clean operations.
func WithCleanIgnored(ignored bool) CleanOption {
	return func(opts *CleanOptions) { opts.Ignored = ignored }
}

// WithCleanForce toggles destructive clean mode.
func WithCleanForce(force bool) CleanOption { return func(opts *CleanOptions) { opts.Force = force } }

// WithCleanExtraArgs appends backend-specific raw clean args.
func WithCleanExtraArgs(args ...string) CleanOption {
	return func(opts *CleanOptions) { opts.ExtraArgs = append([]string(nil), args...) }
}
