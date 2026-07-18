package git

// RefManager provides branch and tag CRUD operations.
type RefManager interface {
	// ListBranches lists repository branches matching filter.
	ListBranches(filter BranchFilter) ([]Branch, error)
	// ListTags lists repository tags.
	ListTags() ([]Tag, error)
	// CreateBranch creates a local branch at target.
	CreateBranch(name, target string) error
	// DeleteBranch deletes a local branch.
	DeleteBranch(name string) error
	// CreateTag creates a lightweight or annotated tag at target.
	CreateTag(name, target, message string) error
	// DeleteTag deletes a tag.
	DeleteTag(name string) error
}

// RemoteManager provides remote repository operations.
type RemoteManager interface {
	// ListRemotes lists configured remotes.
	ListRemotes() ([]Remote, error)
	// Fetch fetches updates from a remote.
	Fetch(remote string, opts ...FetchOption) error
	// Push pushes updates to a remote.
	Push(remote string, opts ...PushOption) error
	// TrackingBranch returns the configured upstream for a local branch.
	TrackingBranch(branch string) (string, error)
}

// ConfigReader provides git config access.
type ConfigReader interface {
	// ConfigGet gets the last configured value for a key.
	ConfigGet(key string) (string, error)
	// ConfigGetAll gets all configured values for a key.
	ConfigGetAll(key string) ([]string, error)
	// ConfigSet sets a config key to a single value.
	ConfigSet(key, value string) error
}

// Maintainer provides repository maintenance operations.
type Maintainer interface {
	// GC runs repository garbage collection.
	GC() error
	// Prune prunes unreachable repository data.
	Prune() error
	// Fsck verifies repository object integrity.
	Fsck() error
	// Clean lists or removes untracked files and returns their paths. By default it performs a dry-run (equivalent to git clean -n); no files are deleted unless WithCleanForce(true) is passed.
	Clean(opts ...CleanOption) ([]string, error)
}
