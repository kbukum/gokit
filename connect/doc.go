// Package connect provides Connect-Go RPC integration for gokit services.
//
// It includes typed authentication guards, service mounting helpers, error mapping,
// logging interceptors, and configuration for Connect-Go handlers.
//
// # Server-side (Handlers)
//
// Services implement the Service interface and are mounted via Mount():
//
//	svc := connect.NewService(path, handler)
//	if err := connect.Mount(srv, svc.Path(), svc.Handler()); err != nil {
//		return err
//	}
//
// Mount accepts any HandlerMounter (e.g. gokit/server.Server) and returns its registration error.
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
// Authenticate the complete request in an outer HTTP middleware before dispatching to Connect. Inject the same typed identity getter into service handlers and the guard:
//
//   - AuthInterceptor: Rejects requests without an already-verified identity.
//   - RequireAuth: Retrieves the verified identity using an injected ClaimsGetter.
//
// Credential parsing, cookie checks, CSRF, and session revocation belong to the injected authentication provider, not to unary interceptors.
package connect
