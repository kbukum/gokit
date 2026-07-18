package di

import (
	"context"
	"fmt"
)

// Option configures a registration or resolution.
type Option func(*options)

type options struct {
	name string
}

// WithName qualifies a registration or lookup with a name, so that multiple values of the same type can coexist in one container. When omitted, a value is keyed by its type alone.
func WithName(name string) Option {
	return func(o *options) { o.name = name }
}

func buildOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// Register registers a pre-built value for type T. The value is returned as-is on every [Resolve]. Re-registering the same key replaces the prior entry. Register does not close the value; use [RegisterCloseable] for a resource whose cleanup the container should own.
func Register[T any](c *Container, value T, opts ...Option) error {
	if c == nil {
		return fmt.Errorf("di: container is nil")
	}
	o := buildOptions(opts)
	c.put(keyFor[T](o.name), &entry{
		mode:        modeEager,
		typeName:    typeName[T](),
		name:        o.name,
		value:       value,
		initialized: true,
	})
	return nil
}

// RegisterSingleton registers a factory for type T that is invoked once on first [Resolve]; the result is cached and returned on every subsequent resolve. The factory resolves its own dependencies from c using the passed [context.Context]. The cached value is not closed by [Container.Close]; use [RegisterSingletonCloseable] for a resource whose cleanup the container should own.
func RegisterSingleton[T any](c *Container, ctor func(context.Context) (T, error), opts ...Option) error {
	if c == nil {
		return fmt.Errorf("di: container is nil")
	}
	o := buildOptions(opts)
	if ctor == nil {
		return fmt.Errorf("di: constructor for %s must not be nil", keyFor[T](o.name))
	}
	c.put(keyFor[T](o.name), &entry{
		mode:     modeSingleton,
		typeName: typeName[T](),
		name:     o.name,
		factory:  wrap(ctor),
	})
	return nil
}

// RegisterTransient registers a factory for type T that is invoked fresh on every [Resolve]; the result is never cached. The factory resolves its own dependencies from c using the passed [context.Context].
func RegisterTransient[T any](c *Container, ctor func(context.Context) (T, error), opts ...Option) error {
	if c == nil {
		return fmt.Errorf("di: container is nil")
	}
	o := buildOptions(opts)
	if ctor == nil {
		return fmt.Errorf("di: constructor for %s must not be nil", keyFor[T](o.name))
	}
	c.put(keyFor[T](o.name), &entry{
		mode:     modeTransient,
		typeName: typeName[T](),
		name:     o.name,
		factory:  wrap(ctor),
	})
	return nil
}

func wrap[T any](ctor func(context.Context) (T, error)) func(context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		v, err := ctor(ctx)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
}

func typeName[T any]() string {
	return keyFor[T]("").typ.String()
}
