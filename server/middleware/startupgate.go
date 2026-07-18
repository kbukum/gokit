package middleware

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// StartupGate blocks API requests with 503 Service Unavailable until MarkReady is called.
// Infrastructure paths (/health, /ready, /alive, /info, /version, /metrics) are always allowed through
// so orchestrators can probe the service during startup.
type StartupGate struct {
	ready     atomic.Bool
	skipPaths map[string]struct{}
}

// StartupGateOption configures a StartupGate.
type StartupGateOption func(*StartupGate)

// WithSkipStartupPaths adds paths that bypass the startup gate.
func WithSkipStartupPaths(paths ...string) StartupGateOption {
	return func(g *StartupGate) {
		for _, p := range paths {
			g.skipPaths[p] = struct{}{}
		}
	}
}

// NewStartupGate creates a gate in the "not ready" state. By default, /health, /ready, /alive,
// /info, /version, and /metrics are always allowed through.
func NewStartupGate(opts ...StartupGateOption) *StartupGate {
	g := &StartupGate{
		skipPaths: map[string]struct{}{
			"/health":  {},
			"/ready":   {},
			"/alive":   {},
			"/info":    {},
			"/version": {},
			"/metrics": {},
		},
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// MarkReady signals that the service is fully initialized
// and API traffic should be allowed through.
func (g *StartupGate) MarkReady() { g.ready.Store(true) }

// IsReady reports whether the gate has been opened.
func (g *StartupGate) IsReady() bool { return g.ready.Load() }

// Middleware returns a gin middleware that returns 503 for non-infrastructure paths until the gate is marked ready.
func (g *StartupGate) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.ready.Load() {
			c.Next()
			return
		}
		if _, ok := g.skipPaths[c.Request.URL.Path]; ok {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"error": "service is starting up",
		})
	}
}
