package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/config"
	"github.com/kbukum/gokit/logging"
)

// DefaultStartBudget bounds [Start] unless [WithStartBudget] overrides it. It matches the setup budget of gokit's component test harness.
const DefaultStartBudget = 30 * time.Second

// StartOption configures [Start].
type StartOption func(*startConfig)

type startConfig struct {
	budget time.Duration
	// Test seams: the lifecycle context's parent (nil means t.Context) and the budget timer.
	base      context.Context
	afterFunc func(d time.Duration, f func()) (stop func() bool)
}

func afterFunc(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }

// WithStartBudget bounds startup in [Start] by d. A nonpositive d uses [DefaultStartBudget].
func WithStartBudget(d time.Duration) StartOption {
	return func(c *startConfig) { c.budget = d }
}

// ErrStartBudget is the cancellation cause of a [Start] whose startup outlasts its setup budget.
var ErrStartBudget = errors.New("bootstrap/testutil: startup exceeded its setup budget")

// Config is the configuration of an App built by [NewApp].
type Config struct {
	config.ServiceConfig
}

// NewApp returns an App named "test" for tests. It discards logs and the startup summary and uses a 5-second graceful shutdown timeout; opts are applied after these defaults, so bootstrap.WithLogger, for example, replaces the discarding logger.
func NewApp(t testing.TB, opts ...bootstrap.Option) *bootstrap.App[*Config] {
	t.Helper()
	logCfg := &logging.Config{Level: "error", Format: "json"}
	logCfg.ApplyDefaults()
	logger, err := logging.New(logCfg, "test", logging.WithWriter(io.Discard))
	if err != nil {
		t.Fatalf("testutil: logger: %v", err)
	}
	cfg := &Config{ServiceConfig: config.ServiceConfig{Name: "test", Version: "test", Environment: "development"}}
	defaults := []bootstrap.Option{bootstrap.WithLogger(logger), bootstrap.WithGracefulTimeout(5 * time.Second)}
	app, err := bootstrap.NewApp(cfg, append(defaults, opts...)...)
	if err != nil {
		t.Fatalf("testutil: new app: %v", err)
	}
	app.Summary.SetWriter(io.Discard)
	return app
}

// Start runs app's startup and fails t if it fails. Startup gets [DefaultStartBudget] unless [WithStartBudget] sets another: if configure, start or ready hooks outlast it, the startup context is canceled with [ErrStartBudget] and startup rolls back. The budget covers startup only. Once started, app keeps its context, which carries t.Context's values but not its cancellation, until Start shuts app down when the test and its subtests finish, reporting a shutdown error on t. Shutdown keeps the startup context's values and is bounded by the App's graceful timeout.
func Start[C bootstrap.Config](t testing.TB, app *bootstrap.App[C], opts ...StartOption) {
	t.Helper()
	cfg := startConfig{afterFunc: afterFunc}
	for _, opt := range opts {
		opt(&cfg)
	}
	budget := cfg.budget
	if budget <= 0 {
		budget = DefaultStartBudget
	}
	base := cfg.base
	if base == nil {
		base = t.Context()
	}
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(base))
	budgetErr := fmt.Errorf("%w (%s)", ErrStartBudget, budget)
	stopTimer := cfg.afterFunc(budget, func() { cancel(budgetErr) })
	err := app.Startup(ctx)
	if !stopTimer() && err == nil {
		// The budget fired as startup returned, so the app may hold a canceled context. The timer callback may not have run yet, so report the budget directly rather than through context.Cause.
		err = errors.Join(budgetErr, app.Shutdown(context.WithoutCancel(ctx)))
	}
	if err != nil {
		cancel(nil)
		t.Fatalf("testutil: startup: %v", err)
		return
	}
	t.Cleanup(func() {
		defer cancel(nil)
		if err := app.Shutdown(context.WithoutCancel(ctx)); err != nil {
			t.Errorf("testutil: shutdown: %v", err)
		}
	})
}

// RegisterListener declares a loopback HTTP [Listener] named name on app and returns it, failing t if app no longer accepts declarations.
func RegisterListener[C bootstrap.Config](t testing.TB, app *bootstrap.App[C], name string) *Listener {
	t.Helper()
	l := NewListener(name)
	if err := app.RegisterListener(name, l); err != nil {
		t.Fatalf("testutil: listen %q: %v", name, err)
	}
	return l
}

// Capture adds a module to app that needs port p, and returns a function that yields the value provided for p once app has started. Calling the function before startup fails t. Because the capture is an ordinary module, startup reports a missing or duplicate provider as a [bootstrap.ModuleError]. Each capture module is named after its port's identity ("capture <port name>@<address>"), so distinct ports with the same name can both be captured; capture each port once per App.
func Capture[C bootstrap.Config, T any](t testing.TB, app *bootstrap.App[C], p *bootstrap.Port[T]) func() T {
	t.Helper()
	c := &capture[T]{port: p}
	if err := app.Use(c); err != nil {
		t.Fatalf("testutil: capture %s: %v", p.Ref(), err)
	}
	return func() T {
		t.Helper()
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.done {
			t.Fatalf("testutil: %s read before the app started", p.Ref())
		}
		return c.value
	}
}

type capture[T any] struct {
	port *bootstrap.Port[T]

	mu    sync.Mutex
	done  bool
	value T
}

func (c *capture[T]) Spec() bootstrap.ModuleSpec {
	return bootstrap.ModuleSpec{Name: fmt.Sprintf("capture %s@%p", c.port.Name(), c.port), Needs: []bootstrap.PortRef{c.port.Ref()}}
}

func (c *capture[T]) Register(_ context.Context, mc *bootstrap.ModuleContext) error {
	v, err := bootstrap.Need(mc, c.port)
	if err != nil {
		return fmt.Errorf("testutil: capture: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value, c.done = v, true
	return nil
}
