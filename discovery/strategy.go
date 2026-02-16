package discovery

import "context"

// LoadBalancingStrategy defines how to select endpoints.
type LoadBalancingStrategy string

const (
	StrategyRandom     LoadBalancingStrategy = "random"
	StrategyRoundRobin LoadBalancingStrategy = "round_robin"
	StrategyWeighted   LoadBalancingStrategy = "weighted"
	StrategyLeastConn  LoadBalancingStrategy = "least_conn"
)

// Shorthand aliases for load balancing strategies.
const (
	Random     = StrategyRandom
	RoundRobin = StrategyRoundRobin
	Weighted   = StrategyWeighted
	LeastConn  = StrategyLeastConn
)

// Query defines parameters for a service discovery query.
type Query struct {
	ServiceName string
	Protocol    string
	Tags        []string
	Strategy    LoadBalancingStrategy
	HealthyOnly bool
}

// Criticality defines how important a service is during discovery.
type Criticality string

const (
	CriticalityRequired Criticality = "required"
	CriticalityOptional Criticality = "optional"
)

// DiscoveryClient defines the high-level service discovery interface.
// The concrete Client type implements this interface.
type DiscoveryClient interface {
	// Discover returns all healthy instances of a service, optionally filtered by protocol.
	Discover(ctx context.Context, serviceName string, protocol ...string) ([]ServiceInstance, error)

	// DiscoverOne returns a single instance selected by the query's load-balancing strategy.
	DiscoverOne(ctx context.Context, query Query) (ServiceInstance, error)

	// DiscoverAll returns instances for all configured services.
	DiscoverAll(ctx context.Context) (map[string][]ServiceInstance, error)

	// Invalidate clears cached entries for a service.
	Invalidate(serviceName string)

	// Close releases resources.
	Close() error
}
