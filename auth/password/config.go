package password

import "fmt"

// Algorithm represents supported password hashing algorithms.
type Algorithm string

const (
	// AlgorithmBcrypt is bcrypt hashing (widely supported, recommended for compatibility).
	AlgorithmBcrypt Algorithm = "bcrypt"

	// AlgorithmArgon2id is argon2id hashing (modern, recommended for new projects).
	AlgorithmArgon2id Algorithm = "argon2id"
)

// Config configures password hashing behavior.
// Loadable from YAML/env via mapstructure tags.
type Config struct {
	// Algorithm selects the hashing algorithm (default: "bcrypt").
	Algorithm Algorithm `mapstructure:"algorithm"`

	// BcryptCost is the bcrypt cost parameter (default: 12, range: 4-31).
	// Only used when Algorithm is "bcrypt".
	BcryptCost int `mapstructure:"bcrypt_cost"`

	// Argon2Time is the number of iterations for argon2id (default: 1).
	Argon2Time uint32 `mapstructure:"argon2_time"`

	// Argon2Memory is the memory usage in KiB for argon2id (default: 65536 = 64MB).
	Argon2Memory uint32 `mapstructure:"argon2_memory"`

	// Argon2Threads is the parallelism for argon2id (default: 4).
	Argon2Threads uint8 `mapstructure:"argon2_threads"`

	// MinLength is the minimum password length (default: 8).
	MinLength int `mapstructure:"min_length"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Algorithm == "" {
		c.Algorithm = AlgorithmBcrypt
	}
	if c.BcryptCost == 0 {
		c.BcryptCost = 12
	}
	if c.Argon2Time == 0 {
		c.Argon2Time = 1
	}
	if c.Argon2Memory == 0 {
		c.Argon2Memory = 64 * 1024
	}
	if c.Argon2Threads == 0 {
		c.Argon2Threads = 4
	}
	if c.MinLength == 0 {
		c.MinLength = 8
	}
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	switch c.Algorithm {
	case AlgorithmBcrypt, AlgorithmArgon2id:
	default:
		return fmt.Errorf("unsupported algorithm: %s (use bcrypt or argon2id)", c.Algorithm)
	}
	if c.BcryptCost < 4 || c.BcryptCost > 31 {
		return fmt.Errorf("bcrypt_cost must be between 4 and 31 (got: %d)", c.BcryptCost)
	}
	if c.MinLength < 1 {
		return fmt.Errorf("min_length must be >= 1 (got: %d)", c.MinLength)
	}
	return nil
}

// NewHasher creates a Hasher from configuration.
// This is the config-driven factory — use it when loading from YAML/env.
func NewHasher(cfg Config) Hasher {
	cfg.ApplyDefaults()
	switch cfg.Algorithm {
	case AlgorithmArgon2id:
		return NewArgon2Hasher(
			WithArgon2Time(cfg.Argon2Time),
			WithArgon2Memory(cfg.Argon2Memory),
			WithArgon2Threads(cfg.Argon2Threads),
		)
	default:
		return NewBcryptHasher(WithCost(cfg.BcryptCost))
	}
}
