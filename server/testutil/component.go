package testutil

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server"
	"github.com/kbukum/gokit/testutil"
)

// Component is a test server component backed by httptest.Server.
// It implements both component.Component and testutil.TestComponent.
type Component struct {
	srv        *server.Server
	ts         *httptest.Server
	log        *logging.Logger
	started    bool
	configured bool
	mu         sync.RWMutex
	lifecycle  sync.Mutex
}

var (
	_ component.Component    = (*Component)(nil)
	_ testutil.TestComponent = (*Component)(nil)
)

// NewComponent creates a new test server component.
func NewComponent() *Component {
	gin.SetMode(gin.TestMode)
	log := logging.NewDefault("server-test")
	cfg := &server.Config{
		Host:    "127.0.0.1",
		Port:    0,
		Enabled: true,
	}
	cfg.ApplyDefaults()

	return &Component{
		srv: server.New(cfg, log),
		log: log,
	}
}

// GinEngine returns the Gin engine for registering routes.
func (c *Component) GinEngine() *gin.Engine {
	return c.srv.GinEngine()
}

// Handle mounts an http.Handler on the server's ServeMux (for ConnectRPC, etc).
func (c *Component) Handle(pattern string, handler http.Handler) {
	c.srv.Handle(pattern, handler)
}

// Server returns the underlying *server.Server.
func (c *Component) Server() *server.Server {
	return c.srv
}

// BaseURL returns the test server's base URL (e.g. "http://127.0.0.1:PORT").
// Returns empty string if not started.
func (c *Component) BaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.ts == nil {
		return ""
	}
	return c.ts.URL
}

// --- component.Component ---

func (c *Component) Name() string { return "server-test" }

func (c *Component) Start(ctx context.Context) error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("component already started")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Apply middleware and get the final handler
	if !c.configured {
		c.srv.ApplyMiddleware() //nolint:contextcheck // route setup has no request-scoped operation
		c.configured = true
	}
	c.ts = httptest.NewServer(c.srv.Handler())
	c.started = true
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	c.mu.Lock()
	if !c.started || c.ts == nil {
		c.mu.Unlock()
		return nil
	}
	srv := c.ts
	c.ts = nil
	c.started = false
	c.mu.Unlock()
	return testutil.CloseHTTPServer(ctx, srv)
}

func (c *Component) Health(_ context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "not started"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

// --- testutil.TestComponent ---

// Reset preserves this stateless server's handlers and origin. Reset application fixtures separately, with requests quiesced; use Stop/Start for an owned restart.
func (c *Component) Reset(ctx context.Context) error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return fmt.Errorf("component not started")
	}

	return ctx.Err()
}

// Snapshot is a no-op for the server component (servers are stateless).
//
//nolint:nilnil // documented no-op contract: stateless component has no snapshot.
func (c *Component) Snapshot(_ context.Context) (any, error) {
	return nil, nil
}

// Restore is a no-op for the server component.
func (c *Component) Restore(_ context.Context, _ any) error {
	return nil
}
