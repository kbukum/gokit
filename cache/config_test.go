package cache

import "testing"

func TestConfigDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	cfg.ApplyDefaults()
	if cfg.Provider != ProviderMemory {
		t.Fatalf("Provider default = %q", cfg.Provider)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate defaulted config: %v", err)
	}

	cfg = Config{DefaultTTL: -1}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted negative default TTL")
	}
}

func TestConfigValidateRequiresProvider(t *testing.T) {
	t.Parallel()

	if err := (&Config{}).Validate(); err == nil {
		t.Fatal("Validate accepted empty provider")
	}
}
