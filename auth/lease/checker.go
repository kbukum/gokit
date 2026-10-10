package lease

import "context"

// Checker reports the current authority for a batch of distinct keys. A true value renews that key's leases and false
// ends them; a missing key is left to expire. Timely returned entries remain authoritative alongside a reporting error. Check must honor ctx,
// whose deadline is the check start plus the lease duration, but expiry does not depend on it doing so.
type Checker[K comparable] interface {
	Check(ctx context.Context, keys []K) (map[K]bool, error)
}

// CheckerFunc adapts a function to [Checker].
type CheckerFunc[K comparable] func(ctx context.Context, keys []K) (map[K]bool, error)

// Check implements [Checker].
func (f CheckerFunc[K]) Check(ctx context.Context, keys []K) (map[K]bool, error) { return f(ctx, keys) }
