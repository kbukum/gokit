package connect

import (
	"strings"
	"testing"
)

func TestConfigApplyDefaults(t *testing.T) {
	var cfg Config
	cfg.ApplyDefaults()

	if cfg.SendMaxBytes != defaultMaxBytes {
		t.Fatalf("SendMaxBytes = %d, want %d", cfg.SendMaxBytes, defaultMaxBytes)
	}
	if cfg.ReadMaxBytes != defaultMaxBytes {
		t.Fatalf("ReadMaxBytes = %d, want %d", cfg.ReadMaxBytes, defaultMaxBytes)
	}
}

func TestConfigApplyDefaultsPreservesConfiguredValues(t *testing.T) {
	cfg := Config{Enabled: true, SendMaxBytes: 1024, ReadMaxBytes: 2048}
	cfg.ApplyDefaults()

	if !cfg.Enabled {
		t.Fatal("Enabled should be preserved")
	}
	if cfg.SendMaxBytes != 1024 {
		t.Fatalf("SendMaxBytes = %d, want 1024", cfg.SendMaxBytes)
	}
	if cfg.ReadMaxBytes != 2048 {
		t.Fatalf("ReadMaxBytes = %d, want 2048", cfg.ReadMaxBytes)
	}
}

func TestConfigValidateAcceptsZeroAndPositiveLimits(t *testing.T) {
	cases := []Config{
		{},
		{SendMaxBytes: 1},
		{ReadMaxBytes: 1},
		{SendMaxBytes: 1, ReadMaxBytes: 2},
	}

	for _, cfg := range cases {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(%+v) returned error: %v", cfg, err)
		}
	}
}

func TestConfigValidateRejectsNegativeLimits(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "send", cfg: Config{SendMaxBytes: -1}, want: "send_max_bytes"},
		{name: "read", cfg: Config{ReadMaxBytes: -1}, want: "read_max_bytes"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}
