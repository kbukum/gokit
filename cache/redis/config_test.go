package redis

import (
	"strings"
	"testing"
)

func TestConfigDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	cfg := Config{Enabled: true, Addr: "127.0.0.1:6379"}
	cfg.ApplyDefaults()
	if cfg.PoolSize != 10 || cfg.MinIdleConns != 2 || cfg.MaxRetries != 3 {
		t.Fatalf("pool defaults = size %d min idle %d retries %d", cfg.PoolSize, cfg.MinIdleConns, cfg.MaxRetries)
	}
	if cfg.MinRetryBackoff != "8ms" || cfg.MaxRetryBackoff != "512ms" {
		t.Fatalf("retry backoff defaults = %q %q", cfg.MinRetryBackoff, cfg.MaxRetryBackoff)
	}
	if cfg.DialTimeout != "5s" || cfg.ReadTimeout != "3s" || cfg.WriteTimeout != "3s" {
		t.Fatalf("timeout defaults = %q %q %q", cfg.DialTimeout, cfg.ReadTimeout, cfg.WriteTimeout)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate defaulted config: %v", err)
	}
}

func TestConfigValidateDisabledSkipsRequiredFields(t *testing.T) {
	t.Parallel()

	if err := (&Config{}).Validate(); err != nil {
		t.Fatalf("Validate disabled config: %v", err)
	}
}

func TestConfigValidateRejectsInvalidSettings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "missing addr", cfg: Config{Enabled: true}, want: "addr"},
		{name: "pool size", cfg: Config{Enabled: true, Addr: "x", PoolSize: -1}, want: "pool_size"},
		{name: "dial timeout", cfg: Config{Enabled: true, Addr: "x", PoolSize: 1, DialTimeout: "bad"}, want: "dial_timeout"},
		{name: "read timeout", cfg: Config{Enabled: true, Addr: "x", PoolSize: 1, DialTimeout: "1s", ReadTimeout: "bad"}, want: "read_timeout"},
		{name: "write timeout", cfg: Config{Enabled: true, Addr: "x", PoolSize: 1, DialTimeout: "1s", ReadTimeout: "1s", WriteTimeout: "bad"}, want: "write_timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.cfg.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate error = %v, want containing %q", err, tc.want)
			}
		})
	}
}
