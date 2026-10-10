package password

import (
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestConfigDefaultsFollowBaseline(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.ApplyDefaults()
	want := Config{Algorithm: AlgorithmArgon2id, BcryptCost: 12, Argon2Time: 3, Argon2Memory: 64 * 1024, Argon2Threads: 4, MinLength: 15, MaxLength: 128}
	if cfg != want {
		t.Fatalf("defaults = %+v, want %+v", cfg, want)
	}
	bcryptCfg := Config{Algorithm: AlgorithmBcrypt}
	bcryptCfg.ApplyDefaults()
	if bcryptCfg.MaxLength != 72 {
		t.Fatalf("bcrypt MaxLength = %d, want 72", bcryptCfg.MaxLength)
	}
}

func TestNewHasherValidatesConfig(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]Config{
		"algorithm":        {Algorithm: "md5"},
		"bcrypt cost":      {Algorithm: AlgorithmBcrypt, BcryptCost: 11},
		"bcrypt max":       {Algorithm: AlgorithmBcrypt, MaxLength: 73},
		"argon2 time":      {Argon2Time: 2},
		"argon2 memory":    {Argon2Memory: 64*1024 - 1},
		"argon2 memory up": {Argon2Memory: maxArgon2Memory + 1},
		"argon2 time up":   {Argon2Time: maxArgon2Time + 1},
		"min length":       {MinLength: -1},
		"max below min":    {MinLength: 20, MaxLength: 19},
		"max length":       {MaxLength: maxLength + 1},
		"verify envelope":  {VerifyLimits: VerifyLimits{EncodedBytes: 255}},
		"verify threads":   {VerifyLimits: VerifyLimits{Argon2Threads: 9}},
		"bcrypt above verify": {
			Algorithm: AlgorithmBcrypt, BcryptCost: 13, VerifyLimits: VerifyLimits{BcryptCost: 12},
		},
		"argon2 time above verify":    {Argon2Time: 4, VerifyLimits: VerifyLimits{Argon2Time: 3}},
		"argon2 memory above verify":  {Argon2Memory: 2 * minArgon2Memory, VerifyLimits: VerifyLimits{Argon2Memory: minArgon2Memory}},
		"argon2 threads above verify": {Argon2Threads: 4, VerifyLimits: VerifyLimits{Argon2Threads: 2}},
	} {
		if _, err := NewHasher(cfg); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestConfigRedactedDescription(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.ApplyDefaults()
	if got := cfg.String(); got != "argon2id(min=15,max=128)" || strings.Contains(got, "%!") {
		t.Fatalf("String = %q", got)
	}
}
