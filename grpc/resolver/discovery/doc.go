// Package discovery provides a gRPC name resolver backed by gokit's
// Discovery interface.  It enables dynamic service resolution — addresses
// are updated at runtime when Consul (or any other discovery backend)
// reports changes.
//
// Usage:
//
//	builder := discovery.NewResolverBuilder(discoveryProvider, logger, discovery.WithScheme("consul"))
//	conn, err := grpc.NewClient("consul:///my-service",
//	    grpc.WithResolvers(builder),
//	    grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
//	)
package discovery
