// Package server provides a unified HTTP server for gokit applications using Gin with HTTP/2 and h2c support for serving both REST and gRPC traffic.
//
// The server follows gokit's component pattern with lifecycle management, health endpoints, and configurable middleware.
//
// # API Documentation
//
// The server can serve interactive API documentation powered by Scalar UI. Enable via config and provide an OpenAPI spec:
//
//	cfg := &server.Config{
//	    Port: 8080,
//	    Docs: server.DocsConfig{
//	        Enabled:  true,
//	        SpecFile: "./api/openapi.json",
//	    },
//	}
//	srv := server.New(cfg, log)
//	srv.MountDocsFromConfig()
//
// Or mount docs directly with [MountDocs] for full control:
//
//	server.MountDocs(engine, server.APIDoc{
//	    Title:    "My API",
//	    SpecPath: "/docs/openapi.json",
//	    Spec:     specBytes,
//	    UIPath:   "/docs",
//	})
//
// # Middleware
//
// Built-in middleware (server/middleware):
//
//   - Recovery: Panic recovery with structured logging
//   - Logging: Request/response logging with duration tracking
//   - CORS: Cross-origin resource sharing configuration
//   - RequestID: Request ID generation and propagation
//   - RateLimit: Token bucket rate limiting
//   - BodySize: Request body size limits
//   - Auth: JWT authentication middleware
//
// Shared middleware order for transport concerns is: tracing → logging → auth → validation → handler → metrics. Apply recovery around that chain.
//
// # Endpoints
//
// Built-in endpoints (server/endpoint):
//
//   - /health: Health check aggregation
//   - /info: Application information
//   - /metrics: Prometheus metrics
//   - /liveness: Kubernetes liveness probe
//   - /readiness: Kubernetes readiness probe
//   - /version: Build version information
package server
