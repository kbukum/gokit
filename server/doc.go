// Package server provides a unified HTTP server for gokit applications
// using Gin with HTTP/2 and h2c support for serving both REST and gRPC traffic.
//
// The server follows gokit's component pattern with lifecycle management,
// health endpoints, and configurable middleware.
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
