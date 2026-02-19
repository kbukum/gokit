package provider

import "context"

// Provider is the base interface all providers must implement.
type Provider interface {
	// Name returns the provider's unique name.
	Name() string
	// IsAvailable checks if the provider is ready to handle requests.
	IsAvailable(ctx context.Context) bool
}

// Factory creates a provider instance from configuration.
type Factory[T Provider] func(cfg map[string]any) (T, error)
