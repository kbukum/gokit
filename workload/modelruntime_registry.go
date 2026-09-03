package workload

import (
	"fmt"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/provider/namedregistry"
)

// ModelRuntimeConfig holds provider-agnostic model runtime configuration.
type ModelRuntimeConfig struct {
	// Provider selects the registered backend by name. It has no default: the
	// backend-neutral port keeps no built-in runtime, and every provider is an
	// opt-in module, so callers must select one explicitly (for example
	// [ProviderDMR]).
	Provider string `mapstructure:"provider" json:"provider"`
}

// Validate checks that the core configuration is valid.
func (c *ModelRuntimeConfig) Validate() error {
	if c.Provider == "" {
		return fmt.Errorf("workload: model runtime provider is required")
	}
	return nil
}

// ModelRuntimeFactory creates a [ModelRuntime] from core config. Provider-specific
// configuration is captured by the typed provider Register function, keeping
// runtime selection free of untyped config blobs.
type ModelRuntimeFactory func(cfg ModelRuntimeConfig, log *logging.Logger) (ModelRuntime, error)

// ModelRuntimeRegistry stores model runtime factories by provider name.
//
// Registries are explicit, isolated, and thread-safe. Use
// [NewModelRuntimeRegistry] to create one, then call provider-specific Register
// functions (for example [github.com/kbukum/gokit/workload/dmr.Register]) to
// populate it before passing it to [NewModelRuntime].
type ModelRuntimeRegistry struct {
	inner *namedregistry.Registry[ModelRuntimeFactory]
}

// NewModelRuntimeRegistry creates an isolated model runtime factory registry.
func NewModelRuntimeRegistry() *ModelRuntimeRegistry {
	return &ModelRuntimeRegistry{inner: namedregistry.New[ModelRuntimeFactory]("workload-modelruntime")}
}

// Register stores a model runtime backend factory for the given provider name.
// It returns an error if name or factory are invalid, or if a duplicate name is
// registered.
func (r *ModelRuntimeRegistry) Register(name string, f ModelRuntimeFactory) error {
	return r.inner.Register(name, f)
}

// Get returns a model runtime factory by provider name.
func (r *ModelRuntimeRegistry) Get(name string) (ModelRuntimeFactory, bool) {
	return r.inner.Get(name)
}

// Names returns the registered provider names in deterministic (sorted) order.
func (r *ModelRuntimeRegistry) Names() []string {
	return r.inner.Names()
}

// NewModelRuntime creates a [ModelRuntime] using the provided registry. The
// registry is mandatory: pass an explicit *ModelRuntimeRegistry with the desired
// providers registered (for example via dmr.Register). A nil logger is defaulted.
func NewModelRuntime(reg *ModelRuntimeRegistry, cfg ModelRuntimeConfig, log *logging.Logger) (ModelRuntime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, fmt.Errorf("workload: model runtime registry is nil")
	}

	l := log
	if l == nil {
		l = logging.NewDefault("workload-modelruntime") //nolint:contextcheck // default logger construction has no request-scoped operation
	}
	l = l.WithComponent("workload-modelruntime")

	f, ok := reg.Get(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("workload: unsupported model runtime provider %q (not registered)", cfg.Provider)
	}

	l.Info("initializing model runtime", map[string]any{"provider": cfg.Provider})
	return f(cfg, l)
}
