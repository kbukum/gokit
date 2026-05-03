# config

Configuration loading from YAML files and environment variables with automatic resolution and validation.

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
| `WithConfigFile()` | Option to specify config file path |
| `WithEnvFile()` | Option to specify .env file path |
| `WithProfile()` | Option to load profile-specific env file |
| `WithFileSystem()` | Option to inject custom filesystem |
| `FileSystem` | Interface for file existence and env loading |

### Loading Order (lowest → highest priority)

1. YAML config file (`config.yml`)
2. Profile env file (`config/profiles/{profile}.env`)
3. `.env` file
4. Environment variables

### ServiceConfig Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Name` | `string` | `""` | Service name |
| `Environment` | `string` | `"development"` | Deployment environment |
| `Version` | `string` | `""` | Service version |
| `Address` | `string` | `"0.0.0.0"` | Service bind address |
| `Port` | `int` | `50051` | Service port |
| `Debug` | `bool` | `false` | Debug mode (auto-enabled in development) |
| `Logging` | `logger.Config` | | Logging configuration (level, format, etc.) |

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
