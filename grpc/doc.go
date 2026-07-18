// Package grpc provides gRPC client configuration, connection management, and interceptors for gokit services.
//
// # Client
//
// The grpc/client sub-package provides a factory for creating gRPC client connections with configurable TLS, keepalive, and message size settings:
//
//	conn, err := client.New(ctx, target, opts...)
//
// # Interceptors
//
// The grpc/interceptor sub-package provides common unary and stream interceptors:
//
//   - Error mapping between gRPC status codes and gokit errors
//   - Request/response logging with structured fields
//   - Resilience policy enforcement for unary RPCs
//
// When composing interceptors, keep shared cross-cutting order explicit: tracing → logging → auth → validation → handler → metrics.
package grpc
