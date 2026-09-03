# workload/dmr

Docker Model Runner (DMR) backend for the `workload` module's runtime-neutral `ModelRuntime` port.

[Docker Model Runner](https://docs.docker.com/ai/model-runner/) pulls, runs, and serves AI models through Docker and exposes OpenAI-, Anthropic-, and Ollama-compatible inference endpoints. This package adapts DMR's REST surface to `workload.ModelRuntime`, so a consumer selects it through configuration like any other workload backend and stays free to swap in Ollama, OCI, or Hugging Face runtimes behind the same port.

## Install

```bash
go get github.com/kbukum/gokit/workload/dmr@latest
```

## Quick Start

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
// Send inference to h.Endpoint.BaseURL (OpenAI-compatible) using an OpenAI client.
```

## How it maps to DMR

| `ModelRuntime` method | DMR REST call | Notes |
|---|---|---|
| `Start(ctx, spec)` | `POST /models/create` `{"from": ref}` (+ `POST /engines/_configure` when `ContextSize` is set) | Pulls the model; DMR loads it lazily. A non-zero `ContextSize` is applied via `_configure`; `Resources` are rejected (no per-model compute control). |
| `Endpoint(ctx, model)` | — | Returns the OpenAI-compatible base URL (`…/engines/v1`); no network call. |
| `Health(ctx)` | `GET /models` | Optional `ModelHealthChecker`. Management-API reachability probe; it does not guarantee a model is loaded or the engine is ready to serve. |
| `Stats(ctx, model)` | `GET /models/{ref}` + `GET /engines/ps` | Optional `ModelStatsReporter`. Reports on-disk size and live load state; DMR exposes no resident-memory metric, so `MemoryBytes` stays zero. |
| `ListModels(ctx)` | `GET /models` | Optional `ModelLister`. |
| `RemoveModel(ctx, model)` | `DELETE /models/{ref}` | Optional `ModelRemover`. |
| `Stop(ctx, model)` | `POST /engines/unload` `{"models":[ref]}` | Unloads a running model; idempotent for unknown/idle models. |

## Configuration

| Field | Default | Description |
|---|---|---|
| `BaseURL` | `http://localhost:12434` | DMR API root. From inside a container use `http://model-runner.docker.internal`. |
| `Timeout` | `30s` | Bounds each non-streaming request so a deadline-less context cannot hang. Model pulls stream progress and are governed separately (see `PullTimeout`). |
| `PullTimeout` | `30m` | Bounds a model pull when the caller's context has no deadline, so a stalled create stream cannot hang forever; a byte cap alone cannot bound an idle stream. An earlier caller deadline takes precedence. Raise it for large multi-gigabyte downloads. |
| `ResiliencePolicy` | none | Optional `*resilience.Policy` applied to non-streaming requests (retry, circuit-breaking, rate limiting). |

The backend is built on the canonical `httpclient` adapter and `resilience` package, so timeouts and any configured policy are handled by the shared owners rather than a bespoke client.

## Security

The DMR API is unauthenticated by design — any client that can reach it can pull, load, and run models. Keep it on a trusted boundary and never expose the endpoint publicly. Model references are treated as untrusted input: path segments are validated so a crafted reference cannot escape the `/models/` route. Model pulls can be long-running; callers own their duration via the request context, and a deadline-less context falls back to the configurable `PullTimeout` (default 30m).

---

[← Back to workload README](../README.md)
