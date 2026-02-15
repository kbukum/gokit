# discovery

Service discovery and registry with pluggable providers — supports Consul and static endpoints.

## Install

```bash
go get github.com/skillsenselab/gokit/discovery@latest
```

## Quick Start

```go
import (
    "github.com/skillsenselab/gokit/discovery"
    _ "github.com/skillsenselab/gokit/discovery/consul"  // register consul provider
    "github.com/skillsenselab/gokit/logger"
)

log := logger.New()
comp := discovery.NewComponent(discovery.Config{
    Enabled:     true,
    Provider:    "consul",
    ConsulAddr:  "localhost:8500",
    ServiceName: "my-service",
    ServicePort: 8080,
}, log)

comp.Start(ctx)
defer comp.Stop(ctx)

comp.Registry().Register(ctx, &discovery.ServiceInfo{
    ID: "my-service-1", Name: "my-service", Address: "10.0.0.1", Port: 8080,
})
instances, _ := comp.Discovery().Discover(ctx, "user-service")
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Registry` | Interface — `Register`, `Deregister`, `Close` |
| `Discovery` | Interface — `Discover`, `Watch`, `Close` |
| `Component` | Managed lifecycle — `Start`, `Stop`, `Health`, `Registry`, `Discovery` |
| `Config` | Provider, Consul address/token, service name/port, health check, tags, metadata |
| `ServiceInfo` | ID, Name, Address, Port, Tags, Metadata |
| `ServiceInstance` | Extends ServiceInfo with Health status and LastSeen |
| `NewComponent(cfg, log)` | Create a managed discovery component |

### Providers

| Package | Description |
|---|---|
| `discovery/consul` | HashiCorp Consul — `NewProvider(cfg, log)` with full Register/Discover/Watch |
| `discovery/static` | In-memory static endpoints — `NewProvider(endpoints)` for dev/testing |

---

[← Back to main gokit README](../README.md)
