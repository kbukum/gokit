package di

import "fmt"

// MustResolve resolves a component with type safety and panics on error.
// For use in tests and application startup only; never call from HTTP handlers or middleware.
func MustResolve[T any](c Container, key string) T {
	instance, err := c.Resolve(key)
	if err != nil {
		panic(fmt.Sprintf("di: failed to resolve %s: %v", key, err))
	}
	result, ok := instance.(T)
	if !ok {
		var zero T
		panic(fmt.Sprintf("di: component %s is %T, expected %T", key, instance, zero))
	}
	return result
}

// Resolve resolves a component with type safety and returns an error on failure.
func Resolve[T any](c Container, key string) (T, error) {
	var zero T
	instance, err := c.Resolve(key)
	if err != nil {
		return zero, fmt.Errorf("di: failed to resolve %s: %w", key, err)
	}
	result, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("di: component %s is %T, expected %T", key, instance, zero)
	}
	return result, nil
}

// ResolveOrError resolves a component with type safety and returns an error on failure.
func ResolveOrError[T any](c Container, key string) (T, error) {
	return Resolve[T](c, key)
}

// TryResolve resolves a component, returns zero value and false if not found.
func TryResolve[T any](c Container, key string) (T, bool) {
	var zero T
	instance, err := c.Resolve(key)
	if err != nil {
		return zero, false
	}
	result, ok := instance.(T)
	if !ok {
		return zero, false
	}
	return result, true
}
