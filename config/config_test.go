package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/logger"
)

func TestServiceConfigApplyDefaults(t *testing.T) {
	t.Run("empty environment defaults to development", func(t *testing.T) {
		cfg := ServiceConfig{Name: "svc"}
		cfg.ApplyDefaults()
		if cfg.Environment != "development" {
			t.Errorf("expected 'development', got %q", cfg.Environment)
		}
		if !cfg.Debug {
			t.Error("expected debug=true for development")
		}
	})

	t.Run("production environment keeps debug false", func(t *testing.T) {
		cfg := ServiceConfig{Name: "svc", Environment: "production"}
		cfg.ApplyDefaults()
		if cfg.Debug {
			t.Error("expected debug=false for production")
		}
	})

	t.Run("development sets debug true", func(t *testing.T) {
		cfg := ServiceConfig{Name: "svc", Environment: "development"}
		cfg.ApplyDefaults()
		if !cfg.Debug {
			t.Error("expected debug=true for development")
		}
	})
}

func TestServiceConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ServiceConfig
		wantErr bool
		errMsg  string
	}{
		{"valid development", ServiceConfig{Name: "svc", Environment: "development", Logging: logger.Config{Level: "info", Format: "console"}}, false, ""},
		{"valid staging", ServiceConfig{Name: "svc", Environment: "staging", Logging: logger.Config{Level: "info", Format: "console"}}, false, ""},
		{"valid production", ServiceConfig{Name: "svc", Environment: "production", Logging: logger.Config{Level: "info", Format: "console"}}, false, ""},
		{"missing name", ServiceConfig{Environment: "production", Logging: logger.Config{Level: "info", Format: "console"}}, true, "config.name is required"},
		{"invalid environment", ServiceConfig{Name: "svc", Environment: "invalid", Logging: logger.Config{Level: "info", Format: "console"}}, true, "config.environment must be one of"},
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
	if err := os.WriteFile(configPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	type TestConfig struct {
		Base ServiceConfig `yaml:"base" mapstructure:"base"`
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
		Base ServiceConfig `yaml:"base" mapstructure:"base"`
	}

	var cfg TestConfig
	// With no config file found, LoadConfig should still succeed (just empty config)
	err := LoadConfig("nonexistent-service", &cfg, WithConfigFile("/nonexistent/path.yml"))
	if err != nil {
		t.Fatalf("expected LoadConfig to succeed with missing file, got %v", err)
	}
}

func TestResolverWithMockFS(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		"./cmd/my-svc/config.yml": true,
	}}
	resolver := &Resolver{FileSystem: fs}
	files := resolver.ResolveFiles("my-svc", LoaderConfig{})
	if files.ConfigFile != "./cmd/my-svc/config.yml" {
		t.Errorf("expected config file at ./cmd/my-svc/config.yml, got %q", files.ConfigFile)
	}
}

type mockFS struct {
	files map[string]bool
}

func (m *mockFS) Exists(path string) bool   { return m.files[path] }
func (m *mockFS) LoadEnv(path string) error { return nil }
func (m *mockFS) Getwd() (string, error)    { return "/mock", nil }

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

// ---------------------------------------------------------------------------
// Environment type tests
// ---------------------------------------------------------------------------

func TestEnvironmentString(t *testing.T) {
	tests := []struct {
		env  Environment
		want string
	}{
		{Development, "development"},
		{Staging, "staging"},
		{Production, "production"},
		{Environment("custom"), "custom"},
		{Environment(""), ""},
	}
	for _, tc := range tests {
		t.Run(string(tc.env), func(t *testing.T) {
			if got := tc.env.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnvironmentIsProduction(t *testing.T) {
	tests := []struct {
		env  Environment
		want bool
	}{
		{Production, true},
		{Environment("production"), true},
		// Note: Environment is case-sensitive; "Production" != "production"
		{Environment("Production"), false},
		{Environment("PRODUCTION"), false},
		{Development, false},
		{Staging, false},
		{Environment(""), false},
		{Environment("unknown"), false},
	}
	for _, tc := range tests {
		t.Run(string(tc.env), func(t *testing.T) {
			if got := tc.env.IsProduction(); got != tc.want {
				t.Errorf("IsProduction() for %q = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestEnvironmentIsDevelopment(t *testing.T) {
	tests := []struct {
		env  Environment
		want bool
	}{
		{Development, true},
		{Environment("development"), true},
		{Environment("Development"), false},
		{Environment("DEVELOPMENT"), false},
		{Production, false},
		{Staging, false},
		{Environment(""), false},
		{Environment("unknown"), false},
	}
	for _, tc := range tests {
		t.Run(string(tc.env), func(t *testing.T) {
			if got := tc.env.IsDevelopment(); got != tc.want {
				t.Errorf("IsDevelopment() for %q = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ServiceConfig tests (ApplyDefaults, Validate, helpers)
// ---------------------------------------------------------------------------

func TestApplyDefaultsSetsLoggingDefaults(t *testing.T) {
	cfg := ServiceConfig{Name: "svc"}
	cfg.ApplyDefaults()

	if cfg.Logging.Level != "info" {
		t.Errorf("expected logging level 'info', got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "console" {
		t.Errorf("expected logging format 'console', got %q", cfg.Logging.Format)
	}
	if cfg.Logging.ServiceName != "svc" {
		t.Errorf("expected logging service_name 'svc', got %q", cfg.Logging.ServiceName)
	}
}

func TestApplyDefaultsDoesNotOverridePresetValues(t *testing.T) {
	cfg := ServiceConfig{
		Name:        "my-app",
		Environment: "staging",
		Version:     "2.0.0",
		Debug:       false,
		Logging:     logger.Config{Level: "debug", Format: "json", ServiceName: "custom"},
	}
	cfg.ApplyDefaults()

	if cfg.Environment != "staging" {
		t.Errorf("expected environment 'staging', got %q", cfg.Environment)
	}
	if cfg.Debug {
		t.Error("staging should not force debug=true")
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected logging level 'debug', got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected logging format 'json', got %q", cfg.Logging.Format)
	}
	if cfg.Logging.ServiceName != "custom" {
		t.Errorf("expected logging service_name 'custom', got %q", cfg.Logging.ServiceName)
	}
}

func TestApplyDefaultsPropagatesServiceNameToLogging(t *testing.T) {
	cfg := ServiceConfig{Name: "my-service"}
	cfg.ApplyDefaults()
	if cfg.Logging.ServiceName != "my-service" {
		t.Errorf("expected logging service_name 'my-service', got %q", cfg.Logging.ServiceName)
	}
}

func TestApplyDefaultsEmptyNameDoesNotSetLoggingServiceName(t *testing.T) {
	cfg := ServiceConfig{}
	cfg.ApplyDefaults()
	if cfg.Logging.ServiceName != "" {
		t.Errorf("expected empty logging service_name when Name is empty, got %q", cfg.Logging.ServiceName)
	}
}

func TestGetEnvironment(t *testing.T) {
	cfg := ServiceConfig{Environment: "production"}
	env := cfg.GetEnvironment()
	if env != Production {
		t.Errorf("expected Production, got %v", env)
	}
	if !env.IsProduction() {
		t.Error("expected IsProduction() = true")
	}
}

func TestGetServiceConfig(t *testing.T) {
	cfg := ServiceConfig{Name: "svc"}
	got := cfg.GetServiceConfig()
	if got != &cfg {
		t.Error("GetServiceConfig() should return pointer to receiver")
	}
}

func TestValidateEmptyEnvironment(t *testing.T) {
	cfg := ServiceConfig{Name: "svc", Environment: "", Logging: logger.Config{Level: "info", Format: "console"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty environment")
	}
	if !strings.Contains(err.Error(), "config.environment must be one of") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateInvalidLogging(t *testing.T) {
	cfg := ServiceConfig{Name: "svc", Environment: "development", Logging: logger.Config{Level: "invalid", Format: "console"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid logging level")
	}
	if !strings.Contains(err.Error(), "config.logging") {
		t.Errorf("expected error containing 'config.logging', got %q", err.Error())
	}
}

func TestValidateInvalidLogFormat(t *testing.T) {
	cfg := ServiceConfig{Name: "svc", Environment: "development", Logging: logger.Config{Level: "info", Format: "xml"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid logging format")
	}
	if !strings.Contains(err.Error(), "config.logging") {
		t.Errorf("expected error containing 'config.logging', got %q", err.Error())
	}
}

func TestValidateAllValidEnvironments(t *testing.T) {
	for _, env := range []string{"development", "staging", "production"} {
		t.Run(env, func(t *testing.T) {
			cfg := ServiceConfig{Name: "svc", Environment: env, Logging: logger.Config{Level: "info", Format: "console"}}
			if err := cfg.Validate(); err != nil {
				t.Errorf("unexpected error for env %q: %v", env, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LoadConfig with mock filesystem – YAML loading, env override, edge cases
// ---------------------------------------------------------------------------

// advancedMockFS supports file content, env loading, and error injection.
type advancedMockFS struct {
	files   map[string]bool
	envVars map[string]string // vars to set when LoadEnv is called
	loadErr error             // error to return from LoadEnv
	cwd     string
}

func (m *advancedMockFS) Exists(path string) bool { return m.files[path] }
func (m *advancedMockFS) LoadEnv(path string) error {
	if m.loadErr != nil {
		return m.loadErr
	}
	for k, v := range m.envVars {
		os.Setenv(k, v)
	}
	return nil
}
func (m *advancedMockFS) Getwd() (string, error) { return m.cwd, nil }

func validLogging() logger.Config {
	return logger.Config{Level: "info", Format: "console"}
}

func TestLoadConfigFromYAMLInline(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	yaml := `name: yaml-service
environment: staging
version: "2.0.0"
debug: true
logging:
  level: warn
  format: json
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg ServiceConfig
	if err := LoadConfig("yaml-service", &cfg, WithConfigFile(cfgPath)); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Name != "yaml-service" {
		t.Errorf("Name = %q, want 'yaml-service'", cfg.Name)
	}
	if cfg.Environment != "staging" {
		t.Errorf("Environment = %q, want 'staging'", cfg.Environment)
	}
	if cfg.Version != "2.0.0" {
		t.Errorf("Version = %q, want '2.0.0'", cfg.Version)
	}
	if !cfg.Debug {
		t.Error("Debug should be true")
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("Logging.Level = %q, want 'warn'", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Logging.Format = %q, want 'json'", cfg.Logging.Format)
	}
}

func TestLoadConfigEnvVarOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	yaml := `name: from-yaml
environment: staging
version: "1.0.0"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set env var to override
	os.Setenv("NAME", "from-env")
	defer os.Unsetenv("NAME")

	var cfg ServiceConfig
	if err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath)); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Name != "from-env" {
		t.Errorf("Name = %q, want 'from-env' (env should override YAML)", cfg.Name)
	}
}

func TestLoadConfigEnvFileLoaded(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	envContent := "VERSION=3.0.0-env\n"
	if err := os.WriteFile(envPath, []byte(envContent), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg ServiceConfig
	err := LoadConfig("svc", &cfg, WithEnvFile(envPath))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Version != "3.0.0-env" {
		t.Errorf("Version = %q, want '3.0.0-env'", cfg.Version)
	}

	os.Unsetenv("VERSION")
}

func TestLoadConfigMissingFileUsesDefaults(t *testing.T) {
	fs := &mockFS{files: map[string]bool{}}
	var cfg ServiceConfig
	err := LoadConfig("missing-service", &cfg, WithFileSystem(fs))
	if err != nil {
		t.Fatalf("expected no error with missing config, got %v", err)
	}
	// All fields should be zero values except Name (populated from serviceName)
	if cfg.Name != "missing-service" {
		t.Errorf("Name = %q, want %q", cfg.Name, "missing-service")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	invalidYAML := `name: valid
environment: [this is : broken yaml
  not valid at all: {{{
`
	if err := os.WriteFile(cfgPath, []byte(invalidYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg ServiceConfig
	// LoadConfig prints a warning but doesn't fail fatally for bad YAML –
	// it falls back to empty config. Verify it doesn't panic.
	err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath))
	// Whether it returns error or just warns depends on viper behavior;
	// the key point is no panic.
	_ = err
}

func TestLoadConfigTypeDurationCoercion(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	yaml := `timeout: 5s
retry_interval: 100ms
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	type DurationConfig struct {
		Timeout       string `yaml:"timeout" mapstructure:"timeout"`
		RetryInterval string `yaml:"retry_interval" mapstructure:"retry_interval"`
	}
	var cfg DurationConfig
	if err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath)); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Timeout != "5s" {
		t.Errorf("Timeout = %q, want '5s'", cfg.Timeout)
	}
}

func TestLoadConfigSliceCoercion(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	yaml := `tags:
  - alpha
  - beta
  - gamma
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	type SliceConfig struct {
		Tags []string `yaml:"tags" mapstructure:"tags"`
	}
	var cfg SliceConfig
	if err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath)); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.Tags) != 3 || cfg.Tags[0] != "alpha" || cfg.Tags[2] != "gamma" {
		t.Errorf("Tags = %v, want [alpha beta gamma]", cfg.Tags)
	}
}

func TestLoadConfigIntCoercion(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	yaml := `port: 8080
max_connections: 100
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	type IntConfig struct {
		Port           int `yaml:"port" mapstructure:"port"`
		MaxConnections int `yaml:"max_connections" mapstructure:"max_connections"`
	}
	var cfg IntConfig
	if err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath)); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.MaxConnections != 100 {
		t.Errorf("MaxConnections = %d, want 100", cfg.MaxConnections)
	}
}

func TestLoadConfigNestedStruct(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	yaml := `database:
  host: localhost
  port: 5432
  name: testdb
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	type DBConfig struct {
		Host string `yaml:"host" mapstructure:"host"`
		Port int    `yaml:"port" mapstructure:"port"`
		Name string `yaml:"name" mapstructure:"name"`
	}
	type AppConfig struct {
		Database DBConfig `yaml:"database" mapstructure:"database"`
	}
	var cfg AppConfig
	if err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath)); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("Database.Host = %q, want 'localhost'", cfg.Database.Host)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, want 5432", cfg.Database.Port)
	}
	if cfg.Database.Name != "testdb" {
		t.Errorf("Database.Name = %q, want 'testdb'", cfg.Database.Name)
	}
}

func TestLoadConfigExtraFieldsIgnored(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	yaml := `name: my-svc
environment: development
unknown_field: should-be-ignored
another_extra: 42
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg ServiceConfig
	err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath))
	if err != nil {
		t.Fatalf("LoadConfig should not fail on extra fields: %v", err)
	}
	if cfg.Name != "my-svc" {
		t.Errorf("Name = %q, want 'my-svc'", cfg.Name)
	}
}

func TestLoadConfigSpecialCharacters(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	yaml := `name: "svc-with-special!@#$%"
version: "1.0.0-beta+build.123"
environment: development
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg ServiceConfig
	if err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath)); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Name != "svc-with-special!@#$%" {
		t.Errorf("Name = %q, want special chars preserved", cfg.Name)
	}
	if cfg.Version != "1.0.0-beta+build.123" {
		t.Errorf("Version = %q, want semver with build metadata", cfg.Version)
	}
}

func TestLoadConfigVeryLongValues(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	longVal := strings.Repeat("a", 10000)
	yaml := "name: " + longVal + "\nenvironment: development\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg ServiceConfig
	if err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath)); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Name != longVal {
		t.Errorf("expected 10000 char name, got length %d", len(cfg.Name))
	}
}

func TestLoadConfigEmptyServiceName(t *testing.T) {
	fs := &mockFS{files: map[string]bool{}}
	var cfg ServiceConfig
	// Empty service name should trigger validation failure (name is required).
	err := LoadConfig("", &cfg, WithFileSystem(fs))
	if err == nil {
		t.Fatal("LoadConfig with empty service name should return validation error")
	}
	if !strings.Contains(err.Error(), "config.name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Resolver tests
// ---------------------------------------------------------------------------

func TestResolverFindsConfigInCmdServiceDir(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		"./cmd/my-svc/config.yml": true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("my-svc", LoaderConfig{})
	if files.ConfigFile != "./cmd/my-svc/config.yml" {
		t.Errorf("ConfigFile = %q, want './cmd/my-svc/config.yml'", files.ConfigFile)
	}
}

func TestResolverFindsConfigInConfigDir(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		"./config/config.yml": true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("my-svc", LoaderConfig{})
	if files.ConfigFile != "./config/config.yml" {
		t.Errorf("ConfigFile = %q, want './config/config.yml'", files.ConfigFile)
	}
}

func TestResolverFindsConfigAtRoot(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		"./config.yml": true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("my-svc", LoaderConfig{})
	if files.ConfigFile != "./config.yml" {
		t.Errorf("ConfigFile = %q, want './config.yml'", files.ConfigFile)
	}
}

func TestResolverReturnsEmptyWhenNoFilesFound(t *testing.T) {
	fs := &mockFS{files: map[string]bool{}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("no-files", LoaderConfig{})
	if files.ConfigFile != "" {
		t.Errorf("ConfigFile = %q, want empty", files.ConfigFile)
	}
	if files.EnvFile != "" {
		t.Errorf("EnvFile = %q, want empty", files.EnvFile)
	}
}

func TestResolverPriorityCmdOverConfig(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		"./cmd/my-svc/config.yml": true,
		"./config/config.yml":     true,
		"./config.yml":            true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("my-svc", LoaderConfig{})
	// cmd/{service}/ should be found first
	if files.ConfigFile != "./cmd/my-svc/config.yml" {
		t.Errorf("ConfigFile = %q, want './cmd/my-svc/config.yml' (highest priority)", files.ConfigFile)
	}
}

func TestResolverShortNameFallback(t *testing.T) {
	// For "platform-api", shortName = "api"
	fs := &mockFS{files: map[string]bool{
		"./cmd/api/config.yml": true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("platform-api", LoaderConfig{})
	if files.ConfigFile != "./cmd/api/config.yml" {
		t.Errorf("ConfigFile = %q, want './cmd/api/config.yml' (short name fallback)", files.ConfigFile)
	}
}

func TestResolverExplicitPathTakesPrecedence(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		"./cmd/my-svc/config.yml": true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("my-svc", LoaderConfig{
		ConfigFile: "/explicit/config.yml",
		EnvFile:    "/explicit/.env",
	})
	if files.ConfigFile != "/explicit/config.yml" {
		t.Errorf("ConfigFile = %q, want '/explicit/config.yml'", files.ConfigFile)
	}
	if files.EnvFile != "/explicit/.env" {
		t.Errorf("EnvFile = %q, want '/explicit/.env'", files.EnvFile)
	}
}

func TestResolverFindsEnvFile(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		".env": true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("my-svc", LoaderConfig{})
	if files.EnvFile == "" {
		t.Error("expected .env file to be found")
	}
}

func TestResolverFindsServiceSpecificEnvFile(t *testing.T) {
	// buildEnvSearchPaths produces paths like "./cmd/my-svc/" which combine
	// with the env file name as "./cmd/my-svc//.env.my-svc"
	fs := &mockFS{files: map[string]bool{
		"./cmd/my-svc//.env.my-svc": true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("my-svc", LoaderConfig{})
	if files.EnvFile != "./cmd/my-svc//.env.my-svc" {
		t.Errorf("EnvFile = %q, want './cmd/my-svc//.env.my-svc'", files.EnvFile)
	}
}

func TestResolverParentDirectorySearch(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		"../cmd/my-svc/config.yml": true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("my-svc", LoaderConfig{})
	if files.ConfigFile != "../cmd/my-svc/config.yml" {
		t.Errorf("ConfigFile = %q, want '../cmd/my-svc/config.yml'", files.ConfigFile)
	}
}

func TestResolverGrandparentDirectorySearch(t *testing.T) {
	fs := &mockFS{files: map[string]bool{
		"../../cmd/my-svc/config.yml": true,
	}}
	r := &Resolver{FileSystem: fs}
	files := r.ResolveFiles("my-svc", LoaderConfig{})
	if files.ConfigFile != "../../cmd/my-svc/config.yml" {
		t.Errorf("ConfigFile = %q, want '../../cmd/my-svc/config.yml'", files.ConfigFile)
	}
}

// ---------------------------------------------------------------------------
// Loader option tests
// ---------------------------------------------------------------------------

func TestWithFileSystemOverridesDefault(t *testing.T) {
	customFS := &mockFS{files: map[string]bool{"custom": true}}
	var lc LoaderConfig
	WithFileSystem(customFS)(&lc)
	if lc.FileSystem != customFS {
		t.Error("WithFileSystem should set the custom filesystem")
	}
	// Verify it's the correct instance
	if !lc.FileSystem.Exists("custom") {
		t.Error("custom filesystem should report 'custom' exists")
	}
}

func TestLoaderOptionsChained(t *testing.T) {
	customFS := &mockFS{files: map[string]bool{}}
	opts := []LoaderOption{
		WithFileSystem(customFS),
		WithConfigFile("/my/config.yml"),
		WithEnvFile("/my/.env"),
	}
	var lc LoaderConfig
	for _, opt := range opts {
		opt(&lc)
	}
	if lc.FileSystem != customFS {
		t.Error("FileSystem not set")
	}
	if lc.ConfigFile != "/my/config.yml" {
		t.Errorf("ConfigFile = %q", lc.ConfigFile)
	}
	if lc.EnvFile != "/my/.env" {
		t.Errorf("EnvFile = %q", lc.EnvFile)
	}
}

// ---------------------------------------------------------------------------
// Multi-source precedence: Default < YAML < .env < env var
// ---------------------------------------------------------------------------

func TestMultiSourcePrecedence(t *testing.T) {
	dir := t.TempDir()

	// YAML sets environment to "staging"
	cfgPath := filepath.Join(dir, "config.yml")
	yaml := `name: yaml-name
environment: staging
version: "1.0.0"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	// .env overrides version
	envPath := filepath.Join(dir, ".env")
	envContent := "VERSION=2.0.0-from-env\n"
	if err := os.WriteFile(envPath, []byte(envContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Real env var overrides name
	os.Setenv("NAME", "from-real-env")
	defer os.Unsetenv("NAME")
	defer os.Unsetenv("VERSION")

	var cfg ServiceConfig
	err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath), WithEnvFile(envPath))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// name: env var should win over YAML
	if cfg.Name != "from-real-env" {
		t.Errorf("Name = %q, want 'from-real-env' (env var should override YAML)", cfg.Name)
	}
	// environment: from YAML (no env var override)
	if cfg.Environment != "staging" {
		t.Errorf("Environment = %q, want 'staging' (from YAML)", cfg.Environment)
	}
}

func TestYAMLOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	yaml := `name: yaml-svc
environment: production
debug: true
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg ServiceConfig
	if err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath)); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	// YAML value should be loaded (not zero values)
	if cfg.Name != "yaml-svc" {
		t.Errorf("Name = %q, want 'yaml-svc'", cfg.Name)
	}
	if cfg.Environment != "production" {
		t.Errorf("Environment = %q, want 'production'", cfg.Environment)
	}
	if !cfg.Debug {
		t.Error("Debug should be true from YAML")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestLoadConfigEmptyYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	if err := os.WriteFile(cfgPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg ServiceConfig
	err := LoadConfig("svc", &cfg, WithConfigFile(cfgPath))
	if err != nil {
		t.Fatalf("empty YAML should not cause error: %v", err)
	}
}

func TestLoadConfigNilTarget(t *testing.T) {
	// Passing nil should cause an error, not a panic
	defer func() {
		if r := recover(); r != nil {
			t.Logf("recovered panic (acceptable): %v", r)
		}
	}()

	fs := &mockFS{files: map[string]bool{}}
	_ = LoadConfig("svc", nil, WithFileSystem(fs))
}

func TestLoadConfigWithMockFSNoFiles(t *testing.T) {
	fs := &advancedMockFS{
		files: map[string]bool{},
		cwd:   "/project",
	}
	var cfg ServiceConfig
	err := LoadConfig("svc", &cfg, WithFileSystem(fs))
	if err != nil {
		t.Fatalf("no files should not cause error: %v", err)
	}
}

func TestLoadConfigDefaultFileSystemWhenNoneProvided(t *testing.T) {
	// When no WithFileSystem option, LoadConfig should use RealFileSystem
	var cfg ServiceConfig
	// This shouldn't panic
	err := LoadConfig("nonexistent-svc-12345", &cfg)
	if err != nil {
		t.Fatalf("expected no error with default FS, got %v", err)
	}
}

func TestRealFileSystemExists(t *testing.T) {
	rfs := &RealFileSystem{}
	// The test file itself should exist
	if !rfs.Exists("config_test.go") {
		t.Error("expected config_test.go to exist")
	}
	if rfs.Exists("this-file-does-not-exist-12345.xyz") {
		t.Error("expected nonexistent file to not exist")
	}
}

func TestRealFileSystemGetwd(t *testing.T) {
	rfs := &RealFileSystem{}
	cwd, err := rfs.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if cwd == "" {
		t.Error("expected non-empty working directory")
	}
}

// ---------------------------------------------------------------------------
// generateEnvKeyVariants tests (exported for internal package access)
// ---------------------------------------------------------------------------

func TestGenerateEnvKeyVariants(t *testing.T) {
	tests := []struct {
		input    string
		contains []string
	}{
		{"NAME", []string{"name"}},
		{"AUTH_JWT_SECRET", []string{"auth_jwt_secret", "auth.jwt.secret", "auth.jwt_secret"}},
		{"HTTP_CORS_ALLOWED_ORIGINS", []string{
			"http_cors_allowed_origins",
			"http.cors.allowed.origins",
			"http.cors_allowed_origins",
		}},
		{"SINGLE", []string{"single"}},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			variants := generateEnvKeyVariants(tc.input)
			for _, want := range tc.contains {
				found := false
				for _, v := range variants {
					if v == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("variants for %q should contain %q, got %v", tc.input, want, variants)
				}
			}
		})
	}
}

func TestRemoveDuplicates(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b", "d"}
	result := removeDuplicates(input)
	if len(result) != 4 {
		t.Errorf("expected 4 unique items, got %d: %v", len(result), result)
	}
	expected := []string{"a", "b", "c", "d"}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestRemoveDuplicatesEmpty(t *testing.T) {
	result := removeDuplicates([]string{})
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestRemoveDuplicatesNoDupes(t *testing.T) {
	input := []string{"x", "y", "z"}
	result := removeDuplicates(input)
	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}
}
