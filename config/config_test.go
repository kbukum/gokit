package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseConfigApplyDefaults(t *testing.T) {
	t.Run("empty environment defaults to development", func(t *testing.T) {
		cfg := BaseConfig{Name: "svc"}
		cfg.ApplyDefaults()
		if cfg.Environment != "development" {
			t.Errorf("expected 'development', got %q", cfg.Environment)
		}
		if !cfg.Debug {
			t.Error("expected debug=true for development")
		}
	})

	t.Run("production environment keeps debug false", func(t *testing.T) {
		cfg := BaseConfig{Name: "svc", Environment: "production"}
		cfg.ApplyDefaults()
		if cfg.Debug {
			t.Error("expected debug=false for production")
		}
	})

	t.Run("development sets debug true", func(t *testing.T) {
		cfg := BaseConfig{Name: "svc", Environment: "development"}
		cfg.ApplyDefaults()
		if !cfg.Debug {
			t.Error("expected debug=true for development")
		}
	})
}

func TestBaseConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     BaseConfig
		wantErr bool
		errMsg  string
	}{
		{"valid development", BaseConfig{Name: "svc", Environment: "development"}, false, ""},
		{"valid staging", BaseConfig{Name: "svc", Environment: "staging"}, false, ""},
		{"valid production", BaseConfig{Name: "svc", Environment: "production"}, false, ""},
		{"missing name", BaseConfig{Environment: "production"}, true, "base.name is required"},
		{"invalid environment", BaseConfig{Name: "svc", Environment: "invalid"}, true, "base.environment must be one of"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadConfigWithYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yml")

	yamlContent := `
base:
  name: test-service
  environment: staging
  version: "1.0.0"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	type TestConfig struct {
		Base BaseConfig `yaml:"base" mapstructure:"base"`
	}

	var cfg TestConfig
	err := LoadConfig("test-service", &cfg, WithConfigFile(configPath))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Base.Name != "test-service" {
		t.Errorf("expected name 'test-service', got %q", cfg.Base.Name)
	}
	if cfg.Base.Environment != "staging" {
		t.Errorf("expected environment 'staging', got %q", cfg.Base.Environment)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	type TestConfig struct {
		Base BaseConfig `yaml:"base" mapstructure:"base"`
	}

	var cfg TestConfig
	// With no config file found, LoadConfig should still succeed (just empty config)
	err := LoadConfig("nonexistent-service", &cfg, WithConfigFile("/nonexistent/path.yml"))
	if err != nil {
		t.Fatalf("expected LoadConfig to succeed with missing file, got %v", err)
	}
}

func TestConfigResolverWithMockFS(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		"./cmd/my-svc/config.yml": true,
	}}
	resolver := &ConfigResolver{FileSystem: fs}
	files := resolver.ResolveFiles("my-svc", LoaderConfig{})
	if files.ConfigFile != "./cmd/my-svc/config.yml" {
		t.Errorf("expected config file at ./cmd/my-svc/config.yml, got %q", files.ConfigFile)
	}
}

type mockFS struct {
	files map[string]bool
}

func (m *mockFS) Exists(path string) bool    { return m.files[path] }
func (m *mockFS) LoadEnv(path string) error   { return nil }
func (m *mockFS) Getwd() (string, error)      { return "/mock", nil }

func TestWithFileSystemOption(t *testing.T) {
	var lc LoaderConfig
	fs := &mockFS{}
	WithFileSystem(fs)(&lc)
	if lc.FileSystem == nil {
		t.Error("expected FileSystem to be set")
	}
}

func TestWithConfigFileOption(t *testing.T) {
	var lc LoaderConfig
	WithConfigFile("/path/to/config.yml")(&lc)
	if lc.ConfigFile != "/path/to/config.yml" {
		t.Errorf("expected config file path, got %q", lc.ConfigFile)
	}
}

func TestWithEnvFileOption(t *testing.T) {
	var lc LoaderConfig
	WithEnvFile("/path/to/.env")(&lc)
	if lc.EnvFile != "/path/to/.env" {
		t.Errorf("expected env file path, got %q", lc.EnvFile)
	}
}
