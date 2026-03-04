package client

// Resolver resolves a service name to a base URL (e.g., "http://host:port").
// Implement this interface to integrate with service discovery.
type Resolver interface {
	Resolve(serviceName string) (string, error)
}

// ResolveFunc adapts a plain function to the Resolver interface.
//
// Usage with a discovery-based factory:
//
//	resolver := client.ResolveFunc(factory.BaseURL)
type ResolveFunc func(serviceName string) (string, error)

// Resolve calls the underlying function.
func (f ResolveFunc) Resolve(serviceName string) (string, error) {
	return f(serviceName)
}

// StaticResolver returns a fixed base URL regardless of service name.
// Useful for testing or direct connections without service discovery.
func StaticResolver(baseURL string) Resolver {
	return ResolveFunc(func(_ string) (string, error) {
		return baseURL, nil
	})
}
