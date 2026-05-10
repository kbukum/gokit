package git

// Repository provides core git repository operations.
type Repository interface {
	// Root returns the absolute path to the repository root.
	Root() string
	// Head returns the reference that HEAD points to.
	Head() (Reference, error)
	// ResolveRef resolves a ref name (branch, tag, or SHA prefix) to an OID.
	ResolveRef(refname string) (Oid, error)
	// IsDirty reports whether the working tree has uncommitted changes.
	IsDirty() (bool, error)
}

// Executor provides a raw git command escape hatch.
type Executor interface {
	// Exec runs the git CLI with the provided arguments in the repository root.
	Exec(args ...string) ([]byte, error)
}
