package config

import (
	"os"
	"path/filepath"
	"testing"
)

type strictSample struct {
	Name    string `json:"name"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
}

func TestLoadStrictAcceptsKnownKeys(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.toml")
	if err := os.WriteFile(path, []byte("name = \"svc\"\nport = 8080\nenabled = true\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := LoadStrict[strictSample](path)
	if err != nil {
		t.Fatalf("LoadStrict: %v", err)
	}
	if got.Name != "svc" || got.Port != 8080 || !got.Enabled {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestLoadStrictRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.toml")
	if err := os.WriteFile(path, []byte("name = \"svc\"\nport = 8080\nmystery = 1\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadStrict[strictSample](path); err == nil {
		t.Fatalf("expected an error for unknown key")
	}
}

func TestLoadStrictNoCodec(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.unknownext")
	if err := os.WriteFile(path, []byte("name = \"svc\""), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadStrict[strictSample](path); err == nil {
		t.Fatalf("expected an error for missing codec")
	}
}
