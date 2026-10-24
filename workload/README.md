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

To list the models a runtime holds, install the matching lister module: `workload/ollama`, `workload/lmstudio` or `workload/openaicompat`.

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

### Listing and describing models

`ModelLister.ListModels` returns each model with an `ai.Model` descriptor: family, variant, digest, task, modalities, context length and capabilities. Each field records its source, and a field the runtime does not state stays unknown, never assumed supported.

| Module | Reads | States |
|---|---|---|
| `workload/dmr` | `GET /models` | family, variant, digest, context length when configured |
| `workload/ollama` | `GET /api/tags`, then `POST /api/show` per model | family, variant, digest, context length, task, vision, tools, thinking, audio |
| `workload/lmstudio` | `GET /api/v1/models` | family, variant, context length, task, vision, tool use, reasoning |
| `workload/openaicompat` | `GET {base}/models` | the id, and vLLM's context length |

Every lister uses the shared `httpclient`, bounds each request and response body, sends an optional credential in a header, and returns errors that carry only the status, never the response body. Set `EndpointPolicy` to limit requests to granted origins: DNS is checked at dial and redirects are refused.

```go
l, err := ollama.New(ollama.Config{BaseURL: "http://localhost:11434", EndpointPolicy: policy})
if err != nil {
    return err
}
models, err := l.ListModels(ctx)
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
| `Config` | BaseURL, Timeout, PullTimeout, ResiliencePolicy, EndpointPolicy |

### `workload/ollama`, `workload/lmstudio`, `workload/openaicompat`

| Symbol | Description |
|---|---|
| `New(cfg)` | Create a lister that implements `workload.ModelLister` |
| `Config` | BaseURL, Timeout, EndpointPolicy, Auth; Ollama adds MaxDescribed and Concurrency |
| `Error` | A non-2xx answer, carrying the status only |

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
