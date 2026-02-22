package provider

import "context"

// Initializable is optionally implemented by providers that need setup
// before handling requests (e.g., dial gRPC, validate binary, warm cache).
// The Manager calls Init() automatically when initializing providers.
type Initializable interface {
	Init(ctx context.Context) error
}

// Closeable is optionally implemented by providers that hold resources
// requiring explicit cleanup (e.g., gRPC connection, daemon process, LDAP bind).
// The Manager calls Close() automatically during shutdown.
type Closeable interface {
	Close(ctx context.Context) error
}
