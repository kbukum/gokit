package di

import (
	"fmt"
	"reflect"
)

// Key is a type-parameterised handle for a DI registration. It binds a
// human-readable name to a concrete Go type T at compile time so that
// registration and resolution cannot disagree on the value type without a
// compiler error.
//
// Keys are values; create them with [NameKey] (typically as package-level
// vars in a "names" or "wiring" package) and reuse them across providers
// and consumers.
//
//	var loggerKey = di.NameKey[*logger.Logger]("logger")
//
//	di.Provide(c, loggerKey, func() (*logger.Logger, error) { return logger.New(...) })
//	log, err := di.ResolveKey(c, loggerKey)
//
// See OSS-review issue F-013 / #43.
type Key[T any] struct {
	name string
}

// NameKey returns a typed key for the given name. The type parameter T fixes
// the value type of every Provide/Resolve that uses this key.
func NameKey[T any](name string) Key[T] { return Key[T]{name: name} }

// Name returns the underlying string name of the key. Useful for error
// messages and introspection only; consumers should use [ResolveKey], not
// raw string lookups, for type safety.
func (k Key[T]) Name() string { return k.name }

// fullKey returns the unique container key for k, qualified by the concrete
// element type of T. Two Key[T]s with different T but the same name produce
// distinct container slots, so a Key[*Foo]("svc") and Key[*Bar]("svc")
// coexist without collision.
func (k Key[T]) fullKey() string {
	t := reflect.TypeOf((*T)(nil)).Elem()
	return t.String() + ":" + k.name
}

// Provide registers a constructor for k. The constructor must return either
// (T) or (T, error); registration fails if the constructor does not produce
// the expected type. Component initialisation is lazy (matching the existing
// [Container.Register] semantics).
func Provide[T any](c Container, k Key[T], ctor any) error {
	if c == nil {
		return fmt.Errorf("di: container is nil")
	}
	if ctor == nil {
		return fmt.Errorf("di: constructor for %s must not be nil", k.fullKey())
	}
	if err := validateCtor[T](ctor); err != nil {
		return fmt.Errorf("di: provide %s: %w", k.fullKey(), err)
	}
	return c.Register(k.fullKey(), ctor)
}

// ProvideSingleton registers a pre-constructed value for k.
func ProvideSingleton[T any](c Container, k Key[T], v T) error {
	if c == nil {
		return fmt.Errorf("di: container is nil")
	}
	return c.RegisterSingleton(k.fullKey(), v)
}

// ProvideTransient registers a constructor for k that creates a new instance
// on every [ResolveKey] call. The result is never cached.
func ProvideTransient[T any](c Container, k Key[T], ctor any) error {
	if c == nil {
		return fmt.Errorf("di: container is nil")
	}
	if ctor == nil {
		return fmt.Errorf("di: constructor for %s must not be nil", k.fullKey())
	}
	if err := validateCtor[T](ctor); err != nil {
		return fmt.Errorf("di: provide transient %s: %w", k.fullKey(), err)
	}
	return c.RegisterTransient(k.fullKey(), ctor)
}

// ResolveKey resolves k from c and returns the typed value. It returns an
// error if the key is not registered or if the resolved value's type does
// not match T (which can only happen if the container was modified through
// the legacy untyped API).
func ResolveKey[T any](c Container, k Key[T]) (T, error) {
	var zero T
	if c == nil {
		return zero, fmt.Errorf("di: container is nil")
	}
	v, err := c.Resolve(k.fullKey())
	if err != nil {
		return zero, fmt.Errorf("di: resolve %s: %w", k.fullKey(), err)
	}
	out, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("di: %s is %T, expected %T", k.fullKey(), v, zero)
	}
	return out, nil
}

// MustResolveKey is the panic-on-error variant of [ResolveKey]. Reserved
// for application startup / init / test / CLI use; do not call from
// request-scoped code.
func MustResolveKey[T any](c Container, k Key[T]) T {
	v, err := ResolveKey(c, k)
	if err != nil {
		panic(err)
	}
	return v
}

// validateCtor checks that ctor is a function returning either T or
// (T, error), accepting either no arguments, a single context.Context, or a
// single Container — matching what [UnifiedContainer.callConstructor]
// understands.
func validateCtor[T any](ctor any) error {
	fn := reflect.TypeOf(ctor)
	if fn == nil || fn.Kind() != reflect.Func {
		return fmt.Errorf("constructor must be a function, got %T", ctor)
	}
	switch fn.NumOut() {
	case 1:
		// must return T (or assignable)
	case 2:
		// must return (T, error)
		errIface := reflect.TypeOf((*error)(nil)).Elem()
		if !fn.Out(1).Implements(errIface) {
			return fmt.Errorf("second return must be error, got %s", fn.Out(1))
		}
	default:
		return fmt.Errorf("constructor must return (T) or (T, error), got %d returns", fn.NumOut())
	}
	want := reflect.TypeOf((*T)(nil)).Elem()
	got := fn.Out(0)
	if !got.AssignableTo(want) {
		return fmt.Errorf("constructor returns %s, not assignable to %s", got, want)
	}
	return nil
}
