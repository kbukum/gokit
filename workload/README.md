# workload

Provider-based workload manager for deploying and managing containerized workloads across Docker and Kubernetes runtimes.

## Install

```bash
go get github.com/kbukum/gokit/workload@latest
```

## Quick Start

### Docker

```go
import (
    "github.com/kbukum/gokit/workload"
    "github.com/kbukum/gokit/workload/docker"
)

mgr, _ := docker.NewManager(&docker.Config{Host: "unix:///var/run/docker.sock"}, nil, log)
result, _ := mgr.Deploy(ctx, workload.DeployRequest{
    Name:  "my-app",
    Image: "nginx:latest",
    Ports: []workload.PortMapping{{Container: 80, Host: 8080}},
})
```

### Kubernetes

```go
import (
    "github.com/kbukum/gokit/workload"
    "github.com/kbukum/gokit/workload/kubernetes"
)

mgr, _ := kubernetes.NewManager(&kubernetes.Config{Namespace: "default"}, nil, log)
result, _ := mgr.Deploy(ctx, workload.DeployRequest{
    Name:  "my-job",
    Image: "busybox:latest",
    Command: []string{"echo", "hello"},
})
```

### As a Component

```go
comp := workload.NewComponent(workload.Config{Provider: "docker"}, dockerCfg, log)
comp.Start(ctx)
defer comp.Stop(ctx)

mgr := comp.Manager()
```

## Key Types & Functions

### `workload`

| Symbol | Description |
|---|---|
| `Manager` | Interface — Deploy, Stop, Remove, Restart, Status, Wait, Logs, List, HealthCheck |
| `ExecProvider` | Optional — `Exec(ctx, id, cmd)` for running commands in workloads |
| `StatsProvider` | Optional — `Stats(ctx, id)` for CPU/memory/network metrics |
| `LogStreamer` | Optional — `StreamLogs(ctx, id, opts)` for real-time log streaming |
| `EventWatcher` | Optional — `WatchEvents(ctx, filter)` for lifecycle events |
| `NewComponent(cfg, providerCfg, log)` | Create lifecycle-managed component |
| `DeployRequest` | Name, Image, Command, Environment, Resources, Ports, Volumes |
| `WorkloadStatus` | ID, Status, Running, Healthy, ExitCode, Restarts |
| `ParseMemory(s)` | Parse memory strings ("512m", "1g") to bytes |
| `ParseCPU(s)` | Parse CPU strings ("0.5", "500m") to nanocores |

### `workload/docker`

| Symbol | Description |
|---|---|
| `NewManager(cfg, labels, log)` | Create Docker manager (implements all optional interfaces) |
| `Config` | Host, APIVersion, TLS, Network, Registry, Platform |

### `workload/kubernetes`

| Symbol | Description |
|---|---|
| `NewManager(cfg, labels, log)` | Create Kubernetes manager (Pod/Job support) |
| `Config` | Kubeconfig, Context, Namespace, ServiceAccount, WorkloadType, ImagePullPolicy |

---

[← Back to main gokit README](../README.md)
