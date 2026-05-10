package git

import (
	cliimpl "github.com/kbukum/gokit/git/cli"
	"github.com/kbukum/gokit/git/embedded"
	"github.com/kbukum/gokit/git/internal/model"
)

// Repo orchestrates git operations across the available backends.
type Repo struct {
	embedded *embedded.Backend
	cli      *cliimpl.Backend
}

var (
	_ Repository    = (*Repo)(nil)
	_ Executor      = (*Repo)(nil)
	_ Differ        = (*Repo)(nil)
	_ TreeReader    = (*Repo)(nil)
	_ LogReader     = (*Repo)(nil)
	_ Blamer        = (*Repo)(nil)
	_ Inspector     = (*Repo)(nil)
	_ IndexManager  = (*Repo)(nil)
	_ Committer     = (*Repo)(nil)
	_ Merger        = (*Repo)(nil)
	_ Rebaser       = (*Repo)(nil)
	_ CherryPicker  = (*Repo)(nil)
	_ Resetter      = (*Repo)(nil)
	_ Checker       = (*Repo)(nil)
	_ Stasher       = (*Repo)(nil)
	_ RefManager    = (*Repo)(nil)
	_ RemoteManager = (*Repo)(nil)
	_ ConfigReader  = (*Repo)(nil)
	_ Maintainer    = (*Repo)(nil)

	_ Repository    = (*embedded.Backend)(nil)
	_ Differ        = (*embedded.Backend)(nil)
	_ TreeReader    = (*embedded.Backend)(nil)
	_ LogReader     = (*embedded.Backend)(nil)
	_ Blamer        = (*embedded.Backend)(nil)
	_ IndexManager  = (*embedded.Backend)(nil)
	_ Committer     = (*embedded.Backend)(nil)
	_ RefManager    = (*embedded.Backend)(nil)
	_ RemoteManager = (*embedded.Backend)(nil)
	_ ConfigReader  = (*embedded.Backend)(nil)

	_ Executor     = (*cliimpl.Backend)(nil)
	_ Inspector    = (*cliimpl.Backend)(nil)
	_ Merger       = (*cliimpl.Backend)(nil)
	_ Rebaser      = (*cliimpl.Backend)(nil)
	_ CherryPicker = (*cliimpl.Backend)(nil)
	_ Resetter     = (*cliimpl.Backend)(nil)
	_ Checker      = (*cliimpl.Backend)(nil)
	_ Stasher      = (*cliimpl.Backend)(nil)
	_ Maintainer   = (*cliimpl.Backend)(nil)
)

// Open opens a git repository at the given path.
func Open(path string, opts ...Option) (*Repo, error) {
	cfg := model.ApplyOptions(opts...)
	backend, err := embedded.Open(path, cfg)
	if err != nil {
		return nil, err
	}
	return newRepo(backend, cfg), nil
}

// Discover finds a git repository by walking up from the given path.
func Discover(path string, opts ...Option) (*Repo, error) {
	cfg := model.ApplyOptions(opts...)
	backend, err := embedded.Discover(path, cfg)
	if err != nil {
		return nil, err
	}
	return newRepo(backend, cfg), nil
}

// Clone clones a repository into path.
func Clone(url, path string, opts ...Option) (*Repo, error) {
	cfg := model.ApplyOptions(opts...)
	backend, err := embedded.Clone(url, path, cfg)
	if err != nil {
		return nil, err
	}
	return newRepo(backend, cfg), nil
}

// Init creates a new git repository at the given path.
func Init(path string, opts ...Option) (*Repo, error) {
	cfg := model.ApplyOptions(opts...)
	backend, err := embedded.Init(path)
	if err != nil {
		return nil, err
	}
	return newRepo(backend, cfg), nil
}

// InitBare creates a new bare git repository at the given path.
func InitBare(path string, opts ...Option) (*Repo, error) {
	cfg := model.ApplyOptions(opts...)
	backend, err := embedded.InitBare(path)
	if err != nil {
		return nil, err
	}
	return newRepo(backend, cfg), nil
}

func newRepo(backend *embedded.Backend, cfg *model.OpenOptions) *Repo {
	return &Repo{
		embedded: backend,
		cli:      cliimpl.New(backend.Root(), cfg),
	}
}

func (r *Repo) Root() string                                 { return r.embedded.Root() }
func (r *Repo) Head() (Reference, error)                     { return r.embedded.Head() }
func (r *Repo) ResolveRef(refname string) (Oid, error)       { return r.embedded.ResolveRef(refname) }
func (r *Repo) IsDirty() (bool, error)                       { return r.embedded.IsDirty() }
func (r *Repo) Exec(args ...string) ([]byte, error)          { return r.cli.Exec(args...) }
func (r *Repo) Diff(from, to string) ([]DiffEntry, error)    { return r.embedded.Diff(from, to) }
func (r *Repo) DiffStats(from, to string) (DiffStats, error) { return r.embedded.DiffStats(from, to) }
func (r *Repo) Status() ([]StatusEntry, error)               { return r.embedded.Status() }
func (r *Repo) TreeHash(revision, path string) (TreeHash, error) {
	return r.embedded.TreeHash(revision, path)
}
func (r *Repo) FileAt(revision, path string) ([]byte, error) {
	return r.embedded.FileAt(revision, path)
}
func (r *Repo) ListEntries(revision, path string) ([]TreeEntry, error) {
	return r.embedded.ListEntries(revision, path)
}
func (r *Repo) Log(opts LogOptions) ([]Commit, error) { return r.embedded.Log(opts) }
func (r *Repo) MergeBase(a, b string) (Oid, error)    { return r.embedded.MergeBase(a, b) }
func (r *Repo) IsAncestor(a, b string) (bool, error)  { return r.embedded.IsAncestor(a, b) }
func (r *Repo) Blame(revision, path string, opts ...BlameOption) ([]BlameLine, error) {
	return r.embedded.Blame(revision, path, opts...)
}
func (r *Repo) Stage(paths ...string) error           { return r.embedded.Stage(paths...) }
func (r *Repo) Unstage(paths ...string) error         { return r.embedded.Unstage(paths...) }
func (r *Repo) StagedEntries() ([]StatusEntry, error) { return r.embedded.StagedEntries() }
func (r *Repo) Commit(message string, opts ...CommitOption) (Oid, error) {
	return r.embedded.Commit(message, opts...)
}
func (r *Repo) ListBranches(filter BranchFilter) ([]Branch, error) {
	return r.embedded.ListBranches(filter)
}
func (r *Repo) ListTags() ([]Tag, error)               { return r.embedded.ListTags() }
func (r *Repo) CreateBranch(name, target string) error { return r.embedded.CreateBranch(name, target) }
func (r *Repo) DeleteBranch(name string) error         { return r.embedded.DeleteBranch(name) }
func (r *Repo) CreateTag(name, target, message string) error {
	return r.embedded.CreateTag(name, target, message)
}
func (r *Repo) DeleteTag(name string) error    { return r.embedded.DeleteTag(name) }
func (r *Repo) ListRemotes() ([]Remote, error) { return r.embedded.ListRemotes() }
func (r *Repo) Fetch(remote string, opts ...FetchOption) error {
	return r.embedded.Fetch(remote, opts...)
}
func (r *Repo) Push(remote string, opts ...PushOption) error { return r.embedded.Push(remote, opts...) }
func (r *Repo) TrackingBranch(branch string) (string, error) {
	return r.embedded.TrackingBranch(branch)
}
func (r *Repo) ConfigGet(key string) (string, error)      { return r.embedded.ConfigGet(key) }
func (r *Repo) ConfigGetAll(key string) ([]string, error) { return r.embedded.ConfigGetAll(key) }
func (r *Repo) ConfigSet(key, value string) error         { return r.embedded.ConfigSet(key, value) }
func (r *Repo) Describe(revision string) (string, error)  { return r.cli.Describe(revision) }
func (r *Repo) RevParse(spec string) (Oid, error)         { return r.cli.RevParse(spec) }
func (r *Repo) Grep(pattern string, paths ...string) ([]GrepMatch, error) {
	return r.cli.Grep(pattern, paths...)
}
func (r *Repo) Show(spec string) ([]byte, error) { return r.cli.Show(spec) }
func (r *Repo) Merge(revision string) error      { return r.cli.Merge(revision) }
func (r *Repo) MergeAbort() error                { return r.cli.MergeAbort() }
func (r *Repo) Rebase(onto string) error         { return r.cli.Rebase(onto) }
func (r *Repo) RebaseContinue() error            { return r.cli.RebaseContinue() }
func (r *Repo) RebaseAbort() error               { return r.cli.RebaseAbort() }
func (r *Repo) CherryPick(revision string) error { return r.cli.CherryPick(revision) }
func (r *Repo) CherryPickContinue() error        { return r.cli.CherryPickContinue() }
func (r *Repo) CherryPickAbort() error           { return r.cli.CherryPickAbort() }
func (r *Repo) Reset(target string, mode ResetMode, paths ...string) error {
	return r.cli.Reset(target, mode, paths...)
}
func (r *Repo) Checkout(target string, paths ...string) error {
	return r.cli.Checkout(target, paths...)
}
func (r *Repo) StashPush(message string) error   { return r.cli.StashPush(message) }
func (r *Repo) StashPop(index int) error         { return r.cli.StashPop(index) }
func (r *Repo) StashList() ([]StashEntry, error) { return r.cli.StashList() }
func (r *Repo) GC() error                        { return r.cli.GC() }
func (r *Repo) Prune() error                     { return r.cli.Prune() }
func (r *Repo) Fsck() error                      { return r.cli.Fsck() }
func (r *Repo) Clean(opts ...CleanOption) ([]string, error) {
	return r.cli.Clean(opts...)
}
