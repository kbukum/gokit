package storage

import "context"

// Checker verifies a backend's configured resources at startup and during health probes.
type Checker interface {
	Check(context.Context) error
}
