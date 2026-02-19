// Package discovery provides service discovery abstractions for gokit applications.
//
// It defines interfaces and types for dynamically discovering healthy service
// instances from registries such as Consul or static configuration. The package
// follows gokit's component pattern with lifecycle management and health checks.
//
// # Architecture
//
//   - Client: Resolves service instances by name with health filtering
//   - Registry: Manages service registration and deregistration
//   - Strategy: Selects an instance from available candidates (e.g., round-robin)
//
// # Backends
//
//   - discovery/consul: HashiCorp Consul service discovery
//   - discovery/static: Static list of endpoints for development/testing
package discovery
