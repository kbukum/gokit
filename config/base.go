package config

import "fmt"

// BaseConfig contains essential fields that every service needs.
type BaseConfig struct {
	Name        string `yaml:"name" mapstructure:"name"`
	Environment string `yaml:"environment" mapstructure:"environment"`
	Version     string `yaml:"version" mapstructure:"version"`
	Debug       bool   `yaml:"debug" mapstructure:"debug"`
}

// ApplyDefaults applies default values to base configuration.
func (c *BaseConfig) ApplyDefaults() {
	if c.Environment == "" {
		c.Environment = "development"
	}
	if c.Environment == "development" {
		c.Debug = true
	}
}

// Validate validates base configuration.
func (c *BaseConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("base.name is required")
	}
	validEnvs := []string{"development", "staging", "production"}
	for _, v := range validEnvs {
		if c.Environment == v {
			return nil
		}
	}
	return fmt.Errorf("base.environment must be one of [development, staging, production] (got: %s)", c.Environment)
}
