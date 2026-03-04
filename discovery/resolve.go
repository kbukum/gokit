package discovery

import (
	"context"
	"fmt"
	"time"
)

// ServiceResolver resolves service names to base URLs using service discovery.
// It wraps a DiscoveryClient with optional service allow-list validation
// and configurable URL scheme.
//
// ServiceResolver structurally satisfies connect/client.Resolver, so it can
// be used directly with ConnectRPC clients without importing the connect module:
//
//	resolver := discovery.NewServiceResolver(discClient, "svc-a", "svc-b")
//	url, err := resolver.Resolve("svc-a") // "http://host:port"
type ServiceResolver struct {
	client   DiscoveryClient
	services map[string]struct{}
	scheme   string
	strategy LoadBalancingStrategy
	timeout  time.Duration
}

// ResolverOption configures a ServiceResolver.
type ResolverOption func(*ServiceResolver)

// WithScheme sets the URL scheme (default: "http").
func WithScheme(scheme string) ResolverOption {
	return func(r *ServiceResolver) {
		r.scheme = scheme
	}
}

// WithStrategy sets the load balancing strategy (default: Random).
func WithStrategy(strategy LoadBalancingStrategy) ResolverOption {
	return func(r *ServiceResolver) {
		r.strategy = strategy
	}
}

// WithResolveTimeout sets the discovery timeout per Resolve call (default: 10s).
func WithResolveTimeout(d time.Duration) ResolverOption {
	return func(r *ServiceResolver) {
		r.timeout = d
	}
}

// NewServiceResolver creates a resolver that discovers services and returns base URLs.
// If services are provided, only those service names are allowed (acts as an allow-list).
// An empty services list allows any service name.
func NewServiceResolver(client DiscoveryClient, services []string, opts ...ResolverOption) *ServiceResolver {
	allowed := make(map[string]struct{}, len(services))
	for _, s := range services {
		allowed[s] = struct{}{}
	}

	r := &ServiceResolver{
		client:   client,
		services: allowed,
		scheme:   "http",
		strategy: Random,
		timeout:  10 * time.Second,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Resolve discovers a service and returns its base URL (e.g., "http://host:port").
// Returns an error if the service is not in the allow-list (when configured)
// or if discovery fails.
func (r *ServiceResolver) Resolve(serviceName string) (string, error) {
	if len(r.services) > 0 {
		if _, ok := r.services[serviceName]; !ok {
			return "", fmt.Errorf("service %q is not configured as a discoverable dependency", serviceName)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	endpoint, err := r.client.DiscoverOne(ctx, Query{
		ServiceName: serviceName,
		Strategy:    r.strategy,
		HealthyOnly: true,
	})
	if err != nil {
		return "", fmt.Errorf("discovery failed for service %q: %w", serviceName, err)
	}

	return fmt.Sprintf("%s://%s", r.scheme, endpoint.HostPort()), nil
}
