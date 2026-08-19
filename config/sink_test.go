package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInMemoryConfigSink(t *testing.T) {
	t.Parallel()
	sink := NewInMemoryConfigSink()
	if !sink.IsEmpty() {
		t.Fatalf("new sink should be empty")
	}
	if err := sink.Set("api.key", NewSecretString("secret-1")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := sink.SetMany([]ConfigEntry{
		{Key: "db.password", Value: NewSecretString("pw")},
		{Key: "api.token", Value: NewSecretString("tok")},
	}); err != nil {
		t.Fatalf("SetMany: %v", err)
	}
	if sink.Len() != 3 {
		t.Fatalf("expected 3 keys, got %d", sink.Len())
	}
	value, ok := sink.Get("api.key")
	if !ok || value.Value() != "secret-1" {
		t.Fatalf("Get api.key = %q, %v", value.Value(), ok)
	}
	if got := sink.Keys(); strings.Join(got, ",") != "api.key,api.token,db.password" {
		t.Fatalf("Keys not sorted: %v", got)
	}
	if err := sink.Remove("api.key"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := sink.Get("api.key"); ok {
		t.Fatalf("api.key should be removed")
	}
	if err := sink.Remove("missing"); err != nil {
		t.Fatalf("Remove missing should be a no-op: %v", err)
	}
}

func TestInMemoryConfigSinkDebugDoesNotLeak(t *testing.T) {
	t.Parallel()
	sink := NewInMemoryConfigSink()
	_ = sink.Set("api.key", NewSecretString("top-secret"))
	value, _ := sink.Get("api.key")
	if strings.Contains(value.String(), "top-secret") {
		t.Fatalf("masked string leaked the secret: %q", value.String())
	}
}

func TestFileConfigSinkRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.toml")
	sink := NewFileConfigSink(path)

	if err := sink.Set("api.key", NewSecretString("secret-1")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := sink.SetMany([]ConfigEntry{
		{Key: "db_password", Value: NewSecretString("pw")},
		{Key: "api_token", Value: NewSecretString("tok")},
	}); err != nil {
		t.Fatalf("SetMany: %v", err)
	}

	reopened := NewFileConfigSink(path)
	table, err := reopened.readTable()
	if err != nil {
		t.Fatalf("readTable: %v", err)
	}
	if table["api.key"] != "secret-1" || table["db_password"] != "pw" || table["api_token"] != "tok" {
		t.Fatalf("round-trip mismatch: %#v", table)
	}

	if err := reopened.Remove("api.key"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	table, err = reopened.readTable()
	if err != nil {
		t.Fatalf("readTable after remove: %v", err)
	}
	if _, ok := table["api.key"]; ok {
		t.Fatalf("api.key should be removed from file")
	}
}

func TestFileConfigSinkMissingFileReadsEmpty(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "absent.toml")
	sink := NewFileConfigSink(path)
	table, err := sink.readTable()
	if err != nil {
		t.Fatalf("readTable on missing file: %v", err)
	}
	if len(table) != 0 {
		t.Fatalf("missing file should read as empty, got %#v", table)
	}
}

func TestFileConfigSinkSharedLockConcurrentWrites(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.toml")
	sink := NewFileConfigSink(path)
	clone := &FileConfigSink{path: sink.path, codec: sink.codec, mu: sink.mu}

	done := make(chan error, 2)
	go func() { done <- sink.Set("a", NewSecretString("1")) }()
	go func() { done <- clone.Set("b", NewSecretString("2")) }()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("concurrent Set: %v", err)
		}
	}
	table, err := sink.readTable()
	if err != nil {
		t.Fatalf("readTable: %v", err)
	}
	if table["a"] != "1" || table["b"] != "2" {
		t.Fatalf("shared-lock writers lost an update: %#v", table)
	}
}
