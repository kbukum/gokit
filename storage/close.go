package storage

import "context"

// Closer is implemented by backends with owned resources. Consumers must drain before closing them.
// A failed close retains ownership and must be safe to retry; nil confirms complete release.
type Closer interface {
	Close(context.Context) error
}
