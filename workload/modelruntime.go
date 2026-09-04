package workload

import (
	"context"
	"time"
)

// ModelRuntime runs and serves models as managed workloads, independent of the
// underlying runtime (Docker Model Runner, Ollama, an OCI registry, Hugging
// Face, …). It is the runtime-neutral seam a backend implements; a concrete
// backend is selected via [ModelRuntimeRegistry] and configuration, mirroring
// the [Manager]/[FactoryRegistry] provider pattern.
//
// A model runtime manages models *within* an already-running runtime service
// (pull, serve, unload) and reports where to send inference requests. It does
// not deploy the runtime itself, and it does not perform inference — callers
// send requests to [Endpoint] using the appropriate client for its [Endpoint.API]
// dialect (the `llm` module owns inference).
//
// The core contract is deliberately narrow — start, stop, and address a model.
// Reachability probing, usage reporting, listing, and deletion are optional
// capabilities a backend opts into via [ModelHealthChecker], [ModelStatsReporter],
// [ModelLister], and [ModelRemover]; callers type-assert for the ones they need.
type ModelRuntime interface {
	// Start ensures the model described by spec is pulled and ready to serve,
	// returning a handle with its inference endpoint. Idempotent: starting an
	// already-ready model does not re-download it.
	Start(ctx context.Context, spec ModelSpec) (*ModelHandle, error)

	// Stop releases a running model (best-effort unload). Idempotent: stopping
	// an unknown or already-stopped model returns nil.
	Stop(ctx context.Context, model string) error

	// Endpoint returns the inference endpoint for a model. It is a pure address
	// computation over the model id: it does not verify the model has been
	// started or exists and makes no network call, so it errors only on invalid
	// input (such as an empty model). Callers that need existence or readiness
	// guarantees use [ModelStatsReporter] or [ModelLister].
	Endpoint(ctx context.Context, model string) (*Endpoint, error)
}

// ModelHealthChecker is optionally implemented by runtimes that can probe the
// backend's management API. Health reports that the management surface is
// reachable; it does not guarantee any particular model is loaded or that the
// inference engine is ready to serve.
type ModelHealthChecker interface {
	Health(ctx context.Context) error
}

// ModelStatsReporter is optionally implemented by runtimes that can report
// per-model resource usage.
type ModelStatsReporter interface {
	Stats(ctx context.Context, model string) (*ModelStats, error)
}

// ModelLister is optionally implemented by runtimes that can enumerate the
// models available locally.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// ModelRemover is optionally implemented by runtimes that can delete a local
// model from disk.
type ModelRemover interface {
	RemoveModel(ctx context.Context, model string) error
}

// Inference API dialect spoken by a runtime [Endpoint].
const (
	APIOpenAI    = "openai"
	APIAnthropic = "anthropic"
	APIOllama    = "ollama"
)

// Well-known model runtime provider names.
const (
	ProviderDMR    = "dmr"
	ProviderOllama = "ollama"
)

// ModelSpec identifies a model to run and how to provision it. It is
// runtime-neutral: Ref is a registry reference the backend understands, such as
// a Docker Hub model ("ai/smollm2"), a Hugging Face ref ("hf.co/org/model"), or
// an OCI reference.
type ModelSpec struct {
	Ref         string            // Registry reference (required)
	ContextSize int               // Context window in tokens (0 = runtime default)
	Resources   *ResourceConfig   // Optional compute constraints; backends that cannot honor them reject a non-nil value
	Labels      map[string]string // Grouping and filtering; advisory, not all backends persist them
	Metadata    map[string]any    // Backend-specific extras (opaque, documented per backend)
}

// ModelHandle describes a started model and how to reach it.
type ModelHandle struct {
	Model    string   // Canonical model id to use in inference requests
	Status   string   // Lifecycle state (StatusRunning, StatusStopped, …)
	Endpoint Endpoint // Where to send inference requests
}

// Endpoint is an inference endpoint exposed by a model runtime.
type Endpoint struct {
	BaseURL string // Base URL, e.g. "http://localhost:12434/engines/v1"
	Model   string // Model id to pass in request bodies
	API     string // Dialect the base URL speaks: APIOpenAI, APIAnthropic, APIOllama
}

// ModelInfo summarizes a model known to the runtime.
type ModelInfo struct {
	ID        string
	Ref       string
	SizeBytes int64
	Labels    map[string]string
	Created   time.Time
}

// ModelStats reports resource usage for a model. Fields a backend cannot
// determine are left zero.
type ModelStats struct {
	Loaded        bool  // Model is loaded in memory
	MemoryBytes   int64 // Resident memory
	DiskSizeBytes int64 // On-disk model size
}
