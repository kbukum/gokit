package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/server/endpoint"
	"github.com/kbukum/gokit/server/middleware"
)

// Server is a unified HTTP server backed by Gin with optional support for
// additional http.Handler mounts (e.g. Connect-Go / gRPC) on the same port.
type Server struct {
	httpServer *http.Server
	engine     *gin.Engine
	mux        *http.ServeMux
	config     Config
	log        *logger.Logger
	mounts     []MountedHandler // tracked for summary display
}

// MountedHandler records a handler mounted on the ServeMux.
type MountedHandler struct {
	Pattern string
	Label   string // optional human-readable label
}

// New creates a new Server. The Gin engine is created but no middleware is
// applied yet — call ApplyDefaults on the config first if needed.
func New(cfg *Config, log *logger.Logger) *Server {
	// Set Gin mode based on global zerolog level.
	if zerolog.GlobalLevel() <= zerolog.DebugLevel {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	mux := http.NewServeMux()

	// Mount Gin as the fallback handler on the root mux.
	mux.Handle("/", engine)

	// Wrap with h2c for HTTP/2 cleartext (required for gRPC without TLS).
	h2s := &http2.Server{
		MaxConcurrentStreams: 250,
		IdleTimeout:          120 * time.Second,
	}
	handler := h2c.NewHandler(mux, h2s)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
	}

	return &Server{
		httpServer: httpServer,
		engine:     engine,
		mux:        mux,
		config:     *cfg,
		log:        log.WithComponent("server"),
	}
}

// GinEngine returns the underlying Gin engine for route registration.
func (s *Server) GinEngine() *gin.Engine {
	return s.engine
}

// Handle mounts an http.Handler at the given pattern on the root ServeMux.
// Use this to add Connect-Go or any other handler alongside Gin.
// The pattern must include a trailing slash for subtree matches (e.g. "/grpc.health.v1.Health/").
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
	s.mounts = append(s.mounts, MountedHandler{Pattern: pattern})
	s.log.Debug("Handler mounted", map[string]interface{}{
		"pattern": pattern,
	})
}

// Mounts returns all handlers mounted on the ServeMux (excluding Gin root).
func (s *Server) Mounts() []MountedHandler {
	return s.mounts
}

// Handler returns the composed http.Handler (with middleware and h2c).
// Call ApplyMiddleware() first to ensure the middleware stack is applied.
// This is useful for testing with httptest.NewServer.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// Start binds the port and begins serving. It returns once the listener is
// bound so the caller knows the port is ready; serving continues in a goroutine.
func (s *Server) Start(ctx context.Context) error {
	s.log.Info("Starting HTTP server", map[string]interface{}{
		"addr": s.httpServer.Addr,
	})

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("server failed to bind %s: %w", s.httpServer.Addr, err)
	}

	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.log.Error("Server error", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}()

	s.log.Info("HTTP server started", map[string]interface{}{
		"addr": s.httpServer.Addr,
	})
	return nil
}

// Stop gracefully shuts down the server with a 5-second deadline.
func (s *Server) Stop(ctx context.Context) error {
	s.log.Info("Shutting down HTTP server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.log.Error("Server shutdown error", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("server shutdown error: %w", err)
	}

	s.log.Info("HTTP server shut down successfully")
	return nil
}

// Addr returns the configured listen address.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// ApplyMiddleware applies the standard middleware stack at the handler level
// so it covers ALL routes — both Gin REST endpoints and ConnectRPC services
// mounted via Handle().
func (s *Server) ApplyMiddleware() {
	stack := []middleware.Middleware{
		middleware.Recovery(s.log),
		middleware.RequestID(),
		middleware.CORS(&s.config.CORS),
		middleware.RequestLogger(s.log),
	}
	if s.config.MaxBodySize != "" {
		stack = append(stack, middleware.BodySizeLimit(s.config.MaxBodySize))
	}

	h2s := &http2.Server{
		MaxConcurrentStreams: 250,
		IdleTimeout:          120 * time.Second,
	}
	s.httpServer.Handler = h2c.NewHandler(middleware.Chain(stack...)(s.mux), h2s)
}

// RegisterDefaultEndpoints registers the standard /health, /info, and /metrics endpoints.
func (s *Server) RegisterDefaultEndpoints(serviceName string, checker endpoint.HealthChecker) {
	s.engine.GET("/health", endpoint.Health(serviceName, checker))
	s.engine.GET("/info", endpoint.Info(serviceName))
	s.engine.GET("/metrics", endpoint.Metrics())
}

// ApplyDefaults applies the standard middleware stack and registers default endpoints.
func (s *Server) ApplyDefaults(serviceName string, checker endpoint.HealthChecker) {
	s.ApplyMiddleware()
	s.RegisterDefaultEndpoints(serviceName, checker)
}
