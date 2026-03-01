# GoKit Adapter Architecture — Enhancement Plan

> **Status**: Implemented (Phases 1-3 complete)  
> **Scope**: Restructuring gokit modules into proper, consistent adapters  
> **Go Version**: 1.25.0  

---

## Table of Contents

1. [What This Is About](#1-what-this-is-about)
2. [Current State Assessment](#2-current-state-assessment)
3. [The Adapter Pattern GoKit Already Has](#3-the-adapter-pattern-gokit-already-has)
4. [What Needs to Change](#4-what-needs-to-change)
5. [The Adapter Contract](#5-the-adapter-contract)
6. [Module-by-Module Enhancement](#6-module-by-module-enhancement)
7. [HTTP Adapter (Rewrite of httpclient)](#7-http-adapter)
8. [gRPC Adapter (Enhancement of grpc)](#8-grpc-adapter)
9. [Kafka Adapter (Enhancement of kafka)](#9-kafka-adapter)
10. [Process Adapter (Enhancement of process)](#10-process-adapter)
11. [Redis Adapter (Enhancement of redis)](#11-redis-adapter)
12. [Database Adapter (Enhancement of database)](#12-database-adapter)
13. [Storage Adapter (Already Close)](#13-storage-adapter)
14. [Derivation: How LLM Would Be Built](#14-derivation-how-llm-would-be-built)
15. [Implementation Phases](#15-implementation-phases)
16. [Design Principles](#16-design-principles)

---

## 1. What This Is About

This is NOT about adding a layer on top of gokit. This is about **making gokit's own modules proper, consistent adapters** that can be used simply (like today) or dynamically (from config, at runtime).

Today `httpclient` is a client library. Tomorrow it should be an **HTTP adapter** — still usable as a simple client, but also capable of being created from config, composed with resilience, and derived into domain-specific adapters (REST, LLM, etc.).

The same applies to `grpc`, `kafka`, `redis`, `database`, `process`.

**GoKit's job**: provide proper native adapters with consistent structure.  
**Project's job**: use those adapters however they want — hardcoded, config-driven, dynamic, whatever.

---

## 2. Current State Assessment

### What GoKit Already Has (the 85%)

The `provider` package already defines what adapters look like:

```go
// Base identity + availability
type Provider interface {
    Name() string
    IsAvailable(ctx context.Context) bool
}

// Four interaction patterns
type RequestResponse[I, O any] interface { Provider; Execute(ctx, I) (O, error) }
type Stream[I, O any]          interface { Provider; Execute(ctx, I) (Iterator[O], error) }
type Sink[I any]               interface { Provider; Send(ctx, I) error }
type Duplex[I, O any]          interface { Provider; Open(ctx) (DuplexStream[I,O], error) }

// Composition
Adapt[I,O,BI,BO]()    // type bridging
Chain()                // middleware
WithResilience()       // circuit breaker, retry, rate limit
Stateful[I,O,C]        // state management

// Lifecycle
type Initializable interface { Init(ctx) error }
type Closeable interface { Close(ctx) error }

// Management
Factory[T]             // config → instance
Registry[T]            // factory storage + instance caching
Manager[T]             // full lifecycle + selection
```

This is already an adapter framework. The problem is that **the concrete modules don't use it consistently**.

### Module Inconsistency Map

| Module | Has Config ✅ | Implements Provider | Lifecycle | Component |
|--------|:---:|:---:|:---:|:---:|
| `httpclient` | ✅ | ⚠️ Optional wrapper | Manual (no Close) | ❌ |
| `grpc` | ✅ | ❌ Returns raw conn | Manual conn.Close() | ❌ |
| `kafka` | ✅ | ⚠️ Producer only (SinkProvider) | ✅ Component | ✅ |
| `redis` | ✅ | ❌ Direct methods | ✅ Component | ✅ |
| `database` | ✅ | ❌ GORM passthrough | ✅ Component | ✅ |
| `process` | ❌ No config | ❌ Optional SubprocessProvider | Functional (stateless) | ❌ |
| `storage` | ✅ | ✅ Per-operation providers | ✅ Component | ✅ |
| `sse` | ❌ No config | ❌ Hub pattern | ✅ Component | ✅ |

### What's Consistent

- Config struct with `ApplyDefaults()` + `Validate()` (most modules)
- Error classification patterns (most modules)
- Logger integration (all modules)

### What's Inconsistent

- **Provider integration**: Optional in some, missing in others, per-operation in storage
- **Lifecycle**: Some Component, some manual, some functional
- **Request/Response types**: Each module invents its own
- **Initialization**: Some eager, some lazy, some functional

---

## 3. The Adapter Pattern GoKit Already Has

The provider package is the adapter framework. We just need to:

1. Make every module **implement** it properly
2. Add the missing **~15%** to the Provider interface
3. Ensure **consistent structure** across all modules

This is an evolution, not a revolution. Most of the code exists.

---

## 4. What Needs to Change

### 4.1 Provider Interface Enhancement

The base `Provider` interface needs minor additions for proper adapter identity:

```go
// provider/provider.go — enhanced

type Provider interface {
    // Identity
    Name() string

    // Health
    IsAvailable(ctx context.Context) bool
}

// Optional: richer health (modules can implement if they want)
type HealthChecker interface {
    Health(ctx context.Context) HealthStatus
}

type HealthStatus struct {
    Status  Status // Healthy, Degraded, Unavailable
    Message string
    Details map[string]any
}
```

### 4.2 Every Module Becomes a Proper Adapter

Each module should follow this structure:

```
module/
├── config.go       ← Config with ApplyDefaults() + Validate()
├── adapter.go      ← Core adapter (wraps native driver)
├── provider.go     ← Implements provider.RequestResponse / Stream / Sink / Duplex
├── component.go    ← Optional: implements component.Component for lifecycle
├── errors.go       ← Error classification
└── doc.go          ← Package documentation
```

The adapter is the module's core. It can be:
- Used directly (simple client usage)
- Used as a `provider.RequestResponse` (for composition)
- Managed by `provider.Manager` (for lifecycle)
- Wrapped with `provider.WithResilience` (for resilience)
- Registered in `provider.Registry` (for dynamic selection)
- Derived into domain adapters (LLM, etc.)

---

## 5. The Adapter Contract

Every gokit adapter follows this shape. Not forced by a shared interface (each adapter has its own types), but by **convention and structure**.

```go
// ================================
// 1. CONFIG — same pattern everywhere
// ================================
type Config struct {
    // Connection info
    // Behavior settings
    // Resilience settings
}
func (c *Config) ApplyDefaults() { ... }
func (c *Config) Validate() error { ... }

// ================================
// 2. ADAPTER — the core connector
// ================================
type Adapter struct {
    config Config
    driver <native-driver>  // *http.Client, *grpc.ClientConn, *kafka.Writer, etc.
    log    *logger.Logger
}

// New creates the adapter. This is the "simple client" use case.
func New(cfg Config, opts ...Option) (*Adapter, error) {
    cfg.ApplyDefaults()
    if err := cfg.Validate(); err != nil { return nil, err }
    // create native driver from config
    return &Adapter{...}, nil
}

// Core operations — adapter-specific, full native capability
func (a *Adapter) <native-operations>() { ... }

// Close releases resources
func (a *Adapter) Close(ctx context.Context) error { ... }

// provider.Provider interface
func (a *Adapter) Name() string { return a.config.Name }
func (a *Adapter) IsAvailable(ctx context.Context) bool { ... }

// ================================
// 3. PROVIDER IMPLEMENTATION
//    The adapter itself IS a provider
//    (RequestResponse, Stream, Sink, or Duplex)
// ================================

// For request-response adapters:
var _ provider.RequestResponse[Request, *Response] = (*Adapter)(nil)
func (a *Adapter) Execute(ctx context.Context, req Request) (*Response, error) { ... }

// For sink adapters:
var _ provider.Sink[Message] = (*Adapter)(nil)
func (a *Adapter) Send(ctx context.Context, msg Message) error { ... }

// ================================
// 4. COMPONENT — optional lifecycle integration
// ================================
type Component struct {
    adapter *Adapter
    config  Config
    log     *logger.Logger
}
var _ component.Component = (*Component)(nil)
func (c *Component) Start(ctx context.Context) error { ... }
func (c *Component) Stop(ctx context.Context) error { ... }
func (c *Component) Health(ctx context.Context) component.HealthStatus { ... }
```

**Key point**: The adapter itself implements the provider interface. No separate "provider wrapper" needed. The adapter IS the provider.

---

## 6. Module-by-Module Enhancement

### Summary of Changes

| Module | Change Level | What Happens |
|--------|:---:|-------------|
| `httpclient` → `http` | **Rewrite** | Becomes HTTP adapter with endpoints, native provider impl |
| `grpc` | **Enhance** | Adapter wraps connection, adds provider impl, cleaner lifecycle |
| `kafka` | **Enhance** | Producer adapter IS a Sink, Consumer adapter IS a Stream |
| `process` | **Enhance** | Adapter wraps Run(), implements RequestResponse |
| `redis` | **Enhance** | Adapter implements RequestResponse for typed ops, keeps direct methods |
| `database` | **Minor** | Already well-structured, add provider interface |
| `storage` | **Minor** | Already closest to the pattern |

---

## 7. HTTP Adapter

The biggest change. `httpclient` becomes a proper HTTP adapter.

### Why Rewrite?

The current `httpclient` is a good HTTP client but:
- Provider wrapping is optional/external (`ClientProvider`)
- No endpoint abstraction — just raw path strings
- Resilience lives in the client AND can be layered via provider — confusing
- No way to define named operations
- `Client` doesn't implement `Provider` directly

### The New HTTP Adapter

```go
package http

// ============================================================
// CONFIG
// ============================================================

type Config struct {
    Name    string            `yaml:"name" mapstructure:"name"`
    BaseURL string            `yaml:"base_url" mapstructure:"base_url"`
    Auth    AuthConfig        `yaml:"auth" mapstructure:"auth"`
    Headers map[string]string `yaml:"headers,omitempty" mapstructure:"headers"`
    Timeout time.Duration     `yaml:"timeout" mapstructure:"timeout"`
    TLS     *TLSConfig        `yaml:"tls,omitempty" mapstructure:"tls"`

    // Resilience — applied at the adapter level
    Retry          *RetryConfig          `yaml:"retry,omitempty" mapstructure:"retry"`
    CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker,omitempty" mapstructure:"circuit_breaker"`
    RateLimiter    *RateLimiterConfig    `yaml:"rate_limiter,omitempty" mapstructure:"rate_limiter"`
}

type AuthConfig struct {
    Type     string `yaml:"type"`     // "bearer", "basic", "api_key", "none"
    Token    string `yaml:"token"`
    Username string `yaml:"username"`
    Password string `yaml:"password"`
    Header   string `yaml:"header"`   // for api_key
}

func (c *Config) ApplyDefaults() { ... }
func (c *Config) Validate() error { ... }


// ============================================================
// ADAPTER
// ============================================================

// Adapter is an HTTP adapter. It can be used:
// - As a simple HTTP client: adapter.Do(ctx, request)
// - As a provider.RequestResponse: adapter.Execute(ctx, request)
// - As a base for derivations: llm.New(httpAdapter)
type Adapter struct {
    config  Config
    client  *http.Client // stdlib http.Client (configured from Config)
    log     *logger.Logger
}

func New(cfg Config, opts ...Option) (*Adapter, error)

// --- Direct HTTP operations (full capability) ---

// Do executes an HTTP request. Full control.
func (a *Adapter) Do(ctx context.Context, req *Request) (*Response, error)

// DoStream executes an HTTP request and returns a streaming response.
func (a *Adapter) DoStream(ctx context.Context, req *Request) (*StreamResponse, error)

// --- Provider interface ---

func (a *Adapter) Name() string
func (a *Adapter) IsAvailable(ctx context.Context) bool

// Execute implements provider.RequestResponse[*Request, *Response].
// Same as Do() but satisfies the provider interface for composition.
func (a *Adapter) Execute(ctx context.Context, req *Request) (*Response, error) {
    return a.Do(ctx, req)
}

// --- Lifecycle ---

func (a *Adapter) Close(ctx context.Context) error

// --- Access to underlying config (for derivations) ---

func (a *Adapter) Config() Config


// ============================================================
// REQUEST / RESPONSE — enhanced from current
// ============================================================

type Request struct {
    Method  string
    Path    string              // appended to Config.BaseURL
    Headers map[string]string
    Query   map[string]string
    Body    any                 // io.Reader, []byte, string, or JSON-encodable struct
    Auth    *AuthConfig         // overrides adapter-level auth
}

type Response struct {
    StatusCode int
    Headers    map[string]string
    Body       []byte
}

func (r *Response) IsSuccess() bool  // 2xx
func (r *Response) IsError() bool    // 4xx/5xx
func (r *Response) JSON(v any) error // unmarshal body
func (r *Response) String() string   // body as string

type StreamResponse struct {
    StatusCode int
    Headers    map[string]string
    // For SSE
    SSE        SSEReader         // if content-type is text/event-stream
    // For raw streaming
    Body       io.ReadCloser     // raw body stream
}
func (s *StreamResponse) Close() error


// ============================================================
// REST CONVENIENCE — built into the adapter, not a separate package
// ============================================================

// JSON convenience methods — same adapter, typed responses
func Get[T any](a *Adapter, ctx context.Context, path string, opts ...RequestOption) (*T, error)
func Post[T any](a *Adapter, ctx context.Context, path string, body any, opts ...RequestOption) (*T, error)
func Put[T any](a *Adapter, ctx context.Context, path string, body any, opts ...RequestOption) (*T, error)
func Delete[T any](a *Adapter, ctx context.Context, path string, opts ...RequestOption) (*T, error)

// RequestOption for REST convenience
type RequestOption func(*Request)
func WithHeader(k, v string) RequestOption
func WithQuery(k, v string) RequestOption
func WithAuth(auth AuthConfig) RequestOption


// ============================================================
// COMPONENT — optional lifecycle integration
// ============================================================

type Component struct { ... }
var _ component.Component = (*Component)(nil)
func NewComponent(cfg Config, log *logger.Logger) *Component
func (c *Component) Start(ctx context.Context) error
func (c *Component) Stop(ctx context.Context) error
func (c *Component) Health(ctx context.Context) component.HealthStatus
func (c *Component) Adapter() *Adapter
```

### What Changed from Current httpclient

| Aspect | Before (httpclient) | After (http adapter) |
|--------|-------|-------|
| Provider | Optional `ClientProvider` wrapper | Adapter itself IS a RequestResponse provider |
| REST | Separate `rest` sub-package with its own client | Generic functions on the same adapter |
| SSE | Separate `sse` sub-package | StreamResponse with SSE reader built in |
| Resilience | Built into Client internally | Configurable, and also composable via `provider.WithResilience` |
| Naming | `httpclient.Client` | `http.Adapter` |
| Close | None | Explicit `Close()` |

### What Stayed the Same

- Config with `ApplyDefaults()` + `Validate()`
- Auth types (Bearer, Basic, APIKey, Custom)
- Error classification
- Request/Response types (enhanced but compatible)
- TLS support

---

## 8. gRPC Adapter

```go
package grpc

type Config struct {
    Name           string        `yaml:"name"`
    Address        string        `yaml:"address"` // host:port
    TLS            *TLSConfig    `yaml:"tls,omitempty"`
    Timeout        time.Duration `yaml:"timeout"`
    Keepalive      *KeepaliveConfig `yaml:"keepalive,omitempty"`
    MaxRecvMsgSize int           `yaml:"max_recv_msg_size,omitempty"`
    MaxSendMsgSize int           `yaml:"max_send_msg_size,omitempty"`
}

// Adapter manages a gRPC connection.
// Unlike HTTP, gRPC adapters don't do request mapping — proto handles types.
// The adapter manages connection lifecycle and provides the conn for proto stubs.
type Adapter struct {
    config Config
    conn   *grpc.ClientConn
    log    *logger.Logger
}

func New(cfg Config, opts ...Option) (*Adapter, error)

// Conn returns the managed gRPC connection.
// Use with generated proto client stubs.
func (a *Adapter) Conn() *grpc.ClientConn

// ClientOf creates a typed gRPC client from the adapter's connection.
//   userClient := grpc.ClientOf(adapter, pb.NewUserServiceClient)
func ClientOf[T any](a *Adapter, newClient func(grpc.ClientConnInterface) T) T

// provider.Provider
func (a *Adapter) Name() string
func (a *Adapter) IsAvailable(ctx context.Context) bool
func (a *Adapter) Close(ctx context.Context) error
```

**Key**: gRPC adapter manages connections, not messages. Proto handles type safety. The adapter's value is config-driven connection setup + lifecycle.

---

## 9. Kafka Adapter

```go
package kafka

type Config struct {
    Name        string     `yaml:"name"`
    Brokers     []string   `yaml:"brokers"`
    GroupID     string     `yaml:"group_id,omitempty"`
    TLS         *TLSConfig `yaml:"tls,omitempty"`
    SASL        *SASLConfig `yaml:"sasl,omitempty"`
    Compression string     `yaml:"compression,omitempty"`
}

// ============================================================
// PRODUCER ADAPTER — implements provider.Sink[Message]
// ============================================================

type ProducerAdapter struct {
    config  Config
    writer  *kafka.Writer
    log     *logger.Logger
}

func NewProducer(cfg Config, opts ...Option) (*ProducerAdapter, error)

// Publish sends a message to a topic.
func (p *ProducerAdapter) Publish(ctx context.Context, msg Message) error

// PublishEvent sends a structured event with metadata headers.
func (p *ProducerAdapter) PublishEvent(ctx context.Context, event Event, key string) error

// provider.Sink[Message]
func (p *ProducerAdapter) Send(ctx context.Context, msg Message) error { return p.Publish(ctx, msg) }
func (p *ProducerAdapter) Name() string
func (p *ProducerAdapter) IsAvailable(ctx context.Context) bool
func (p *ProducerAdapter) Close(ctx context.Context) error

// ============================================================
// CONSUMER ADAPTER — implements provider.Stream[ConsumerConfig, Message]
// ============================================================

type ConsumerAdapter struct {
    config   Config
    reader   *kafka.Reader
    log      *logger.Logger
}

func NewConsumer(cfg Config, opts ...Option) (*ConsumerAdapter, error)

// Consume blocks, reading messages and calling handler.
func (c *ConsumerAdapter) Consume(ctx context.Context, handler MessageHandler) error

// provider.Stream — returns an iterator over messages
func (c *ConsumerAdapter) Execute(ctx context.Context, cfg ConsumerConfig) (provider.Iterator[Message], error)
func (c *ConsumerAdapter) Name() string
func (c *ConsumerAdapter) IsAvailable(ctx context.Context) bool
func (c *ConsumerAdapter) Close(ctx context.Context) error
```

---

## 10. Process Adapter

```go
package process

type Config struct {
    Name        string        `yaml:"name,omitempty"`
    GracePeriod time.Duration `yaml:"grace_period,omitempty"`
    Timeout     time.Duration `yaml:"timeout,omitempty"`
}

// Adapter executes subprocesses.
// Implements provider.RequestResponse[Command, *Result].
type Adapter struct {
    config Config
    log    *logger.Logger
}

func New(cfg Config) *Adapter

// Run executes a command.
func (a *Adapter) Run(ctx context.Context, cmd Command) (*Result, error)

// provider.RequestResponse[Command, *Result]
func (a *Adapter) Execute(ctx context.Context, cmd Command) (*Result, error) { return a.Run(ctx, cmd) }
func (a *Adapter) Name() string
func (a *Adapter) IsAvailable(ctx context.Context) bool

// Command and Result stay the same as today
type Command struct { Binary, Args, Dir, Env, Stdin, GracePeriod }
type Result struct { Stdout, Stderr, ExitCode, Duration }
```

---

## 11. Redis Adapter

```go
package redis

type Config struct {
    Name         string `yaml:"name,omitempty"`
    Addr         string `yaml:"addr"`
    Password     string `yaml:"password,omitempty"`
    DB           int    `yaml:"db,omitempty"`
    PoolSize     int    `yaml:"pool_size,omitempty"`
    // ... timeouts etc
}

// Adapter wraps Redis with proper adapter contract.
// Direct methods (Get/Set/etc.) are still available.
// Also implements provider for typed store operations.
type Adapter struct {
    config Config
    rdb    *goredis.Client
    log    *logger.Logger
}

func New(cfg Config, log *logger.Logger) (*Adapter, error)

// --- Direct Redis operations (full capability, same as today) ---
func (a *Adapter) Get(ctx context.Context, key string) (string, error)
func (a *Adapter) Set(ctx context.Context, key string, value any, ttl time.Duration) error
func (a *Adapter) Del(ctx context.Context, keys ...string) error
func (a *Adapter) GetJSON(ctx context.Context, key string, dest any) error
func (a *Adapter) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
// ... etc

// --- provider.Provider ---
func (a *Adapter) Name() string
func (a *Adapter) IsAvailable(ctx context.Context) bool
func (a *Adapter) Close(ctx context.Context) error

// --- TypedStore stays the same ---
// TypedStore[C] implements provider.ContextStore[C] — already exists
func NewTypedStore[C any](adapter *Adapter, prefix string) *TypedStore[C]
```

---

## 12. Database Adapter

Already well-structured. Minor enhancement:

```go
package database

// Adapter wraps GORM with proper adapter contract.
// Replaces the current DB struct.
type Adapter struct {
    config Config
    gormDB *gorm.DB
    log    *logger.Logger
}

func New(cfg Config, dialector gorm.Dialector, log *logger.Logger) (*Adapter, error)

// GormDB returns the underlying GORM DB for direct use.
func (a *Adapter) GormDB() *gorm.DB

// provider.Provider
func (a *Adapter) Name() string
func (a *Adapter) IsAvailable(ctx context.Context) bool
func (a *Adapter) Close(ctx context.Context) error

// Component stays the same
```

---

## 13. Storage Adapter

Already the closest to the pattern. Minor cleanup:

```go
package storage

// Adapter IS the storage interface implementation.
// Already follows the adapter pattern via Factory + Component.
// Enhancement: Adapter itself implements provider.Provider.
type Adapter struct {
    config  Config
    storage Storage  // the actual backend (local, s3, supabase)
    log     *logger.Logger
}

func (a *Adapter) Name() string
func (a *Adapter) IsAvailable(ctx context.Context) bool
```

---

## 14. Derivation: How LLM Would Be Built

This is NOT part of gokit. This shows how a project (or a future gokit module) would derive from the HTTP adapter.

```go
// In a project or in gokit/adapter/llm (future)
package llm

import httpadapter "github.com/kbukum/gokit/http"

// Config extends HTTP adapter config with LLM-specific fields.
type Config struct {
    httpadapter.Config `yaml:",inline" mapstructure:",squash"`

    // LLM-specific
    Model       string  `yaml:"model"`
    Temperature float64 `yaml:"temperature"`
    MaxTokens   int     `yaml:"max_tokens,omitempty"`
}

// Adapter is an LLM adapter derived from the HTTP adapter.
type Adapter struct {
    http   *httpadapter.Adapter
    config Config
}

func New(cfg Config, opts ...Option) (*Adapter, error) {
    // Create the HTTP adapter with the embedded HTTP config
    httpAdapter, err := httpadapter.New(cfg.Config)
    if err != nil {
        return nil, err
    }
    return &Adapter{http: httpAdapter, config: cfg}, nil
}

// Execute sends a typed completion request.
func (a *Adapter) Execute(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    // Build HTTP request — this is where the LLM adapter knows its provider's format
    httpReq := a.buildRequest(req)
    httpResp, err := a.http.Do(ctx, httpReq)
    if err != nil {
        return nil, err
    }
    return a.parseResponse(httpResp)
}

// buildRequest — each LLM derivation implements this differently
func (a *Adapter) buildRequest(req CompletionRequest) *httpadapter.Request { ... }

// parseResponse — each LLM derivation implements this differently
func (a *Adapter) parseResponse(resp *httpadapter.Response) (*CompletionResponse, error) { ... }

// provider.Provider
func (a *Adapter) Name() string
func (a *Adapter) IsAvailable(ctx context.Context) bool

// HTTP gives access to the full HTTP adapter when needed
func (a *Adapter) HTTP() *httpadapter.Adapter
```

Then **Ollama** and **OpenAI** would further derive from the LLM adapter:

```go
// ollama/adapter.go
package ollama

import "project/adapter/llm"

type Adapter struct {
    *llm.Adapter
}

func New(cfg llm.Config) (*Adapter, error) {
    base, err := llm.New(cfg)
    // Ollama-specific setup...
    return &Adapter{base}, nil
}

// Override buildRequest/parseResponse for Ollama's JSON format
```

Or alternatively, the LLM adapter itself handles different providers through its buildRequest/parseResponse methods based on configuration — that's the project's choice, not gokit's.

---

## 15. Implementation Phases

### Phase 1: Provider Enhancement + HTTP Adapter

**Goal**: Establish the adapter pattern with the most important module.

| # | Task | Description |
|---|------|-------------|
| 1.1 | Provider health | Add optional `HealthChecker` interface to `provider` package |
| 1.2 | HTTP adapter | Rewrite `httpclient` as `http.Adapter` — Config, New, Do, DoStream, Execute, Close |
| 1.3 | HTTP REST helpers | Generic `Get[T]`, `Post[T]`, etc. as functions on Adapter |
| 1.4 | HTTP SSE | StreamResponse with SSE reader built in (merge `sse` sub-package) |
| 1.5 | HTTP Component | Component wrapping Adapter for lifecycle |
| 1.6 | HTTP tests | Full test coverage |
| 1.7 | Migration guide | How to move from `httpclient.Client` to `http.Adapter` |

**Exit criteria**: HTTP adapter works as both a simple client and a provider. All existing httpclient functionality preserved.

### Phase 2: gRPC + Kafka Adapters

| # | Task | Description |
|---|------|-------------|
| 2.1 | gRPC adapter | Enhance `grpc` module — Adapter wraps conn, proper lifecycle, `ClientOf[T]` |
| 2.2 | Kafka producer adapter | Enhance kafka producer — ProducerAdapter IS a `Sink[Message]` |
| 2.3 | Kafka consumer adapter | Enhance kafka consumer — ConsumerAdapter IS a `Stream` |
| 2.4 | Tests | Full coverage |

### Phase 3: Process + Redis + Database Adapters

| # | Task | Description |
|---|------|-------------|
| 3.1 | Process adapter | Enhance `process` — Adapter with Config, implements RequestResponse |
| 3.2 | Redis adapter | Enhance `redis` — Adapter implements Provider, keeps direct methods |
| 3.3 | Database adapter | Minor — rename DB → Adapter, add Provider interface |
| 3.4 | Storage cleanup | Minor — ensure Adapter implements Provider |
| 3.5 | Tests | Full coverage |

### Phase 4: Documentation + Examples

| # | Task | Description |
|---|------|-------------|
| 4.1 | Adapter guide | "How to write a gokit adapter" document |
| 4.2 | Derivation guide | "How to derive domain adapters (LLM, etc.)" document |
| 4.3 | Migration guide | Module-by-module migration from old APIs |
| 4.4 | Examples | Example derivation: LLM adapter using HTTP adapter |

---

## 16. Design Principles

### 1. The Adapter IS the Provider

No separate "provider wrapper." The adapter itself implements `provider.RequestResponse`, `provider.Sink`, `provider.Stream`, or `provider.Duplex` directly. This means any adapter automatically works with `WithResilience`, `Chain`, `Manager`, etc.

### 2. Full Capability, Always

The adapter is a proper native client. HTTP adapter does everything `httpclient` does today. gRPC adapter gives you a real `grpc.ClientConn`. Kafka adapter gives you real producer/consumer. The provider interface is **in addition to**, not **instead of**, native capability.

### 3. Config-Driven but Not Config-Mandated

Every adapter can be created from a `Config` struct. That struct can come from:
- Hardcoded Go code
- YAML file
- Database
- Environment variables
- Whatever the project wants

GoKit doesn't provide a "spec loader." GoKit provides the adapter that accepts a config. The project decides how to get that config.

### 4. Derivation Through Composition

Domain-specific adapters (LLM, webhook, etc.) are built by wrapping a native adapter:

```go
type LLMAdapter struct {
    http *httpadapter.Adapter  // has-a HTTP adapter
}
```

The derivation adds domain logic (typed requests, response parsing). The native adapter provides transport. Projects or future gokit modules can create these derivations.

### 5. Consistent Structure, Not Forced Interfaces

All adapters follow the same structural pattern:
- `Config` → `Adapter` → `Provider interface` → `Component`

But they don't share a single generic interface. An HTTP adapter has `Do()`. A Kafka producer has `Publish()`. A process adapter has `Run()`. Each has native methods that make sense for its protocol.

### 6. GoKit Provides Native Adapters. Projects Build Domain Adapters.

GoKit's scope:
- HTTP adapter (transport)
- gRPC adapter (transport)
- Kafka adapter (transport)
- Process adapter (transport)
- Redis adapter (data store)
- Database adapter (data store)
- Storage adapter (data store)

Project's scope:
- LLM adapter (derives from HTTP)
- Payment adapter (derives from HTTP)
- Notification adapter (derives from HTTP or gRPC)
- Whatever domain-specific adapters they need
