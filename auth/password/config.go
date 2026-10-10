package password

import (
	"fmt"

	apperrors "github.com/kbukum/gokit/errors"
)

// Algorithm represents supported password hashing algorithms.
type Algorithm string

const (
	// AlgorithmArgon2id is argon2id hashing, the default.
	AlgorithmArgon2id Algorithm = "argon2id"

	// AlgorithmBcrypt is bcrypt hashing, for verifying and issuing hashes in systems that already use it.
	AlgorithmBcrypt Algorithm = "bcrypt"
)

const (
	defaultMinLength = 15
	defaultMaxLength = 128
	bcryptMaxBytes   = 72
	maxLength        = 1024
	minArgon2Memory  = 64 * 1024
	maxArgon2Memory  = 256 * 1024
	minArgon2Time    = 3
	maxArgon2Time    = 6
)

// Config configures password hashing. Loadable from YAML/env via mapstructure tags.
type Config struct {
	// Algorithm selects the hashing algorithm (default: "argon2id").
	Algorithm Algorithm `mapstructure:"algorithm"`

	// BcryptCost is the bcrypt cost parameter (default: 12, range: 12-14).
	BcryptCost int `mapstructure:"bcrypt_cost"`

	// Argon2Time is the number of argon2id iterations (default: 3, range: 3-6).
	Argon2Time uint32 `mapstructure:"argon2_time"`

	// Argon2Memory is the argon2id memory in KiB (default: 65536 = 64 MiB, at most 256 MiB).
	Argon2Memory uint32 `mapstructure:"argon2_memory"`

	// Argon2Threads is the argon2id parallelism (default: 4).
	Argon2Threads uint8 `mapstructure:"argon2_threads"`

	// MinLength is the minimum password length in characters enforced by Hash (default: 15, per NIST SP 800-63B-4
	// for single-factor passwords).
	MinLength int `mapstructure:"min_length"`

	// MaxLength is the maximum password length in characters (default: 128, or 72 for bcrypt, at most 1024). Hash
	// rejects longer passwords; Verify reports them as a mismatch without hashing.
	MaxLength int `mapstructure:"max_length"`
	// VerifyLimits bounds persisted hash costs. Zero fields default to the issuing parameters.
	VerifyLimits VerifyLimits `mapstructure:"verify_limits"`
}

// ApplyDefaults sets defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Algorithm == "" {
		c.Algorithm = AlgorithmArgon2id
	}
	if c.BcryptCost == 0 {
		c.BcryptCost = 12
	}
	if c.Argon2Time == 0 {
		c.Argon2Time = minArgon2Time
	}
	if c.Argon2Memory == 0 {
		c.Argon2Memory = minArgon2Memory
	}
	if c.Argon2Threads == 0 {
		c.Argon2Threads = 4
	}
	if c.MinLength == 0 {
		c.MinLength = defaultMinLength
	}
	if c.MaxLength == 0 {
		c.MaxLength = defaultMaxLength
		if c.Algorithm == AlgorithmBcrypt {
			c.MaxLength = bcryptMaxBytes
		}
	}
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	switch c.Algorithm {
	case AlgorithmArgon2id:
		if c.Argon2Time < minArgon2Time || c.Argon2Time > maxArgon2Time {
			return apperrors.InvalidInput("argon2_time", "argon2_time must be from 3 to 6")
		}
		if c.Argon2Memory < minArgon2Memory || c.Argon2Memory > maxArgon2Memory {
			return apperrors.InvalidInput("argon2_memory", "argon2_memory must be from 65536 to 262144 KiB")
		}
		if c.Argon2Threads < 1 || c.Argon2Threads > 8 {
			return apperrors.InvalidInput("argon2_threads", "argon2_threads must be from 1 to 8")
		}
	case AlgorithmBcrypt:
		if c.BcryptCost < 12 || c.BcryptCost > 14 {
			return apperrors.InvalidInput("bcrypt_cost", "bcrypt_cost must be from 12 to 14")
		}
		if c.MaxLength > bcryptMaxBytes {
			return apperrors.InvalidInput("max_length", "bcrypt max_length must not exceed 72")
		}
	default:
		return apperrors.InvalidInput("algorithm", "algorithm must be argon2id or bcrypt")
	}
	if c.MinLength < 1 || c.MinLength > maxLength {
		return apperrors.InvalidInput("min_length", "min_length must be from 1 to 1024")
	}
	if c.MaxLength < c.MinLength || c.MaxLength > maxLength {
		return apperrors.InvalidInput("max_length", "max_length must be from min_length to 1024")
	}
	return nil
}

// String describes the policy without parameters useful only to an attacker.
func (c Config) String() string {
	return fmt.Sprintf("%s(min=%d,max=%d)", c.Algorithm, c.MinLength, c.MaxLength)
}

// NewHasher applies defaults, validates cfg and returns the configured Hasher.
func NewHasher(cfg Config) (Hasher, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	limits := cfg.VerifyLimits
	limits.applyDefaults(cfg)
	if err := limits.validate(cfg); err != nil {
		return nil, err
	}
	policy := lengthPolicy{min: cfg.MinLength, max: cfg.MaxLength}
	if cfg.Algorithm == AlgorithmBcrypt {
		return &bcryptHasher{lengthPolicy: policy, cost: cfg.BcryptCost, limits: limits}, nil
	}
	return &argon2Hasher{lengthPolicy: policy, time: cfg.Argon2Time, memory: cfg.Argon2Memory, threads: cfg.Argon2Threads, limits: limits}, nil
}
