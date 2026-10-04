package session

import (
	"context"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/resilience"
)

func admitted[T any](ctx context.Context, m *Manager, call func() (T, error)) (T, error) {
	return resilience.ExecuteWithResult(ctx, m.gate, func() (T, error) {
		var zero T
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if m.ctx.Err() != nil {
			return zero, auth.Failure("SESSION_CLOSED")
		}
		return call()
	})
}

type acquisition struct {
	lifetime context.Context
	release  func()
}
