// Package httpclient provides a configurable HTTP adapter with built-in
// authentication, TLS, resilience (retry, circuit breaker, rate limiting),
// and streaming support.
//
// The Adapter is both a full-capability HTTP client and a provider.RequestResponse
// implementation, so it can be used standalone or composed via the provider framework
// (WithResilience, Manager, Registry, etc.).
//
// # Basic Usage
//
//	adapter, _ := httpclient.New(httpclient.Config{
//	    Name:    "my-api",
//	    BaseURL: "https://api.example.com",
//	    Timeout: 30 * time.Second,
//	    Auth:    httpclient.BearerAuth("my-token"),
//	})
//
//	resp, err := adapter.Do(ctx, httpclient.Request{
//	    Method: http.MethodGet,
//	    Path:   "/users/123",
//	})
//
// # As a Provider
//
//	// The adapter IS a provider — no wrapper needed.
//	var p provider.RequestResponse[httpclient.Request, *httpclient.Response] = adapter
//	resilient := provider.WithResilience(adapter, resilienceConfig)
//
// # REST Convenience
//
//	user, err := httpclient.Get[User](adapter, ctx, "/users/123")
//	created, err := httpclient.Post[User](adapter, ctx, "/users", body)
//
// # With Resilience
//
//	adapter, _ := httpclient.New(httpclient.Config{
//	    BaseURL: "https://api.example.com",
//	    Retry:   httpclient.DefaultRetryConfig(),
//	    CircuitBreaker: httpclient.DefaultCircuitBreakerConfig("my-api"),
//	})
package httpclient
