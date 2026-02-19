package auth

import (
	"fmt"

	"github.com/kbukum/gokit/auth/jwt"
	"github.com/kbukum/gokit/auth/oidc"
	"github.com/kbukum/gokit/auth/password"
)

// Config holds all authentication configuration.
// It composes subpackage configs for loading from YAML/env via mapstructure.
// Sub-configs are pointers so unused features are nil and don't force
// unnecessary validation or defaults.
type Config struct {
	// Enabled controls whether authentication is active.
	Enabled bool `mapstructure:"enabled"`

	// JWT configures the JWT token service (nil if not used).
	JWT *jwt.Config `mapstructure:"jwt"`

	// Password configures password hashing (nil if not used).
	Password *password.Config `mapstructure:"password"`

	// OIDC configures OIDC provider verification (nil if not used).
	OIDC *oidc.Config `mapstructure:"oidc"`
}

// ApplyDefaults sets sensible defaults for non-nil sub-configurations.
func (c *Config) ApplyDefaults() {
	if c.JWT != nil {
		c.JWT.ApplyDefaults()
	}
	if c.Password != nil {
		c.Password.ApplyDefaults()
	}
	if c.OIDC != nil {
		c.OIDC.ApplyDefaults()
	}
}

// Validate checks all non-nil sub-configurations.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.JWT != nil {
		if err := c.JWT.Validate(); err != nil {
			return fmt.Errorf("auth.jwt: %w", err)
		}
	}
	if c.Password != nil {
		if err := c.Password.Validate(); err != nil {
			return fmt.Errorf("auth.password: %w", err)
		}
	}
	if c.OIDC != nil && c.OIDC.Enabled {
		if err := c.OIDC.Validate(); err != nil {
			return fmt.Errorf("auth.oidc: %w", err)
		}
	}
	return nil
}

// Describe returns a human-readable one-liner for the startup summary.
// Example: "JWT(HS256) TTL=15m0s password=bcrypt OIDC(issuer.com)"
func (c *Config) Describe() string {
	if !c.Enabled {
		return "disabled"
	}
	var line string
	if c.JWT != nil {
		line += fmt.Sprintf("JWT(%s) TTL=%s", c.JWT.Method, c.JWT.AccessTokenTTL)
	}
	if c.Password != nil {
		if line != "" {
			line += " "
		}
		line += fmt.Sprintf("password=%s", c.Password.Algorithm)
	}
	if c.OIDC != nil && c.OIDC.Enabled {
		if line != "" {
			line += " "
		}
		line += fmt.Sprintf("OIDC(%s)", c.OIDC.Issuer)
	}
	if line == "" {
		return "enabled (no providers configured)"
	}
	return line
}
