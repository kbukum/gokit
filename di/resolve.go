package di

import "fmt"

// MustResolve resolves a component with type safety, panics on error.
// Use this in handlers when you need a dependency.
//
// Example:
//
//	botRepo := di.MustResolve[contracts.BotRepository](h.container, shareddi.Shared.BotRepository)
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

// Resolve resolves a component with type safety, returns error on failure.
// Use this when you want to handle resolution errors gracefully.
//
// Example:
//
//	botRepo, err := di.Resolve[contracts.BotRepository](c, shareddi.Shared.BotRepository)
//	if err != nil {
//	    return fmt.Errorf("failed to get bot repository: %w", err)
//	}
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

// TryResolve resolves a component, returns zero value and false if not found.
// Use this when a dependency is optional.
//
// Example:
//
//	if metrics, ok := di.TryResolve[MetricsClient](c, "metrics"); ok {
//	    metrics.RecordEvent(...)
//	}
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
