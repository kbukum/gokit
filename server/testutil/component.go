package testutil

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/server"
	"github.com/kbukum/gokit/testutil"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// Component is a test server component backed by httptest.Server.
// It implements both component.Component and testutil.TestComponent.
type Component struct {
	srv     *server.Server
	ts      *httptest.Server
	log     *logger.Logger
	started bool
	mu      sync.RWMutex
}

var _ component.Component = (*Component)(nil)
var _ testutil.TestComponent = (*Component)(nil)

// NewComponent creates a new test server component.
func NewComponent() *Component {
	log := logger.NewDefault("server-test")
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

func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("component already started")
	}

	// Apply middleware and get the final handler
	c.srv.ApplyMiddleware()
	c.ts = httptest.NewServer(c.srv.Handler())
	c.started = true
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started || c.ts == nil {
		return nil
	}
	c.ts.Close()
	c.started = false
	return nil
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

// Reset recreates the server with a fresh Gin engine and routes.
func (c *Component) Reset(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return fmt.Errorf("component not started")
	}

	// Close existing
	if c.ts != nil {
		c.ts.Close()
	}

	// Recreate server
	cfg := &server.Config{Host: "127.0.0.1", Port: 0, Enabled: true}
	cfg.ApplyDefaults()
	c.srv = server.New(cfg, c.log)

	c.srv.ApplyMiddleware()
	c.ts = httptest.NewServer(c.srv.Handler())
	return nil
}

// Snapshot is a no-op for the server component (servers are stateless).
func (c *Component) Snapshot(_ context.Context) (interface{}, error) {
	return nil, nil
}

// Restore is a no-op for the server component.
func (c *Component) Restore(_ context.Context, _ interface{}) error {
	return nil
}
