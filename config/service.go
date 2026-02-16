package config

import (
	"fmt"

	"github.com/kbukum/gokit/logger"
)

// ServiceConfig contains the essential configuration fields every service needs.
// Projects extend this by embedding it in their own config structs.
//
// Example:
//
//	type MyConfig struct {
//	    gkconfig.ServiceConfig `yaml:",inline" mapstructure:",squash"`
//	    Database database.Config `yaml:"database" mapstructure:"database"`
//	}
type ServiceConfig struct {
	Name        string        `yaml:"name" mapstructure:"name"`
	Environment string        `yaml:"environment" mapstructure:"environment"`
	Version     string        `yaml:"version" mapstructure:"version"`
	Debug       bool          `yaml:"debug" mapstructure:"debug"`
	Logging     logger.Config `yaml:"logging" mapstructure:"logging"`
}

// GetServiceConfig returns the base ServiceConfig.
// When embedded in a larger config struct, this method is promoted
// so the embedding struct automatically satisfies the Config interface.
func (c *ServiceConfig) GetServiceConfig() *ServiceConfig {
	return c
}

// ApplyDefaults applies default values to the base configuration.
// Override this in embedding structs and call c.ServiceConfig.ApplyDefaults() first.
func (c *ServiceConfig) ApplyDefaults() {
	if c.Environment == "" {
		c.Environment = "development"
	}
	if c.Environment == "development" {
		c.Debug = true
	}
	// Propagate service name into logging so Init() uses the right tag.
	if c.Logging.ServiceName == "" && c.Name != "" {
		c.Logging.ServiceName = c.Name
	}
	c.Logging.ApplyDefaults()
}

// Validate validates the base configuration fields.
// Override this in embedding structs and call c.ServiceConfig.Validate() first.
func (c *ServiceConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("config.name is required")
	}
	validEnvs := []string{"development", "staging", "production"}
	found := false
	for _, v := range validEnvs {
		if c.Environment == v {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("config.environment must be one of [development, staging, production] (got: %s)", c.Environment)
	}
	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("config.logging: %w", err)
	}
	return nil
}
