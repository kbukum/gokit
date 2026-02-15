package server

import (
	"context"
	"fmt"
	"sort"

	"github.com/skillsenselab/gokit/component"
)

const componentName = "http-server"

// Ensure *Server satisfies component.Component at compile time.
var _ component.Component = (*ServerComponent)(nil)

// Ensure *ServerComponent satisfies component.Describable at compile time.
var _ component.Describable = (*ServerComponent)(nil)

// Ensure *ServerComponent satisfies component.RouteProvider at compile time.
var _ component.RouteProvider = (*ServerComponent)(nil)

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

// Describe returns infrastructure summary info for the bootstrap display.
func (sc *ServerComponent) Describe() component.Description {
	cfg := sc.server.config
	return component.Description{
		Name:    "HTTP Server",
		Type:    "server",
		Details: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Port:    cfg.Port,
	}
}

// Routes returns all registered HTTP routes for the startup summary.
func (sc *ServerComponent) Routes() []component.Route {
	ginRoutes := sc.server.engine.Routes()

	// Sort: API routes first (by path), then system routes
	sort.Slice(ginRoutes, func(i, j int) bool {
		iSys := systemPaths[ginRoutes[i].Path]
		jSys := systemPaths[ginRoutes[j].Path]
		if iSys != jSys {
			return !iSys
		}
		if ginRoutes[i].Path != ginRoutes[j].Path {
			return ginRoutes[i].Path < ginRoutes[j].Path
		}
		return methodOrder(ginRoutes[i].Method) < methodOrder(ginRoutes[j].Method)
	})

	routes := make([]component.Route, 0, len(ginRoutes))
	for _, r := range ginRoutes {
		handler := formatHandlerName(r.Handler)
		if systemPaths[r.Path] {
			handler = handler + " ⚙️"
		}
		routes = append(routes, component.Route{
			Method:  r.Method,
			Path:    r.Path,
			Handler: handler,
		})
	}
	return routes
}
