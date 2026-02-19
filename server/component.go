package server

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kbukum/gokit/component"
)

const componentName = "http-server"

// Ensure *Server satisfies component.Component at compile time.
var _ component.Component = (*Component)(nil)

// Ensure *Component satisfies component.Describable at compile time.
var _ component.Describable = (*Component)(nil)

// Ensure *Component satisfies component.RouteProvider at compile time.
var _ component.RouteProvider = (*Component)(nil)

// Component wraps Server to implement component.Component.
type Component struct {
	server *Server
}

// NewComponent returns a component.Component backed by the given Server.
func NewComponent(s *Server) *Component {
	return &Component{server: s}
}

// Name returns the component name used for registration.
func (sc *Component) Name() string { return componentName }

// Start starts the underlying HTTP server.
func (sc *Component) Start(ctx context.Context) error {
	return sc.server.Start(ctx)
}

// Stop gracefully shuts down the underlying HTTP server.
func (sc *Component) Stop(ctx context.Context) error {
	return sc.server.Stop(ctx)
}

// Health returns the health status of the server.
func (sc *Component) Health(ctx context.Context) component.Health {
	if sc.server.httpServer != nil {
		return component.Health{
			Name:   componentName,
			Status: component.StatusHealthy,
		}
	}
	return component.Health{
		Name:    componentName,
		Status:  component.StatusUnhealthy,
		Message: "HTTP server not initialized",
	}
}

// Describe returns infrastructure summary info for the bootstrap display.
func (sc *Component) Describe() component.Description {
	cfg := sc.server.config
	details := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Include mounted ConnectRPC/gRPC services
	if mounts := sc.server.Mounts(); len(mounts) > 0 {
		services := extractServiceNames(mounts)
		if len(services) > 0 {
			details += " + ConnectRPC: " + strings.Join(services, ", ")
		}
	}

	return component.Description{
		Name:    "HTTP Server",
		Type:    "server",
		Details: details,
		Port:    cfg.Port,
	}
}

// Routes returns all registered HTTP routes for the startup summary.
func (sc *Component) Routes() []component.Route {
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

	routes := make([]component.Route, 0, len(ginRoutes)+len(sc.server.mounts))
	for _, r := range ginRoutes {
		handler := formatHandlerName(r.Handler)
		if systemPaths[r.Path] {
			handler += " ⚙️"
		}
		routes = append(routes, component.Route{
			Method:  r.Method,
			Path:    r.Path,
			Handler: handler,
		})
	}

	// Append mounted handlers (ConnectRPC services)
	for _, m := range sc.server.mounts {
		routes = append(routes, component.Route{
			Method:  "CONNECT",
			Path:    m.Pattern,
			Handler: extractSingleServiceName(m.Pattern),
		})
	}

	return routes
}
