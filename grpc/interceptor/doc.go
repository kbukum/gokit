// Package interceptor provides reusable gRPC unary and stream interceptors
// for logging, tracing, metrics, authentication, and resilience patterns.
// Recommended shared order is tracing → logging → auth → validation →
// handler → metrics.
package interceptor
