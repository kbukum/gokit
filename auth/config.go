package auth

import (
	"fmt"

	"github.com/skillsenselab/gokit/auth/jwt"
	"github.com/skillsenselab/gokit/auth/oidc"
	"github.com/skillsenselab/gokit/auth/password"
)

// Config holds all authentication configuration.
// It composes subpackage configs for loading from YAML/env via mapstructure.
type Config struct {
	// Enabled controls whether authentication is active.
	Enabled bool `mapstructure:"enabled"`

	// JWT configures the JWT token service.
	JWT jwt.Config `mapstructure:"jwt"`

	// Password configures password hashing.
	Password password.Config `mapstructure:"password"`

	// OIDC configures OIDC provider verification (optional).
	OIDC oidc.Config `mapstructure:"oidc"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	c.JWT.ApplyDefaults()
	c.Password.ApplyDefaults()
	c.OIDC.ApplyDefaults()
}

// Validate checks all sub-configurations.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if err := c.JWT.Validate(); err != nil {
		return fmt.Errorf("auth.jwt: %w", err)
	}
	if err := c.Password.Validate(); err != nil {
		return fmt.Errorf("auth.password: %w", err)
	}
	if c.OIDC.Enabled {
		if err := c.OIDC.Validate(); err != nil {
			return fmt.Errorf("auth.oidc: %w", err)
		}
	}
	return nil
}
