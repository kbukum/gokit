# GoKit Adapter Guide

This document explains how gokit modules work as adapters and how to build
domain-specific adapters on top of them.

## Core Principle

Every gokit infrastructure module is both a native client AND a `provider.Provider`.
No separate wrapper is needed — the adapter IS the provider.

```go
// The HTTP adapter is simultaneously:
// 1. A full-capability HTTP client
adapter.Do(ctx, httpclient.Request{Method: "GET", Path: "/users"})

// 2. A provider.RequestResponse for composition
var p provider.RequestResponse[httpclient.Request, *httpclient.Response] = adapter

// 3. Composable with resilience, selection, etc.
resilient := provider.WithResilience(adapter, resilienceConfig)
```

## Adapter Consistency

All gokit adapters follow the same structural pattern:

```
Config → New() → Adapter → provider.Provider interface → Component (optional)
```

| Module | Adapter Type | Provider Interface | Native Methods |
|--------|-------------|-------------------|----------------|
| `httpclient` | `*Adapter` | `RequestResponse[Request, *Response]` | `Do()`, `DoStream()` |
| `grpc/client` | `*Adapter` | `Provider` + `Closeable` | `Conn()`, `ClientOf[T]()` |
| `kafka/producer` | `*Producer` | `Sink[kafka.Message]` | `Publish()`, `WriteMessages()` |
| `kafka/consumer` | `*Consumer` | `Provider` | `Consume()` |
| `process` | `*Adapter` | `RequestResponse[Command, *Result]` | `Run()` |
| `redis` | `*Client` | `Provider` | `Get()`, `Set()`, etc. |
| `database` | `*DB` | `Provider` | `GormDB`, `Transaction()` |
| `storage` | `*Component` | `Provider` | `Upload()`, `Download()` |

## Using Adapters

### Simple Client Usage

```go
adapter, err := httpclient.New(httpclient.Config{
    Name:    "my-api",
    BaseURL: "https://api.example.com",
    Auth:    httpclient.BearerAuth("token"),
})

resp, err := adapter.Do(ctx, httpclient.Request{
    Method: http.MethodGet,
    Path:   "/users/123",
})
```

### REST Typed Convenience

```go
user, err := httpclient.Get[User](adapter, ctx, "/users/123")
created, err := httpclient.Post[User](adapter, ctx, "/users", newUser)
```

### As a Provider (for composition)

```go
// Use with provider framework for resilience
resilient := provider.WithResilience(adapter, provider.ResilienceConfig{
    CircuitBreaker: cbConfig,
    Retry:          retryConfig,
})

// Use with manager for lifecycle + selection
mgr := provider.NewManager(registry, selector)
```

### As a Component (for lifecycle)

```go
comp := httpclient.NewComponent(httpclient.Config{
    Name:    "payment-api",
    BaseURL: "https://payments.example.com",
})
// Register with bootstrap
bootstrap.Register(comp)
// After Start(), get the adapter:
adapter := comp.Adapter()
```

### gRPC Adapter

```go
adapter, err := client.NewAdapter(grpccfg.Config{
    Name: "user-service",
    Host: "localhost",
    Port: 50051,
}, log)

// Create typed clients from the managed connection
userClient := client.ClientOf(adapter, pb.NewUserServiceClient)
resp, err := userClient.GetUser(ctx, &pb.GetUserRequest{Id: "123"})
```

### Kafka Producer as Sink

```go
producer, err := producer.NewProducer(kafka.Config{
    Name:    "events",
    Brokers: []string{"localhost:9092"},
    Enabled: true,
}, log)

// Native usage
producer.Publish(ctx, "events", event)

// As provider.Sink[kafka.Message]
var sink provider.Sink[kafka.Message] = producer
sink.Send(ctx, msg)
```

## Building Domain Adapters (Derivation)

Domain-specific adapters derive from native adapters through composition.
GoKit provides the transport; your project provides the domain logic.

### Example: LLM Adapter

```go
package llm

import "github.com/kbukum/gokit/httpclient"

type Config struct {
    httpclient.Config `yaml:",inline"`
    Model             string  `yaml:"model"`
    Temperature       float64 `yaml:"temperature"`
    MaxTokens         int     `yaml:"max_tokens,omitempty"`
}

type Adapter struct {
    http   *httpclient.Adapter
    config Config
}

func New(cfg Config) (*Adapter, error) {
    httpAdapter, err := httpclient.New(cfg.Config)
    if err != nil {
        return nil, err
    }
    return &Adapter{http: httpAdapter, config: cfg}, nil
}

func (a *Adapter) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    httpReq := &httpclient.Request{
        Method: http.MethodPost,
        Path:   "/v1/chat/completions",
        Body:   a.buildBody(req),
    }
    resp, err := a.http.Do(ctx, httpReq)
    if err != nil {
        return nil, err
    }
    return a.parseResponse(resp)
}

// provider.Provider
func (a *Adapter) Name() string                       { return a.config.Name }
func (a *Adapter) IsAvailable(ctx context.Context) bool { return a.http.IsAvailable(ctx) }
```

### Config-Driven Usage

```go
// Config comes from anywhere: YAML, database, environment...
cfg := llm.Config{
    Config: httpclient.Config{
        Name:    "openai",
        BaseURL: "https://api.openai.com",
        Auth:    httpclient.BearerAuth(os.Getenv("OPENAI_KEY")),
    },
    Model:       "gpt-4",
    Temperature: 0.7,
}

adapter, err := llm.New(cfg)
```

## Provider Health

Adapters can optionally implement `provider.HealthChecker` for detailed health:

```go
type HealthChecker interface {
    Health(ctx context.Context) HealthStatus
}

type HealthStatus struct {
    Status  Status // StatusHealthy, StatusDegraded, StatusUnavailable
    Message string
    Details map[string]any
}
```

## Design Principles

1. **Adapter IS the Provider** — No wrapper layer. Direct interface implementation.
2. **Full Capability Always** — Native methods are always available alongside provider interface.
3. **Config-Driven, Not Config-Mandated** — Config struct from any source (YAML, DB, env, hardcoded).
4. **Derivation Through Composition** — Domain adapters wrap native adapters.
5. **Consistent Structure** — All follow Config → Adapter → Provider → Component convention.
6. **GoKit = Transports, Projects = Domains** — GoKit provides HTTP/gRPC/Kafka/etc. Projects build LLM/Payment/etc.
