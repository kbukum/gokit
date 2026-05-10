package git

import "github.com/kbukum/gokit/git/internal/model"

type (
	OpenOptions       = model.OpenOptions
	Option            = model.Option
	DiffOptions       = model.DiffOptions
	DiffOption        = model.DiffOption
	LogOptions        = model.LogOptions
	BlameOptions      = model.BlameOptions
	BlameOption       = model.BlameOption
	CommitOptions     = model.CommitOptions
	CommitOption      = model.CommitOption
	FetchOptions      = model.FetchOptions
	FetchOption       = model.FetchOption
	PushOptions       = model.PushOptions
	PushOption        = model.PushOption
	DescribeOptions   = model.DescribeOptions
	DescribeOption    = model.DescribeOption
	GrepOptions       = model.GrepOptions
	GrepOption        = model.GrepOption
	MergeOptions      = model.MergeOptions
	MergeOption       = model.MergeOption
	RebaseOptions     = model.RebaseOptions
	RebaseOption      = model.RebaseOption
	CherryPickOptions = model.CherryPickOptions
	CherryPickOption  = model.CherryPickOption
	CheckoutOptions   = model.CheckoutOptions
	CheckoutOption    = model.CheckoutOption
	CleanOptions      = model.CleanOptions
	CleanOption       = model.CleanOption
)

var (
	WithPreferCLI                 = model.WithPreferCLI
	WithCLIPath                   = model.WithCLIPath
	WithTransport                 = model.WithTransport
	WithSigning                   = model.WithSigning
	WithExtraArgs                 = model.WithExtraArgs
	WithDiffContext               = model.WithDiffContext
	WithDiffNameOnly              = model.WithDiffNameOnly
	WithDiffExtraArgs             = model.WithDiffExtraArgs
	WithLogExtraArgs              = model.WithLogExtraArgs
	WithLineRange                 = model.WithLineRange
	WithIgnoreWhitespace          = model.WithIgnoreWhitespace
	WithBlameExtraArgs            = model.WithBlameExtraArgs
	WithCommitAuthor              = model.WithCommitAuthor
	WithCommitCommitter           = model.WithCommitCommitter
	WithCommitSign                = model.WithCommitSign
	WithCommitAmend               = model.WithCommitAmend
	WithCommitExtraArgs           = model.WithCommitExtraArgs
	WithFetchPrune                = model.WithFetchPrune
	WithFetchDepth                = model.WithFetchDepth
	WithFetchRefspecs             = model.WithFetchRefspecs
	WithFetchExtraArgs            = model.WithFetchExtraArgs
	WithPushForce                 = model.WithPushForce
	WithPushRefspecs              = model.WithPushRefspecs
	WithPushExtraArgs             = model.WithPushExtraArgs
	WithDescribeMatch             = model.WithDescribeMatch
	WithDescribeAnnotatedTagsOnly = model.WithDescribeAnnotatedTagsOnly
	WithDescribeLong              = model.WithDescribeLong
	WithDescribeAbbreviated       = model.WithDescribeAbbreviated
	WithDescribeAlways            = model.WithDescribeAlways
	WithDescribeExtraArgs         = model.WithDescribeExtraArgs
	WithGrepPathspecs             = model.WithGrepPathspecs
	WithGrepIgnoreCase            = model.WithGrepIgnoreCase
	WithGrepLineNumbers           = model.WithGrepLineNumbers
	WithGrepExtraArgs             = model.WithGrepExtraArgs
	WithMergeCommit               = model.WithMergeCommit
	WithMergeFFOnly               = model.WithMergeFFOnly
	WithMergeNoFastForward        = model.WithMergeNoFastForward
	WithMergeSquash               = model.WithMergeSquash
	WithMergeMessage              = model.WithMergeMessage
	WithMergeExtraArgs            = model.WithMergeExtraArgs
	WithRebaseInteractive         = model.WithRebaseInteractive
	WithRebaseAutosquash          = model.WithRebaseAutosquash
	WithRebaseExtraArgs           = model.WithRebaseExtraArgs
	WithCherryPickMainline        = model.WithCherryPickMainline
	WithCherryPickNoCommit        = model.WithCherryPickNoCommit
	WithCherryPickExtraArgs       = model.WithCherryPickExtraArgs
	WithCheckoutCreateBranch      = model.WithCheckoutCreateBranch
	WithCheckoutForce             = model.WithCheckoutForce
	WithCheckoutDetach            = model.WithCheckoutDetach
	WithCheckoutExtraArgs         = model.WithCheckoutExtraArgs
	WithCleanDirectories          = model.WithCleanDirectories
	WithCleanIgnored              = model.WithCleanIgnored
	WithCleanForce                = model.WithCleanForce
	WithCleanExtraArgs            = model.WithCleanExtraArgs
)
