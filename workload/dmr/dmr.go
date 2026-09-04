package dmr

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/workload"
)

// Register installs the Docker Model Runner backend into the supplied model
// runtime registry under [workload.ProviderDMR], capturing the given typed
// config. Call this once at startup before invoking [workload.NewModelRuntime].
// It returns an error if the registry is nil, the config is invalid, or the
// provider name is already registered.
func Register(registry *workload.ModelRuntimeRegistry, cfg Config) error {
	if registry == nil {
		return fmt.Errorf("dmr: model runtime registry is nil")
	}
	c := cfg
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return err
	}
	return registry.Register(workload.ProviderDMR, func(_ workload.ModelRuntimeConfig, log *logging.Logger) (workload.ModelRuntime, error) {
		return NewRuntime(&c, log)
	})
}

// Runtime implements [workload.ModelRuntime] (plus the optional
// [workload.ModelHealthChecker], [workload.ModelStatsReporter],
// [workload.ModelLister], and [workload.ModelRemover] capabilities) against a
// Docker Model Runner daemon.
type Runtime struct {
	client *client
	log    *logging.Logger
}

var (
	_ workload.ModelRuntime       = (*Runtime)(nil)
	_ workload.ModelHealthChecker = (*Runtime)(nil)
	_ workload.ModelStatsReporter = (*Runtime)(nil)
	_ workload.ModelLister        = (*Runtime)(nil)
	_ workload.ModelRemover       = (*Runtime)(nil)
)

// NewRuntime creates a Docker Model Runner runtime from config. The config is
// copied before defaults are applied, so a caller may reuse the same value
// concurrently; a nil config or logger is rejected and defaulted respectively.
func NewRuntime(cfg *Config, log *logging.Logger) (*Runtime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("dmr: config is nil")
	}
	c := *cfg
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if log == nil {
		log = logging.NewDefault("dmr")
	}
	cl, err := newClient(&c)
	if err != nil {
		return nil, err
	}
	return &Runtime{client: cl, log: log.WithComponent("dmr")}, nil
}

// Start pulls the model, applies any supported provisioning settings, and
// returns its inference endpoint. DMR loads models lazily on first request, so a
// successful pull leaves the model ready to serve. A non-zero ContextSize is
// applied via DMR's per-model configuration endpoint; Resources are rejected
// because DMR has no per-model compute-constraint control. Labels and Metadata
// are caller-side annotations that DMR does not persist.
func (r *Runtime) Start(ctx context.Context, spec workload.ModelSpec) (*workload.ModelHandle, error) {
	if spec.Ref == "" {
		return nil, fmt.Errorf("dmr: model ref is required")
	}
	if spec.Resources != nil {
		return nil, fmt.Errorf("dmr: per-model Resources are not supported by Docker Model Runner")
	}
	// Validate the context size before any pull side effect: DMR's context-size
	// is a signed 32-bit wire field, so a negative value is nonsensical and an
	// over-range value would only be rejected by the daemon after a wasted pull.
	if spec.ContextSize < 0 {
		return nil, fmt.Errorf("dmr: context size must not be negative, got %d", spec.ContextSize)
	}
	if spec.ContextSize > math.MaxInt32 {
		return nil, fmt.Errorf("dmr: context size %d exceeds the DMR maximum of %d", spec.ContextSize, math.MaxInt32)
	}
	r.log.InfoCtx(ctx, "pulling model", map[string]any{"ref": spec.Ref})
	if err := r.client.pull(ctx, spec.Ref); err != nil {
		return nil, fmt.Errorf("dmr: start %q: %w", spec.Ref, err)
	}
	if spec.ContextSize > 0 {
		if err := r.client.configure(ctx, spec.Ref, spec.ContextSize); err != nil {
			return nil, fmt.Errorf("dmr: configure %q: %w", spec.Ref, err)
		}
	}
	return &workload.ModelHandle{
		Model:    spec.Ref,
		Status:   workload.StatusRunning,
		Endpoint: r.client.endpoint(spec.Ref),
	}, nil
}

// Stop unloads a running model via DMR's unload endpoint. Idempotent: unloading
// an unknown or already-idle model is a no-op. Use RemoveModel to delete a model
// from disk.
func (r *Runtime) Stop(ctx context.Context, model string) error {
	if model == "" {
		return fmt.Errorf("dmr: model is required")
	}
	if err := r.client.unload(ctx, model); err != nil {
		return fmt.Errorf("dmr: stop %q: %w", model, err)
	}
	return nil
}

// Health verifies the Docker Model Runner daemon is reachable.
func (r *Runtime) Health(ctx context.Context) error {
	if err := r.client.health(ctx); err != nil {
		return fmt.Errorf("dmr: health: %w", err)
	}
	return nil
}

// Endpoint returns the OpenAI-compatible inference endpoint for a model. The
// endpoint is stable regardless of load state, so this makes no network call.
func (r *Runtime) Endpoint(_ context.Context, model string) (*workload.Endpoint, error) {
	if model == "" {
		return nil, fmt.Errorf("dmr: model is required")
	}
	ep := r.client.endpoint(model)
	return &ep, nil
}

// Stats reports on-disk size and live load state for a model. DMR's REST surface
// does not expose resident memory, so MemoryBytes is left zero.
func (r *Runtime) Stats(ctx context.Context, model string) (*workload.ModelStats, error) {
	if model == "" {
		return nil, fmt.Errorf("dmr: model is required")
	}
	m, err := r.client.get(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("dmr: stats %q: %w", model, err)
	}
	loaded, err := r.client.loaded(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("dmr: stats %q: %w", model, err)
	}
	return &workload.ModelStats{Loaded: loaded, DiskSizeBytes: m.sizeBytes()}, nil
}

// ListModels returns the models available locally.
func (r *Runtime) ListModels(ctx context.Context) ([]workload.ModelInfo, error) {
	objs, err := r.client.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("dmr: list models: %w", err)
	}
	out := make([]workload.ModelInfo, 0, len(objs))
	for i := range objs {
		o := &objs[i]
		info := workload.ModelInfo{ID: o.ID, Ref: o.ref(), SizeBytes: o.sizeBytes()}
		if o.Created > 0 {
			info.Created = time.Unix(o.Created, 0)
		}
		out = append(out, info)
	}
	return out, nil
}

// RemoveModel deletes a local model from disk.
func (r *Runtime) RemoveModel(ctx context.Context, model string) error {
	if model == "" {
		return fmt.Errorf("dmr: model is required")
	}
	if err := r.client.remove(ctx, model); err != nil {
		return fmt.Errorf("dmr: remove %q: %w", model, err)
	}
	return nil
}
