// Package connect provides Connect-Go RPC integration for gokit services.
//
// It includes JWT authentication interceptors, service mounting helpers,
// error mapping, logging interceptors, and configuration for Connect-Go
// handlers.
//
// # Server-side (Handlers)
//
// Services implement the Service interface and are mounted via Mount():
//
//	svc := connect.NewService(path, handler)
//	connect.Mount(srv, svc.Path(), svc.Handler())
//
// Mount accepts any HandlerMounter (e.g. gokit/server.Server).
//
// # Client-side
//
// The client subpackage provides h2c HTTP clients for ConnectRPC:
//
//	httpClient := client.NewHTTPClient(client.Config{BaseURL: "http://localhost:8080"})
//	svcClient := myv1connect.NewMyServiceClient(httpClient, cfg.BaseURL)
//
// # Authentication
//
// JWT authentication is provided through Connect interceptors:
//
//   - TokenAuthInterceptor: Rejects unauthenticated requests
//   - OptionalTokenAuthInterceptor: Allows unauthenticated requests but extracts claims if present
package connect
