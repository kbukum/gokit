package llm

import (
	"testing"
	"time"
)

func TestConfig_ApplyDefaults_EmptyDialect(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()

	if cfg.Timeout != 120*time.Second {
		t.Errorf("Timeout = %v, want 120s", cfg.Timeout)
	}
	if cfg.Name != "" {
		t.Errorf("Name = %q, want empty when dialect is empty", cfg.Name)
	}
}
