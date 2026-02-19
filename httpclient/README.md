# httpclient

Configurable HTTP client with built-in authentication, TLS, resilience (retry, circuit breaker, rate limiting), and streaming support. Includes a typed REST client for JSON APIs.

## Install

```bash
go get github.com/kbukum/gokit/httpclient
```

## Quick Start

### Base HTTP Client

```go
client, err := httpclient.New(httpclient.Config{
    BaseURL: "https://api.example.com",
    Auth:    httpclient.BearerAuth("my-token"),
})

resp, err := client.Do(ctx, httpclient.Request{
    Method: http.MethodGet,
    Path:   "/users/123",
})
fmt.Println(resp.StatusCode, string(resp.Body))
```

### REST Client (JSON)

```go
import "github.com/kbukum/gokit/httpclient/rest"

client, err := rest.New(httpclient.Config{
    BaseURL: "https://api.example.com",
    Auth:    httpclient.BearerAuth("token"),
})

// Typed GET
user, err := rest.Get[User](ctx, client, "/users/123")
fmt.Println(user.Data.Name)

// Typed POST
created, err := rest.Post[User](ctx, client, "/users", CreateUserRequest{
    Name: "Alice",
})
```

### With Resilience

```go
client, err := httpclient.New(httpclient.Config{
    BaseURL:        "https://api.example.com",
    Retry:          httpclient.DefaultRetryConfig(),
    CircuitBreaker: httpclient.DefaultCircuitBreakerConfig("my-api"),
})
// Retry on transient errors (5xx, timeouts, connection failures)
// Circuit breaker opens after repeated failures
```

### SSE Streaming

```go
stream, err := client.DoStream(ctx, httpclient.Request{
    Method: http.MethodGet,
    Path:   "/events",
})
defer stream.Close()

for {
    event, err := stream.SSE.Next()
    if err == io.EOF {
        break
    }
    fmt.Println(event.Event, event.Data)
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Client` | Base HTTP client with auth, TLS, resilience |
| `Config` | Client configuration (BaseURL, Timeout, Auth, TLS, Retry, CB, RateLimiter) |
| `Request` / `Response` | HTTP request and response types |
| `StreamResponse` | Streaming response with SSE reader |
| `rest.Client` | JSON-focused REST client wrapping base client |
| `rest.Get[T]` / `Post[T]` / `Put[T]` / `Patch[T]` / `Delete[T]` | Generic typed REST methods |
| `sse.Reader` | Reusable Server-Sent Events parser |
| `BearerAuth()` / `BasicAuth()` / `APIKeyAuth()` / `CustomAuth()` | Auth configuration helpers |
| `DefaultRetryConfig()` / `DefaultCircuitBreakerConfig()` | Sensible resilience defaults |
| `IsTimeout()` / `IsAuth()` / `IsNotFound()` / `IsRetryable()` | Error classification helpers |

## Authentication

```go
// Bearer token
httpclient.BearerAuth("token")

// Basic auth
httpclient.BasicAuth("user", "pass")

// API key in header
httpclient.APIKeyAuth("secret")
httpclient.APIKeyAuthHeader("secret", "X-Custom-Key")

// API key in query parameter
httpclient.APIKeyAuthQuery("secret", "api_key")

// Custom auth
httpclient.CustomAuth(func(req *http.Request) {
    req.Header.Set("X-Signature", sign(req))
})
```

## Error Handling

```go
resp, err := client.Do(ctx, req)
if err != nil {
    switch {
    case httpclient.IsAuth(err):        // 401, 403
    case httpclient.IsNotFound(err):    // 404
    case httpclient.IsRateLimit(err):   // 429
    case httpclient.IsServerError(err): // 5xx
    case httpclient.IsTimeout(err):     // timeouts
    case httpclient.IsConnection(err):  // connection failures
    case httpclient.IsRetryable(err):   // any retryable error
    }
}
```

## TLS

Uses shared `security.TLSConfig` from gokit core:

```go
client, err := httpclient.New(httpclient.Config{
    BaseURL: "https://internal-api.example.com",
    TLS: &security.TLSConfig{
        CAFile:   "/path/to/ca.pem",
        CertFile: "/path/to/client-cert.pem",
        KeyFile:  "/path/to/client-key.pem",
    },
})
```

---

[⬅ Back to main README](../README.md)
