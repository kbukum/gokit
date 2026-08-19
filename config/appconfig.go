package config

// AppConfig is the contract every application config struct satisfies. It composes
// the programmatic-defaults and validation hooks with access to the embedded
// ServiceConfig, so the loader can apply defaults, validate, and read service-level
// settings through a single typed interface. A struct that embeds *ServiceConfig and
// implements ApplyDefaults and Validate satisfies it automatically, since
// ServiceConfig promotes GetServiceConfig.
type AppConfig interface {
	Defaultable
	Validatable
	// GetServiceConfig returns the embedded service configuration.
	GetServiceConfig() *ServiceConfig
}
