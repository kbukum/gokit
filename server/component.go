package server

import (
	"context"

	"github.com/skillsenselab/gokit/component"
)

const componentName = "http-server"

// Ensure *Server satisfies component.Component at compile time.
var _ component.Component = (*ServerComponent)(nil)

// ServerComponent wraps Server to implement component.Component.
type ServerComponent struct {
	server *Server
}

// NewComponent returns a component.Component backed by the given Server.
func NewComponent(s *Server) *ServerComponent {
	return &ServerComponent{server: s}
}

// Name returns the component name used for registration.
func (sc *ServerComponent) Name() string { return componentName }

// Start starts the underlying HTTP server.
func (sc *ServerComponent) Start(ctx context.Context) error {
	return sc.server.Start(ctx)
}

// Stop gracefully shuts down the underlying HTTP server.
func (sc *ServerComponent) Stop(ctx context.Context) error {
	return sc.server.Stop(ctx)
}

// Health returns the health status of the server.
func (sc *ServerComponent) Health(ctx context.Context) component.ComponentHealth {
	if sc.server.httpServer != nil {
		return component.ComponentHealth{
			Name:   componentName,
			Status: component.StatusHealthy,
		}
	}
	return component.ComponentHealth{
		Name:    componentName,
		Status:  component.StatusUnhealthy,
		Message: "HTTP server not initialized",
	}
}
