package provider

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
)

// Selector picks a provider from the available options.
type Selector[T Provider] interface {
	Select(ctx context.Context, providers map[string]T) (T, error)
}

// PrioritySelector tries providers in the given priority order
// and returns the first one that is available.
type PrioritySelector[T Provider] struct {
	// Priority is the ordered list of provider names to try.
	Priority []string
}

// Select returns the first available provider in priority order.
func (s *PrioritySelector[T]) Select(ctx context.Context, providers map[string]T) (T, error) {
	for _, name := range s.Priority {
		if p, ok := providers[name]; ok && p.IsAvailable(ctx) {
			return p, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("no available provider found in priority list")
}

// RoundRobinSelector distributes requests across providers.
type RoundRobinSelector[T Provider] struct {
	counter atomic.Uint64
}

// Select picks the next provider using round-robin distribution.
func (s *RoundRobinSelector[T]) Select(ctx context.Context, providers map[string]T) (T, error) {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		var zero T
		return zero, fmt.Errorf("no providers available")
	}

	n := len(names)
	start := int(s.counter.Add(1) - 1)
	for i := range n {
		idx := (start + i) % n
		p := providers[names[idx]]
		if p.IsAvailable(ctx) {
			return p, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("no available provider found")
}

// HealthCheckSelector picks the first available provider by calling IsAvailable.
type HealthCheckSelector[T Provider] struct{}

// Select returns the first provider that reports as available.
func (s *HealthCheckSelector[T]) Select(ctx context.Context, providers map[string]T) (T, error) {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if p := providers[name]; p.IsAvailable(ctx) {
			return p, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("no available provider found")
}
