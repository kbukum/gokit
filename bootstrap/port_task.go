package bootstrap

import (
	"context"
	"fmt"
	"sync"

	apperrors "github.com/kbukum/gokit/errors"
)

// RunPortTask runs a finite task, such as an operator command, against the value a module provides for port p. It adds a module that needs p, then runs the App with [App.RunTask] and passes the provided value to task, so the task uses the same composition, components and lifecycle as a served process. A missing or duplicate provider fails startup with a [*ModuleError] before task runs; task and teardown failures are reported as RunTask reports them. It returns [ErrLifecycleUsed] when the App can no longer accept modules.
func RunPortTask[C Config, T any](ctx context.Context, app *App[C], p *Port[T], task func(context.Context, T) error) error {
	if app == nil {
		return apperrors.InvalidInput("bootstrap.app", "app is required")
	}
	if p == nil {
		return apperrors.InvalidInput("bootstrap.port", "port is required")
	}
	if task == nil {
		return apperrors.InvalidInput("bootstrap.task", "task is required")
	}
	consumer := &portTask[T]{port: p}
	if err := app.Use(consumer); err != nil {
		return err
	}
	return app.RunTask(ctx, func(ctx context.Context) error {
		value, ok := consumer.get()
		if !ok {
			return fmt.Errorf("bootstrap: %s was not provided", p.Ref())
		}
		return task(ctx, value)
	})
}

// portTask is the module RunPortTask adds; it reads its port during Register.
type portTask[T any] struct {
	port *Port[T]

	mu    sync.Mutex
	done  bool
	value T
}

func (m *portTask[T]) Spec() ModuleSpec {
	return ModuleSpec{Name: fmt.Sprintf("task %s@%p", m.port.Name(), m.port), Needs: []PortRef{m.port.Ref()}}
}

func (m *portTask[T]) Register(_ context.Context, mc *ModuleContext) error {
	v, err := Need(mc, m.port)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.value, m.done = v, true
	return nil
}

func (m *portTask[T]) get() (T, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.value, m.done
}
