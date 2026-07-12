package di

import (
	"context"
	"fmt"
)

// Disposer releases the resources held by a registered value of type T. It is
// invoked by [Container.Close] with a context that bounds shutdown.
type Disposer[T any] func(ctx context.Context, value T) error

// RegisterCloseable registers a pre-built value for type T together with a
// disposer that [Container.Close] runs to release it. The value is returned
// as-is on every [Resolve].
//
// Unlike [Register], the container owns this value's cleanup. Re-registering the
// same key records an additional disposer, so a value replaced by a later
// registration is still closed.
func RegisterCloseable[T any](c *Container, value T, dispose Disposer[T], opts ...Option) error {
	if c == nil {
		return fmt.Errorf("di: container is nil")
	}
	if dispose == nil {
		return fmt.Errorf("di: disposer for %s must not be nil", typeName[T]())
	}
	o := buildOptions(opts)
	k := keyFor[T](o.name)
	c.put(k, &entry{
		mode:        modeEager,
		typeName:    typeName[T](),
		name:        o.name,
		value:       value,
		initialized: true,
	})
	c.addCloser(k, func(ctx context.Context) error { return dispose(ctx, value) })
	return nil
}

// RegisterSingletonCloseable registers a singleton factory for type T together
// with a disposer. The factory is invoked once on first [Resolve]; at that point
// the disposer is recorded and later run by [Container.Close] in reverse order
// of construction. An unresolved singleton constructs nothing and records no
// disposer, so nothing is closed for it.
func RegisterSingletonCloseable[T any](c *Container, ctor func(context.Context) (T, error), dispose Disposer[T], opts ...Option) error {
	if c == nil {
		return fmt.Errorf("di: container is nil")
	}
	if ctor == nil {
		return fmt.Errorf("di: constructor for %s must not be nil", typeName[T]())
	}
	if dispose == nil {
		return fmt.Errorf("di: disposer for %s must not be nil", typeName[T]())
	}
	o := buildOptions(opts)
	c.put(keyFor[T](o.name), &entry{
		mode:     modeSingleton,
		typeName: typeName[T](),
		name:     o.name,
		factory:  wrap(ctor),
		disposer: func(ctx context.Context, v any) error {
			value, ok := v.(T)
			if !ok {
				return fmt.Errorf("di: disposer for %s got %T", typeName[T](), v)
			}
			return dispose(ctx, value)
		},
	})
	return nil
}
