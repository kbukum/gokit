# config

Configuration loading from YAML files and environment variables with automatic resolution and validation, plus strict (unknown-key-rejecting) loading, change watching, and pluggable persistence sinks.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package config

import (
    gkconfig "github.com/kbukum/gokit/config"
    "github.com/kbukum/gokit/database"
    "github.com/kbukum/gokit/cache"
    "github.com/kbukum/gokit/server"
    "github.com/kbukum/gokit/messaging/kafka"
    "github.com/kbukum/gokit/discovery"
)

// ServiceConfig embeds the base gokit config and adds infrastructure modules.
type ServiceConfig struct {
    gkconfig.ServiceConfig `yaml:",inline" mapstructure:",squash"`

    HTTP      server.Config    `yaml:"server" mapstructure:"server"`
    Database  database.Config  `yaml:"database" mapstructure:"database"`
    Cache     cache.Config     `yaml:"cache" mapstructure:"cache"`
    Kafka     kafka.Config     `yaml:"kafka" mapstructure:"kafka"`
    Discovery discovery.Config `yaml:"discovery" mapstructure:"discovery"`
}
```

Then load it in your bootstrap:

```go
var cfg config.ServiceConfig
if err := gkconfig.LoadConfig("my-service", &cfg); err != nil {
    panic(err)
}
fmt.Printf("Running %s on %s:%d in %s mode\n",
    cfg.Name, cfg.Address, cfg.Port, cfg.Environment)
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `ServiceConfig` | Common fields: Name, Environment, Version, Address, Port, Debug, Logging |
| `LoadConfig()` | Load config from YAML + env with auto-resolution |
| `Resolver` | Resolves config and .env file paths |
| `WithConfigFile()` | Option to specify a config file path; it must exist |
| `WithEnvFile()` | Option to specify a .env file path; it must exist |
| `WithProfile()` | Option to load a profile env file; a named profile must exist, an `ENVIRONMENT`-derived one is optional |
| `WithProfileDir()` | Option to make one directory the only place searched for `<profile>.env` |
| `WithoutDiscovery()` | Option to skip the working-directory search, so only explicit inputs load |
| `ErrFileNotFound` / `ErrInvalidProfile` | Errors for a missing explicit input or a profile name outside `^[a-z0-9][a-z0-9_-]*$` |
| `WithFileSystem()` | Option to inject custom filesystem |
| `FileSystem` | Interface for file existence and env loading; `Exists` returns an error for probe failures other than "not found" |
| `LoadStrict[T]()` / `LoadStrictWithCodec[T]()` | Load into `T`, rejecting unknown keys instead of ignoring them |
| `AppConfig` | Typed contract: embed `*ServiceConfig` and implement `ApplyDefaults` + `Validate` |
| `ConfigSink` / `NewInMemoryConfigSink()` / `NewFileConfigSink()` | Persist runtime configuration entries (in-memory or file-backed) |
| `ConfigWatch` / `ConfigChange` | Stream set / remove change events over a channel |

### Loading Order (lowest → highest priority)

1. Config file — auto-discovery finds YAML only (`config.yml`); `WithConfigFile()` also accepts an explicit TOML file (`.toml`)
2. Profile env file (`config/profiles/{profile}.env`)
3. `.env` file
4. Environment variables

**Explicit inputs fail loudly:** a missing explicit file or named profile returns `ErrFileNotFound` instead of falling back to defaults. Discovered files stay optional. Env files are loaded into the process environment and never override a variable that is already set.

### ServiceConfig Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Name` | `string` | `""` | Service name |
| `Environment` | `string` | `"development"` | Deployment environment |
| `Version` | `string` | `""` | Service version |
| `Address` | `string` | `"0.0.0.0"` | Service bind address |
| `Port` | `int` | `50051` | Service port |
| `Debug` | `bool` | `false` | Debug mode (auto-enabled in development) |
| `Logging` | `logging.Config` | | Logging configuration (level, format, etc.) |

### Environment Type

The `Environment` type represents deployment environments with helper methods:

```go
const (
	Development Environment = "development"
	Staging     Environment = "staging"
	Production  Environment = "production"
)
```

Access via `ServiceConfig`:

```go
env := cfg.GetEnvironment()
if env.IsProduction() {
	// production-specific behavior
}
if env.IsDevelopment() {
	// enable debug features
}
```

### Validation

`ServiceConfig.Validate()` returns `errors.AppError` (from `github.com/kbukum/gokit/errors`) for validation failures.

---

[⬅ Back to main README](../README.md)
