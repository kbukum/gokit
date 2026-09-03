# workload

Provider-based workload manager for deploying and managing containerized workloads across Docker and Kubernetes runtimes.

## Install

```bash
go get github.com/kbukum/gokit/workload@latest
```

The Docker Model Runner backend is a separately versioned nested module; install it too when you use the model runtime:

```bash
go get github.com/kbukum/gokit/workload/dmr@latest
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
import "github.com/kbukum/gokit/workload/docker"

reg := workload.NewFactoryRegistry()
if err := docker.Register(reg); err != nil {
    return err
}
comp := workload.NewComponent(reg, workload.Config{Provider: "docker"}, dockerCfg, log)
if err := comp.Start(ctx); err != nil {
    return err
}
defer func() { _ = comp.Stop(ctx) }()

mgr := comp.Manager()
```

## Model runtime

Beyond deploying containers, `workload` exposes a runtime-neutral `ModelRuntime` port for running and serving AI models as managed workloads — pull a model, discover its inference endpoint, report stats, and unload it — independent of the backend. Docker Model Runner (DMR) is the first backend; Ollama, OCI, and Hugging Face runtimes plug in behind the same port via the identical opt-in `Register` pattern.

`ModelRuntime` manages models *within* a runtime and reports *where* to send inference; it does not perform inference itself — send requests to the returned endpoint with an appropriate client (the `llm` module owns inference).

```go
import (
    "github.com/kbukum/gokit/workload"
    "github.com/kbukum/gokit/workload/dmr"
)

reg := workload.NewModelRuntimeRegistry()
if err := dmr.Register(reg, dmr.Config{}); err != nil {
    return err
}
rt, err := workload.NewModelRuntime(reg, workload.ModelRuntimeConfig{Provider: workload.ProviderDMR}, log)
if err != nil {
    return err
}

h, err := rt.Start(ctx, workload.ModelSpec{Ref: "ai/smollm2"})
if err != nil {
    return err
}
// h.Endpoint.BaseURL is an OpenAI-compatible base URL for inference.
```

Test consumers without a real daemon using the in-memory fake in [`workload/testutil`](testutil):

```go
rt := testutil.NewMockModelRuntime()
h, _ := rt.Start(ctx, workload.ModelSpec{Ref: "ai/smollm2"})
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
| `NewComponent(registry, cfg, providerCfg, log)` | Create lifecycle-managed component |
| `DeployRequest` | Name, Image, Command, Environment, Resources, Ports, Volumes |
| `WorkloadStatus` | ID, Status, Running, Healthy, ExitCode, Restarts |
| `ParseMemory(s)` | Parse memory strings ("512m", "1g") to bytes |
| `ParseCPU(s)` | Parse CPU strings ("0.5", "500m") to nanocores |
| `ModelRuntime` | Interface — Start, Stop, Endpoint (run/serve a model) |
| `ModelHealthChecker` / `ModelStatsReporter` | Optional — probe reachability / report per-model usage |
| `ModelLister` / `ModelRemover` | Optional — enumerate / delete local models |
| `NewModelRuntime(registry, cfg, log)` | Create a model runtime from a registry |
| `ModelSpec` / `ModelHandle` / `Endpoint` | Model identity, started-model handle, inference endpoint |

### `workload/dmr`

| Symbol | Description |
|---|---|
| `Register(registry, cfg)` | Install the Docker Model Runner backend under `ProviderDMR` |
| `NewRuntime(cfg, log)` | Create a DMR runtime directly |
| `Config` | BaseURL, Timeout, PullTimeout, ResiliencePolicy |

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
