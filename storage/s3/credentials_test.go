package s3

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMountedCredentialsAreLoadedWithoutFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	access, secret := filepath.Join(dir, "access"), filepath.Join(dir, "secret")
	for path, value := range map[string]string{access: "fixture-access\n", secret: "fixture-secret\n"} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{Bucket: "fixture", AccessKeyFile: access, SecretKeyFile: secret}
	s, err := NewStorage(t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	got, err := s.client.Options().Credentials.Retrieve(t.Context())
	if err != nil || got.AccessKeyID != "fixture-access" || got.SecretAccessKey != "fixture-secret" {
		t.Fatal("mounted credentials were not selected")
	}
	cfg.SecretKeyFile = filepath.Join(dir, "missing")
	if s, err := NewStorage(t.Context(), &cfg); s != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing secret used fallback credentials: backend=%v error=%v", s != nil, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if s, err := NewStorage(ctx, &cfg); s != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled initialization: backend=%v error=%v", s != nil, err)
	}
}

func TestCredentialSourcesRejectMixedAndIncompleteBindings(t *testing.T) {
	t.Parallel()
	for _, cfg := range []Config{
		{Bucket: "fixture", AccessKeyFile: "/access"},
		{Bucket: "fixture", SecretKeyFile: "/secret"},
		{Bucket: "fixture", AccessKeyID: "inline", AccessKeyFile: "/access", SecretKeyFile: "/secret"},
		{Bucket: "fixture", AccessKeyID: "inline"},
		{Bucket: "fixture", SecretAccessKey: "inline"},
	} {
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err == nil {
			t.Fatal("mixed or incomplete credential source admitted")
		}
	}
}
