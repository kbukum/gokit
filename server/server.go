package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server/endpoint"
	"github.com/kbukum/gokit/server/middleware"
)

// Server is a unified HTTP server backed by Gin with optional support for additional http.Handler mounts (e.g. Connect-Go / gRPC) on the same port.
type Server struct {
	httpServer     *http.Server
	engine         *gin.Engine
	mux            *http.ServeMux
	restHandler    http.Handler // Gin engine, optionally wrapped with the per-request timeout
	config         Config
	log            *logging.Logger
	mounts         []MountedHandler      // tracked for summary display
	listener       net.Listener          // set by Start(); used by ListenAddr()
	secHeaders     middleware.Middleware // built once from config; no-op when disabled
	serveWG        sync.WaitGroup
	cancelRequests context.CancelFunc
	admissionMu    sync.Mutex
	closing        bool
	active         int
	drained        chan struct{}
	metricsMW      middleware.Middleware
}

// MountedHandler records a handler mounted on the ServeMux.
type MountedHandler struct {
	Pattern string
	Label   string // optional human-readable label
}

// New creates a new Server. The Gin engine is created but no middleware is applied yet —
// call ApplyDefaults on the config first if needed.
func New(cfg *Config, log *logging.Logger) *Server {
	// Default the logger once so the server, request context, recovery, and
	// request-logging middleware all share the same instance and New(cfg, nil)
	// never panics on a nil logger.
	if log == nil {
		log = logging.NewDefault("server")
	}

	// Set Gin mode based on the injected logger's level.
	if log.Level() <= slog.LevelDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		middleware.SetMetricRoute(c.Request, c.FullPath())
		c.Next()
	})
	mux := http.NewServeMux()

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
	} else if cfg.H2CEnabled() {
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

	srv := &Server{
		httpServer: httpServer,
		engine:     engine,
		mux:        mux,
		config:     *cfg,
		log:        log,
		secHeaders: secHeaders,
		drained:    make(chan struct{}),
	}
	// Default the REST handler to the bare Gin engine; ApplyMiddleware may wrap
	// it with the per-request timeout. Mount an indirection at "/" so that later
	// wrapping is visible without re-registering the (single-use) mux pattern.
	srv.restHandler = engine
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.restHandler.ServeHTTP(w, r)
	}))
	srv.httpServer.Handler = srv.admit(mux)
	return srv
}

// GinEngine returns the underlying Gin engine for route registration.
func (s *Server) GinEngine() *gin.Engine {
	return s.engine
}

// Logger returns the server's injected logger, for callers that mount routes or
// docs directly (e.g. [MountDocs]) and need to share the server's logging.
func (s *Server) Logger() *logging.Logger {
	return s.log
}

// Handle mounts an http.Handler at the given pattern on the root ServeMux.
// Use this to add Connect-Go or any other handler alongside Gin.
// The pattern must include a trailing slash for subtree matches (e.g. "/grpc.health.v1.Health/").
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		middleware.SetMetricRoute(r, pattern)
		handler.ServeHTTP(w, r)
	}))
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
	if err := s.config.Validate(); err != nil {
		return err
	}
	if s.config.TLS != nil && s.config.TLS.IsEnabled() && s.httpServer.TLSConfig == nil {
		if _, err := s.config.TLS.Build(); err != nil {
			return err
		}
	}
	s.log.DebugCtx(ctx, "Starting HTTP server", map[string]any{
		"addr": s.httpServer.Addr,
	})

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("server failed to bind %s: %w", s.httpServer.Addr, err)
	}
	s.listener = listener
	requestCtx, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	s.cancelRequests = cancelRequests
	s.httpServer.BaseContext = func(net.Listener) context.Context { return requestCtx }

	s.serveWG.Add(1)
	go func() { //nolint:contextcheck // serve goroutine outlives the Start ctx
		defer s.serveWG.Done()
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

// Stop gracefully shuts down the server, waiting up to the configured
// shutdown_timeout for in-flight requests to drain.
func (s *Server) Stop(ctx context.Context) error {
	ctx, cancel := s.shutdownContext(ctx)
	defer cancel()
	err := s.drain(ctx)
	s.serveWG.Wait()
	return err
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
//
// The optional per-request timeout is the exception: it wraps only the Gin
// (REST) engine, not the RPC/streaming handlers mounted via Handle(), because
// http.TimeoutHandler buffers the response and cannot flush or hijack — which
// would break streaming RPCs and h2c.
func (s *Server) ApplyMiddleware() {
	// Wrap the REST engine (and only the REST engine) with the per-request
	// timeout. RPC/streaming mounts keep serving through the bare mux.
	s.restHandler = s.engine
	if s.config.RequestTimeout > 0 {
		s.restHandler = middleware.Timeout(time.Duration(s.config.RequestTimeout) * time.Second)(middleware.Recovery(s.log)(s.admit(s.engine)))
	}

	stack := []middleware.Middleware{
		middleware.InjectLogger(s.log),
		middleware.Recovery(s.log),
		middleware.RequestID(),
	}
	if s.metricsMW != nil {
		stack = append([]middleware.Middleware{s.metricsMW}, stack...)
	}
	if s.secHeaders != nil {
		stack = append(stack, s.secHeaders)
	}
	stack = append(stack,
		middleware.CORS(&s.config.CORS),
		middleware.RequestLogger(s.log),
	)
	if s.config.MaxBodyBytes > 0 {
		stack = append(stack, middleware.BodySizeLimit(s.config.MaxBodyBytes))
	}

	s.httpServer.Handler = s.admit(middleware.Chain(stack...)(s.mux))
}

// RegisterDefaultEndpoints registers the standard observability endpoints:
//   - GET /health   — full Health response with component statuses
//   - GET /healthz  — alias for /health (k8s convention)
//   - GET /livez    — liveness probe (process is up)
//   - GET /readyz   — readiness probe (component-aware)
//   - GET /info     — build/runtime info
func (s *Server) RegisterDefaultEndpoints(serviceName string, checker endpoint.HealthChecker) {
	healthHandler := endpoint.Health(serviceName, checker)
	s.engine.GET("/health", healthHandler)
	s.engine.GET("/healthz", healthHandler)
	s.engine.GET("/livez", endpoint.Liveness(serviceName))
	s.engine.GET("/readyz", endpoint.Readiness(serviceName, checker))
	s.engine.GET("/info", endpoint.Info(serviceName))
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

	MountDocs(s.engine, s.log, APIDoc{
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
