# config

Configuration loading from YAML files and environment variables with automatic resolution and validation.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/kbukum/gokit/config"
)

type AppConfig struct {
    config.ServiceConfig `yaml:",inline" mapstructure:",squash"`
    Port                 int    `yaml:"port" mapstructure:"port"`
    DatabaseURL          string `yaml:"database_url" mapstructure:"database_url"`
}

func main() {
    var cfg AppConfig
    if err := config.LoadConfig("my-service", &cfg); err != nil {
        panic(err)
    }
    fmt.Printf("Running %s in %s mode\n", cfg.Name, cfg.Environment)
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `ServiceConfig` | Common fields: Name, Environment, Version, Debug, Logging |
| `LoadConfig()` | Load config from YAML + env with auto-resolution |
| `ConfigResolver` | Resolves config and .env file paths |
| `WithConfigFile()` | Option to specify config file path |
| `WithEnvFile()` | Option to specify .env file path |
| `WithFileSystem()` | Option to inject custom filesystem |
| `FileSystem` | Interface for file existence and env loading |

---

[⬅ Back to main README](../README.md)
