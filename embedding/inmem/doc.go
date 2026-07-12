// Package inmem provides a deterministic in-memory embedding provider for tests
// and local development.
//
// [New] returns a [Provider] that maps text to fixed-dimension vectors
// deterministically, so embedding-dependent code can be exercised without a
// network model or external service.
package inmem
