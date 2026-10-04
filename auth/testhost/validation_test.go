package testhost

import (
	"errors"
	"os"
	"testing"
)

func TestMissingPrivateFixtureFailsBeforeStateAcquisition(t *testing.T) {
	t.Parallel()
	cfg := hostConfig(t)
	if _, err := New(t.Context(), cfg, Fixture{}); err == nil {
		t.Fatal("missing private fixture acquired an environment")
	}
	if _, err := os.Stat(cfg.StateFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid fixture created database state")
	}
	cfg.Origin = "http://localhost:0"
	if _, err := New(t.Context(), cfg, Fixture{}); err == nil {
		t.Fatal("insecure configuration acquired an environment")
	}
}
