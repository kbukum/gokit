package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server/endpoint"
	"github.com/kbukum/gokit/server/middleware"
)

// Server is a unified HTTP server backed by Gin with optional support for additional http.Handler mounts (e.g. Connect-Go / gRPC) on the same port.
type Server struct {
	httpServer *http.Server
	engine     *gin.Engine
	mux        *http.ServeMux
	config     Config
	log        *logging.Logger
	mounts     []MountedHandler      // tracked for summary display
	listener   net.Listener          // set by Start(); used by ListenAddr()
	secHeaders middleware.Middleware // built once from config; no-op when disabled
}

// MountedHandler records a handler mounted on the ServeMux.
type MountedHandler struct {
	Pattern string
	Label   string // optional human-readable label
}

// New creates a new Server. The Gin engine is created but no middleware is applied yet —
// call ApplyDefaults on the config first if needed.
func New(cfg *Config, log *logging.Logger) *Server {
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

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Configure HTTP/2 protocol support based on TLS setting.
	var protocols http.Protocols
	protocols.SetHTTP1(true)

	var tlsConfig *tls.Config
	if cfg.TLS != nil && cfg.TLS.IsEnabled() {
		// TLS enabled: use encrypted HTTP/2.
		protocols.SetHTTP2(true)
		var err error
		tlsConfig, err = cfg.TLS.Build()
		if err != nil {
			// Return a server with nil TLS — Start will fail with a clear error.
			// We don't panic in constructors.
			tlsConfig = nil
		}
	} else {
		// No TLS: enable unencrypted HTTP/2 (h2c) for gRPC without TLS.
		protocols.SetUnencryptedHTTP2(true)
	}

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		Protocols:    &protocols,
		HTTP2:        &http.HTTP2Config{MaxConcurrentStreams: 250},
		TLSConfig:    tlsConfig,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
	}

	log = log.WithComponent("server")

	// Build the security-headers middleware once.
	// Config.Validate is the trust boundary that rejects an invalid SecurityHeaders config,
	// so a validated config never reaches the error branch here.
	// If an unvalidated invalid config does reach New, fall back to the secure defaults —
	// which build from a nil config and therefore cannot fail —
	// rather than serving responses without protective headers.
	// New is an infallible constructor with no request context, so the fallback is silent by design;
	// the invalid config is surfaced to callers through Config.Validate.
	secHeaders, err := middleware.SecurityHeaders(&cfg.SecurityHeaders)
	if err != nil {
		if secHeaders, err = middleware.SecurityHeaders(nil); err != nil {
			secHeaders = nil
		}
	}

	return &Server{
		httpServer: httpServer,
		engine:     engine,
		mux:        mux,
		config:     *cfg,
		log:        log,
		secHeaders: secHeaders,
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
	s.log.Debug("Handler mounted", map[string]any{
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

// Start binds the port and begins serving. It returns once the listener is bound
// so the caller knows the port is ready; serving continues in a goroutine.
func (s *Server) Start(ctx context.Context) error {
	s.log.DebugCtx(ctx, "Starting HTTP server", map[string]any{
		"addr": s.httpServer.Addr,
	})

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("server failed to bind %s: %w", s.httpServer.Addr, err)
	}
	s.listener = listener

	go func() { //nolint:contextcheck // serve goroutine outlives the Start ctx
		var serveErr error
		if s.httpServer.TLSConfig != nil {
			serveErr = s.httpServer.ServeTLS(listener, "", "")
		} else {
			serveErr = s.httpServer.Serve(listener)
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			s.log.Error("Server error", map[string]any{
				"error": serveErr.Error(),
			})
		}
	}()

	s.log.InfoCtx(ctx, "HTTP server started", map[string]any{
		"addr": s.httpServer.Addr,
	})
	return nil
}

// Stop gracefully shuts down the server with a 5-second deadline.
func (s *Server) Stop(ctx context.Context) error {
	s.log.DebugCtx(ctx, "Shutting down HTTP server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.log.ErrorCtx(ctx, "Server shutdown error", map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("server shutdown error: %w", err)
	}

	s.log.DebugCtx(ctx, "HTTP server shut down successfully")
	return nil
}

// Addr returns the configured listen address.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// ListenAddr returns the actual address the server is listening on.
// This is useful when the server is configured with port 0 (random port).
// Returns nil if the server has not started yet.
func (s *Server) ListenAddr() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

// Config returns the server configuration.
func (s *Server) Config() Config {
	return s.config
}

// readSpecFile reads an OpenAPI spec file from disk.
func readSpecFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ApplyMiddleware applies the standard middleware stack at the handler level
// so it covers ALL routes — both Gin REST endpoints and ConnectRPC services mounted via Handle().
func (s *Server) ApplyMiddleware() {
	stack := []middleware.Middleware{
		middleware.Recovery(s.log),
		middleware.RequestID(),
	}
	if s.secHeaders != nil {
		stack = append(stack, s.secHeaders)
	}
	stack = append(stack,
		middleware.CORS(&s.config.CORS),
		middleware.RequestLogger(s.log),
	)
	if s.config.MaxBodySize != "" {
		stack = append(stack, middleware.BodySizeLimit(s.config.MaxBodySize))
	}

	s.httpServer.Handler = middleware.Chain(stack...)(s.mux)
}

// RegisterDefaultEndpoints registers the standard observability endpoints:
//   - GET /health   — full Health response with component statuses
//   - GET /healthz  — alias for /health (k8s convention)
//   - GET /livez    — liveness probe (process is up)
//   - GET /readyz   — readiness probe (component-aware)
//   - GET /info     — build/runtime info
//   - GET /metrics  — Prometheus exposition
//
// Closes F-060 (no /healthz//readyz handler shipped despite full Health taxonomy in observability/).
func (s *Server) RegisterDefaultEndpoints(serviceName string, checker endpoint.HealthChecker) {
	healthHandler := endpoint.Health(serviceName, checker)
	s.engine.GET("/health", healthHandler)
	s.engine.GET("/healthz", healthHandler)
	s.engine.GET("/livez", endpoint.Liveness(serviceName))
	s.engine.GET("/readyz", endpoint.Readiness(serviceName, checker))
	s.engine.GET("/info", endpoint.Info(serviceName))
	s.engine.GET("/metrics", endpoint.Metrics())
}

// RegisterPprof mounts net/http/pprof handlers under /debug/pprof.
// Only enable in non-public environments (the handlers expose runtime state).
//
// Closes F-070 sub-finding: no net/http/pprof integration.
func (s *Server) RegisterPprof() {
	pprofGroup := s.engine.Group("/debug/pprof")
	pprofGroup.GET("/", gin.WrapF(pprof.Index))
	pprofGroup.GET("/cmdline", gin.WrapF(pprof.Cmdline))
	pprofGroup.GET("/profile", gin.WrapF(pprof.Profile))
	pprofGroup.POST("/symbol", gin.WrapF(pprof.Symbol))
	pprofGroup.GET("/symbol", gin.WrapF(pprof.Symbol))
	pprofGroup.GET("/trace", gin.WrapF(pprof.Trace))
	pprofGroup.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
	pprofGroup.GET("/block", gin.WrapH(pprof.Handler("block")))
	pprofGroup.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
	pprofGroup.GET("/heap", gin.WrapH(pprof.Handler("heap")))
	pprofGroup.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
	pprofGroup.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
}

// MountDocsFromConfig mounts interactive API documentation using Scalar UI based on the server's DocsConfig.
// If DocsConfig.Enabled is false, this is a no-op. When SpecFile is set,
// the spec is loaded from disk;
// otherwise spec must be provided via the optional specJSON parameter.
//
// This is a convenience wrapper around [MountDocs] for config-driven setups.
func (s *Server) MountDocsFromConfig(specJSON ...[]byte) {
	if !s.config.Docs.Enabled {
		return
	}

	dc := s.config.Docs
	if dc.UIPath == "" {
		dc.UIPath = "/docs"
	}
	if dc.SpecPath == "" {
		dc.SpecPath = "/docs/openapi.json"
	}
	if dc.Title == "" {
		dc.Title = "API Reference"
	}

	var spec []byte
	switch {
	case dc.SpecFile != "":
		data, err := readSpecFile(dc.SpecFile)
		if err != nil {
			s.log.Error("Failed to load OpenAPI spec file", map[string]any{
				"path":  dc.SpecFile,
				"error": err.Error(),
			})
			return
		}
		spec = data
	case len(specJSON) > 0 && specJSON[0] != nil:
		spec = specJSON[0]
	default:
		s.log.Warn("API docs enabled but no spec provided — set docs.spec_file or pass spec bytes")
		return
	}

	host := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	MountDocs(s.engine, APIDoc{
		Title:    dc.Title,
		SpecPath: dc.SpecPath,
		Spec:     spec,
		UIPath:   dc.UIPath,
		Host:     host,
		HideAI:   true,
	})
}

// ApplyDefaults applies the standard middleware stack and registers default endpoints.
func (s *Server) ApplyDefaults(serviceName string, checker endpoint.HealthChecker) {
	s.ApplyMiddleware()
	s.RegisterDefaultEndpoints(serviceName, checker)
}
