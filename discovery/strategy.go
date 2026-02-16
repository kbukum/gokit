package discovery

// LoadBalancingStrategy defines how to select endpoints.
type LoadBalancingStrategy string

const (
	StrategyRandom     LoadBalancingStrategy = "random"
	StrategyRoundRobin LoadBalancingStrategy = "round_robin"
	StrategyWeighted   LoadBalancingStrategy = "weighted"
	StrategyLeastConn  LoadBalancingStrategy = "least_conn"
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
