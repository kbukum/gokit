package git

// Differ provides diff and working tree status operations.
type Differ interface {
	// Diff returns file changes between two refs (branches, tags, or SHAs).
	Diff(from, to string) ([]DiffEntry, error)
	// DiffStats returns aggregated statistics for changes between two refs.
	DiffStats(from, to string) (DiffStats, error)
	// Status returns the working tree status (staged, unstaged, untracked files).
	Status() ([]StatusEntry, error)
}

// TreeReader provides read access to git tree objects.
type TreeReader interface {
	// TreeHash returns the OID of the tree at the given path and revision.
	TreeHash(revision, path string) (TreeHash, error)
	// FileAt returns the content of a file at the given revision and path.
	FileAt(revision, path string) ([]byte, error)
	// ListEntries returns the entries in a tree at the given revision and path.
	ListEntries(revision, path string) ([]TreeEntry, error)
}

// LogReader provides commit log traversal.
type LogReader interface {
	// Log returns commits matching the supplied traversal options.
	Log(opts LogOptions) ([]Commit, error)
	// MergeBase returns a merge base for a and b.
	MergeBase(a, b string) (Oid, error)
	// IsAncestor reports whether a is an ancestor of b.
	IsAncestor(a, b string) (bool, error)
}

// Blamer provides per-line commit attribution.
type Blamer interface {
	// Blame returns line-level attribution for a file at a revision.
	Blame(revision, path string, opts ...BlameOption) ([]BlameLine, error)
}

// Inspector provides advanced read-only inspection helpers.
type Inspector interface {
	// Describe resolves a human-readable name for a revision.
	Describe(revision string) (string, error)
	// RevParse resolves a revision expression to an object ID.
	RevParse(spec string) (Oid, error)
	// Grep searches tracked content for pattern matches.
	Grep(pattern string, paths ...string) ([]GrepMatch, error)
	// Show returns raw object or file content for a revision expression.
	Show(spec string) ([]byte, error)
}
