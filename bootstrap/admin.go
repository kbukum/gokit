package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/observability"
)

// AdminComponentName is the registry name of the admin listener [WithAdmin] adds.
const AdminComponentName = "admin"

// ErrInvalidAdminConfig reports an [AdminConfig] that fails validation.
var ErrInvalidAdminConfig = errors.New("bootstrap: invalid admin config")

// AdminConfig configures the diagnostics listener [WithAdmin] adds. It serves without TLS, so it binds only a trusted address: the default host is loopback, and port zero asks the OS for an ephemeral port.
type AdminConfig struct {
	// Host is a loopback or private IP address; empty means 127.0.0.1.
	Host string `yaml:"host" mapstructure:"host"`
	Port int    `yaml:"port" mapstructure:"port"`
	// Pprof serves net/http/pprof under /debug/pprof/.
	Pprof bool `yaml:"pprof" mapstructure:"pprof"`
	// HealthTimeout bounds the component health checks behind /readyz; zero means 2s.
	HealthTimeout time.Duration `yaml:"health_timeout" mapstructure:"health_timeout"`
	// Metrics serves /metrics; nil means [observability.RuntimeMetricsHandler].
	Metrics http.Handler `yaml:"-" mapstructure:"-"`
}

// Validate reports a host that is not a loopback or private IP address, an out-of-range port or a negative health timeout.
func (c AdminConfig) Validate() error {
	var errs []error
	if c.Host != "" {
		if ip, err := netip.ParseAddr(c.Host); err != nil || (!ip.IsLoopback() && !ip.IsPrivate()) {
			errs = append(errs, fmt.Errorf("%w: host %q must be a loopback or private IP address", ErrInvalidAdminConfig, c.Host))
		}
	}
	if c.Port < 0 || c.Port > 65535 {
		errs = append(errs, fmt.Errorf("%w: port %d must be between 0 and 65535", ErrInvalidAdminConfig, c.Port))
	}
	if c.HealthTimeout < 0 {
		errs = append(errs, fmt.Errorf("%w: health_timeout must not be negative", ErrInvalidAdminConfig))
	}
	return errors.Join(errs...)
}

// WithAdmin adds a diagnostics listener the App owns. It serves:
//
//   - GET /livez: 200 while the process serves.
//   - GET /readyz: a [Readiness] body; 503 while starting, draining, or when a component is unhealthy, and 200 when every component is healthy ("ready") or some are degraded ("degraded").
//   - GET /metrics: Prometheus metrics.
//   - /debug/pprof/: when Pprof is set.
//
// The listener is registered before every other component in the admin shutdown phase, so it starts first and stops last; it does not quiesce, so probes and metrics stay available while the App drains. Its component name is [AdminComponentName].
func WithAdmin(cfg AdminConfig) Option {
	return func(o *appOptions) {
		o.admin = &cfg
	}
}

// ReadinessStatus is the status /readyz reports.
type ReadinessStatus string

// Readiness statuses.
const (
	ReadinessStarting ReadinessStatus = "starting"
	ReadinessReady    ReadinessStatus = "ready"
	ReadinessDegraded ReadinessStatus = "degraded"
	ReadinessNotReady ReadinessStatus = "not_ready"
	ReadinessDraining ReadinessStatus = "draining"
)

// Readiness is the /readyz response body. Components is empty while starting or draining.
type Readiness struct {
	Service    string             `json:"service"`
	Status     ReadinessStatus    `json:"status"`
	Components []component.Health `json:"components,omitempty"`
}

// lifecycleState is what readiness reports before component health is consulted.
type lifecycleState int32

const (
	stateStarting lifecycleState = iota
	stateServing
	stateDraining
)

// AdminAddr returns the admin listener's bound address, or nil without [WithAdmin] or while it is not running.
func (a *App[C]) AdminAddr() net.Addr {
	if a.admin == nil {
		return nil
	}
	return a.admin.addr()
}

// readiness evaluates /readyz from the lifecycle state and component health.
func (a *App[C]) readiness(ctx context.Context) Readiness {
	r := Readiness{Service: a.Name}
	switch lifecycleState(a.state.Load()) {
	case stateStarting:
		r.Status = ReadinessStarting
		return r
	case stateDraining:
		r.Status = ReadinessDraining
		return r
	case stateServing:
	}
	r.Status = ReadinessReady
	for _, h := range a.Components.HealthAll(ctx) {
		r.Components = append(r.Components, h)
		switch h.Status {
		case component.StatusUnhealthy:
			r.Status = ReadinessNotReady
		case component.StatusDegraded:
			if r.Status == ReadinessReady {
				r.Status = ReadinessDegraded
			}
		case component.StatusHealthy:
		}
	}
	return r
}

func newAdmin(cfg AdminConfig, readiness func(context.Context) Readiness) (*adminComponent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.HealthTimeout == 0 {
		cfg.HealthTimeout = 2 * time.Second
	}
	metrics := cfg.Metrics
	if metrics == nil {
		var err error
		if metrics, err = observability.RuntimeMetricsHandler(); err != nil {
			return nil, fmt.Errorf("bootstrap: admin metrics: %w", err)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
		defer cancel()
		body := readiness(ctx)
		code := http.StatusOK
		if body.Status != ReadinessReady && body.Status != ReadinessDegraded {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, body)
	})
	mux.Handle("GET /metrics", metrics)
	if cfg.Pprof {
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("POST /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	}
	return &adminComponent{cfg: cfg, handler: mux}, nil
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body) //nolint:errchkjson // the client may have gone; nothing to report
}

// adminComponent is the App-owned diagnostics listener.
type adminComponent struct {
	cfg     AdminConfig
	handler http.Handler

	mu       sync.Mutex
	listener net.Listener
	srv      *http.Server
	done     chan error
}

func (c *adminComponent) Name() string { return AdminComponentName }

func (c *adminComponent) ShutdownPhase() component.ShutdownPhase { return component.PhaseAdmin }

func (c *adminComponent) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.srv != nil {
		return errors.New("bootstrap: admin listener already started")
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", net.JoinHostPort(c.cfg.Host, strconv.Itoa(c.cfg.Port)))
	if err != nil {
		return fmt.Errorf("bootstrap: admin listener: %w", err)
	}
	srv := &http.Server{Handler: c.handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	c.listener, c.srv, c.done = listener, srv, done
	return nil
}

func (c *adminComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	srv, done := c.srv, c.done
	c.listener, c.srv, c.done = nil, nil, nil
	c.mu.Unlock()
	if srv == nil {
		return nil
	}
	err := srv.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, srv.Close())
	}
	if serveErr := <-done; !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, serveErr)
	}
	return err
}

func (c *adminComponent) Health(context.Context) component.Health {
	if c.addr() == nil {
		return component.Health{Name: AdminComponentName, Status: component.StatusUnhealthy, Message: "not serving"}
	}
	return component.Health{Name: AdminComponentName, Status: component.StatusHealthy}
}

func (c *adminComponent) addr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.listener == nil {
		return nil
	}
	return c.listener.Addr()
}
