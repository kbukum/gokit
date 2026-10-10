package password

import apperrors "github.com/kbukum/gokit/errors"

// VerifyLimits is the supported persisted-hash envelope. Issuance must fit it; verification never starts KDF work outside it.
type VerifyLimits struct {
	EncodedBytes  int    `mapstructure:"encoded_bytes"`
	Argon2Memory  uint32 `mapstructure:"argon2_memory"`
	Argon2Time    uint32 `mapstructure:"argon2_time"`
	Argon2Threads uint8  `mapstructure:"argon2_threads"`
	BcryptCost    int    `mapstructure:"bcrypt_cost"`
}

func (l *VerifyLimits) applyDefaults(c Config) {
	if l.EncodedBytes == 0 {
		l.EncodedBytes = 512
	}
	if l.Argon2Memory == 0 {
		l.Argon2Memory = c.Argon2Memory
	}
	if l.Argon2Time == 0 {
		l.Argon2Time = c.Argon2Time
	}
	if l.Argon2Threads == 0 {
		l.Argon2Threads = c.Argon2Threads
	}
	if l.BcryptCost == 0 {
		l.BcryptCost = c.BcryptCost
	}
}

func (l VerifyLimits) validate(c Config) error {
	if l.EncodedBytes < 256 || l.EncodedBytes > 512 || l.Argon2Memory < 1 || l.Argon2Memory > maxArgon2Memory ||
		l.Argon2Time < 1 || l.Argon2Time > maxArgon2Time || l.Argon2Threads < 1 || l.Argon2Threads > 8 || l.BcryptCost < 4 || l.BcryptCost > 14 {
		return apperrors.InvalidInput("verify_limits", "verification limits exceed the supported hash envelope")
	}
	if c.Algorithm == AlgorithmBcrypt {
		if c.BcryptCost > l.BcryptCost {
			return apperrors.InvalidInput("verify_limits", "bcrypt issuance must fit verification limits")
		}
	} else if c.Argon2Memory > l.Argon2Memory || c.Argon2Time > l.Argon2Time || c.Argon2Threads > l.Argon2Threads {
		return apperrors.InvalidInput("verify_limits", "argon2 issuance must fit verification limits")
	}
	return nil
}

func unsupportedCost() error {
	return apperrors.New(apperrors.ErrCodeInternal, "Stored password hash exceeds supported verification cost").WithReason(ReasonUnsupportedCost)
}
