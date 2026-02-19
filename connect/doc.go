// Package connect provides Connect-Go RPC integration for gokit services.
//
// It includes JWT authentication interceptors, service mounting helpers,
// and configuration for Connect-Go handlers with support for required
// and optional JWT validation, claims extraction, and context propagation.
//
// # Service Registration
//
// Services implement the Service interface and are mounted via Mount():
//
//	svc := connect.NewService(path, handler)
//	connect.Mount(mux, svc, interceptors...)
//
// # Authentication
//
// JWT authentication is provided through Connect interceptors:
//
//   - RequiredAuthInterceptor: Rejects unauthenticated requests
//   - OptionalAuthInterceptor: Allows unauthenticated requests but extracts claims if present
package connect
