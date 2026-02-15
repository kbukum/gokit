package llm

import "github.com/skillsenselab/gokit/provider"

// NewRegistry creates a new provider registry for LLM providers.
func NewRegistry() *provider.Registry[Provider] {
	return provider.NewRegistry[Provider]()
}

// ManagerOption configures the LLM provider manager.
type ManagerOption func(*managerConfig)

type managerConfig struct {
	selector provider.Selector[Provider]
}

// WithSelector sets the provider selection strategy for the manager.
func WithSelector(s provider.Selector[Provider]) ManagerOption {
	return func(c *managerConfig) {
		c.selector = s
	}
}

// NewManager creates a new provider manager for LLM providers.
func NewManager(opts ...ManagerOption) *provider.Manager[Provider] {
	cfg := &managerConfig{
		selector: &provider.HealthCheckSelector[Provider]{},
	}
	for _, o := range opts {
		o(cfg)
	}
	return provider.NewManager(NewRegistry(), cfg.selector)
}
