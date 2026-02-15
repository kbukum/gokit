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

	"github.com/skillsenselab/gokit/logger"
	"github.com/skillsenselab/gokit/server/endpoint"
	"github.com/skillsenselab/gokit/server/middleware"
)

// Server is a unified HTTP server backed by Gin with optional support for
// additional http.Handler mounts (e.g. Connect-Go / gRPC) on the same port.
type Server struct {
	httpServer *http.Server
	engine     *gin.Engine
	mux        *http.ServeMux
	config     Config
	log        *logger.Logger
}

// New creates a new Server. The Gin engine is created but no middleware is
// applied yet — call ApplyDefaults on the config first if needed.
func New(cfg Config, log *logger.Logger) *Server {
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
		IdleTimeout:         120 * time.Second,
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
		config:     cfg,
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
	s.log.Debug("Handler mounted", map[string]interface{}{
		"pattern": pattern,
	})
}

// Start binds the port and begins serving. It returns once the listener is
// bound so the caller knows the port is ready; serving continues in a goroutine.
func (s *Server) Start(ctx context.Context) error {
	s.log.Info("Starting HTTP server", map[string]interface{}{
		"addr": s.httpServer.Addr,
	})

	listener, err := net.Listen("tcp", s.httpServer.Addr)
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

// ApplyMiddleware applies the standard middleware stack to the server's Gin engine:
// recovery, request-ID, CORS, body-size limit, and request logging.
func (s *Server) ApplyMiddleware() {
	s.engine.Use(middleware.Recovery())
	s.engine.Use(middleware.RequestID())
	s.engine.Use(middleware.CORS(s.config.CORS))
	if s.config.MaxBodySize != "" {
		s.engine.Use(middleware.BodySizeLimit(s.config.MaxBodySize))
	}
	s.engine.Use(middleware.RequestLogger())
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
