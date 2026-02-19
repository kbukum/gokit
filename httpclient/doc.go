// Package httpclient provides a configurable HTTP client with built-in
// authentication, TLS, resilience (retry, circuit breaker, rate limiting),
// and streaming support.
//
// The base Client handles all HTTP protocol concerns. Subpackages provide
// protocol-specific convenience layers:
//
//   - rest: JSON-focused REST client with generic typed methods
//   - sse: Server-Sent Events reader
//
// # Basic Usage
//
//	client := httpclient.New(httpclient.Config{
//	    BaseURL: "https://api.example.com",
//	    Timeout: 30 * time.Second,
//	    Auth:    httpclient.BearerAuth("my-token"),
//	})
//
//	resp, err := client.Do(ctx, httpclient.Request{
//	    Method: http.MethodGet,
//	    Path:   "/users/123",
//	})
//
// # With Resilience
//
//	client := httpclient.New(httpclient.Config{
//	    BaseURL: "https://api.example.com",
//	    Retry:   httpclient.DefaultRetryConfig(),
//	    CircuitBreaker: httpclient.DefaultCircuitBreakerConfig("my-api"),
//	})
package httpclient
