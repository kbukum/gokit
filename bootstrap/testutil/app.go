package testutil

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/config"
	"github.com/kbukum/gokit/logging"
)

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

// Start runs app's startup and fails t if it fails. On success it shuts app down when the test and its subtests finish, reporting a shutdown error on t. Startup uses a background context so components keep running until that shutdown, after t.Context is canceled.
func Start[C bootstrap.Config](t testing.TB, app *bootstrap.App[C]) {
	t.Helper()
	if err := app.Startup(context.Background()); err != nil {
		t.Fatalf("testutil: startup: %v", err)
	}
	t.Cleanup(func() {
		if err := app.Shutdown(context.Background()); err != nil {
			t.Errorf("testutil: shutdown: %v", err)
		}
	})
}

// Listen declares a loopback HTTP [Listener] named name on app and returns it, failing t if app no longer accepts declarations.
func Listen[C bootstrap.Config](t testing.TB, app *bootstrap.App[C], name string) *Listener {
	t.Helper()
	l := NewListener(name)
	if err := app.Listen(name, l); err != nil {
		t.Fatalf("testutil: listen %q: %v", name, err)
	}
	return l
}

// Capture adds a module named "capture <port name>" to app that needs port p, and returns a function that yields the value provided for p once app has started. Calling the function before startup fails t. Because the capture is an ordinary module, startup reports a missing or duplicate provider as a [bootstrap.ModuleError]. Capture each port once per App.
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
	return bootstrap.ModuleSpec{Name: "capture " + c.port.Name(), Needs: []bootstrap.PortRef{c.port.Ref()}}
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
