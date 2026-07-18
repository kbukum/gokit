package resilience

import (
	"context"
	"time"
)

// Await runs fn with a timeout-derived context and relies on fn honoring context cancellation.
// It does not spawn goroutines, so cancellation and timeout do not outlive the caller.
func Await[T any](ctx context.Context, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	if timeout <= 0 {
		return fn(ctx)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return fn(timeoutCtx)
}
