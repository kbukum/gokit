# gokit/logging

Production-ready structured logging built on the standard library's [`log/slog`](https://pkg.go.dev/log/slog).

The `Logger` is a thin ergonomic facade over `*slog.Logger`. All the value-add behavior — masking, sampling, per-module levels, context enrichment, OTLP export — lives in a composable `slog.Handler` middleware chain, so the logging engine is never baked into your code. You inject a `*logging.Logger` at the composition root; there is no process-global logger on the consumer path. When you need stdlib-native slog, `Logger.Slog()` hands you the underlying `*slog.Logger` with the same governed handler chain attached, and you can bring your own sink with `WithHandler` / `WithBaseSink` without editing the kit.

## Features

- Structured JSON / console output over `log/slog`
- Bring-your-own `slog.Handler` sink — pluggable, no kit edits (see [Logging port](#bring-your-own-sink--logging-port))
- Sensitive data masking (**on by default**)
- Rate-based log sampling (burst + thereafter), clock-injectable for deterministic tests
- Per-module log level overrides
- OpenTelemetry Logs bridge (OTLP export)
- Unified log schema (consistent across gokit, pykit, rskit)
- Context propagation (trace ID, span ID, correlation ID)

## Quick Start

```go
package main

import "github.com/kbukum/gokit/logging"

func main() {
    // Default logger — masking enabled, console format, info level.
    // NewDefault never enables OTLP, so it cannot fail and returns a single value.
    log := logging.NewDefault("my-service")
    log.Info("server started", logging.Fields("port", 8080))

    // Component-scoped logger (applies any per-module level override)
    dbLog := log.WithComponent("database")
    dbLog.Debug("query executed", logging.DurationFields("select", elapsed))

    // Inject the logger everywhere — there is no package-level global.
    // Need stdlib-native slog? Reach through the escape hatch:
    slogger := log.Slog() // *slog.Logger with the full governed handler chain
    _ = slogger

    // Sensitive data is automatically redacted
    log.Info("user login", logging.Fields("password", "hunter2"))
    // output: password=***REDACTED***
}
```

`New` returns `(*Logger, error)` — construction fails only when an enabled OTLP exporter cannot
initialize, so handle it at the composition root:

```go
cfg := &logging.Config{Level: "info", Format: "json", ServiceName: "my-service"}
log, err := logging.New(cfg, "my-service")
if err != nil {
    return err
}
defer log.Close()
```

For configurations known-good at author time (no OTLP, or a config validated elsewhere) and for
tests, use `MustNew` — the sanctioned Must-twin that panics instead of returning an error, mirroring
`regexp.MustCompile`. Do not use it on runtime or user-supplied config paths.

```go
log := logging.MustNew(&logging.Config{Level: "debug", Format: "console"}, "my-service")
```

## Bring Your Own Sink / Logging Port

The logging engine is not baked into gokit. `Logger` is a facade over `*slog.Logger`, and every
value-add feature is a `slog.Handler` in a middleware chain:

```
moduleLevel → sampling → masking → context → fanout{ default sink, OTLP, your handler(s)… }
```

Out of the box you get the default sink (console in dev, JSON in prod) with masking on — zero config.
When you need something else, four constructor options and one escape hatch cover it, and none of
them require editing the kit:

| Seam | What it does |
|------|--------------|
| `Logger.Slog()` | Escape hatch: the underlying `*slog.Logger`, carrying the full governed handler chain. Hand it to any code that speaks stdlib slog. |
| `WithHandler(h slog.Handler)` | **Add** a sink alongside the default. Your handler still sits behind masking/sampling/module-levels, so policy stays enforced. Bridge to zerolog/zap or a custom backend here. Pass it multiple times to fan out. |
| `WithBaseSink(h slog.Handler)` | **Replace** the default JSON/console sink entirely while keeping gokit's middleware in front. Use it to own the terminal output format. |
| `WithWriter(w io.Writer)` | Point the default sink at any `io.Writer` (a file, a buffer, a test sink) instead of the configured stdout/stderr. No effect when `WithBaseSink` supplies a custom sink. |
| `WithMasker(m Masker)` | Supply a custom `Masker` as a first-class seam. Passing one **enables masking even when the config leaves it disabled** (see [Custom Masker](#custom-masker)). |
| `WithClock(now func() time.Time)` | Inject the clock the sampler reads, for deterministic tests. |

```go
// Bring your own sink — records are still masked, sampled, and module-gated:
zapBridge := myslogzap.NewHandler(zapLogger)          // any slog.Handler
log := logging.MustNew(cfg, "my-service", logging.WithHandler(zapBridge))

// Or fully own the terminal format while keeping the middleware chain:
log = logging.MustNew(cfg, "my-service",
    logging.WithBaseSink(slog.NewJSONHandler(os.Stdout, nil)))

// Or just redirect the default sink to a buffer in a test:
var buf bytes.Buffer
log = logging.MustNew(cfg, "my-service", logging.WithWriter(&buf))
```

Because the middleware wraps the fanout, masking and sampling apply uniformly to the default sink,
the OTLP branch, and every consumer-supplied handler — you cannot accidentally bypass redaction by
bringing your own backend.

## Configuration

```yaml
logging:
  level: info              # debug | info | warn | error | fatal | trace
  format: json             # json | console | text
  output: stdout           # stdout | stderr
  no_color: false
  timestamp: true
  caller: false
  service_name: my-service

  # Sensitive data masking
  masking:
    enabled: true           # on by default
    field_names:             # additional field names to redact
      - my_secret_field
    value_patterns:          # additional regex patterns
      - 'MYSECRET_[A-Z0-9]{20}'
    replacement: "***REDACTED***"
    preserve_last: 0         # preserve last N chars (0 = full redaction)

  # Rate-based sampling
  sampling:
    enabled: false
    initial_rate: 100        # allow first N per second per level
    thereafter_rate: 100     # then keep every Nth

  # Per-module log level overrides
  module_levels:
    database: debug
    kafka: warn
    auth: trace

  # OpenTelemetry OTLP export
  otlp:
    enabled: false
    endpoint: "localhost:4317"
    protocol: grpc           # grpc | http
    insecure: false
    headers:
      x-api-key: "my-key"
```

## Masking

Masking is **enabled by default**.
Every log field is checked against sensitive field names (case-insensitive) and value patterns (regex). If a match is found, the value is replaced before it reaches any output sink.

Masking applies to structured attribute **values**, not to the free-text log message. Keep dynamic data in fields (`logger.Fields(...)`), never interpolated into the message string, so it can be redacted.

### Default Masked Fields

| # | Field Name | Description |
|---|-----------|-------------|
| 1 | `password` | User passwords |
| 2 | `secret` | Generic secrets |
| 3 | `token` | Generic tokens |
| 4 | `api_key` | API keys |
| 5 | `apikey` | API keys (alternate) |
| 6 | `api-key` | API keys (hyphenated) |
| 7 | `authorization` | Auth headers |
| 8 | `auth_token` | Authentication tokens |
| 9 | `access_token` | OAuth access tokens |
| 10 | `refresh_token` | OAuth refresh tokens |
| 11 | `private_key` | Private keys |
| 12 | `ssn` | Social Security numbers |
| 13 | `credit_card` | Credit card numbers |
| 14 | `card_number` | Card numbers (alternate) |
| 15 | `cvv` | Card verification values |
| 16 | `pin` | Personal identification numbers |

### Value Patterns

These patterns detect sensitive data regardless of field name:

| # | Pattern | Example Input | Masked Output |
|---|---------|---------------|---------------|
| 1 | JWT | `eyJhbGci...payload...sig` | `[JWT_REDACTED]` |
| 2 | Bearer token | `Bearer abc123def` | `Bearer [REDACTED]` |
| 3 | AWS Access Key | `AKIAIOSFODNN7EXAMPLE` | `[AWS_KEY_REDACTED]` |
| 4 | Credit Card | `4111-1111-1111-1234` | `****-****-****-1234` |
| 5 | SSN | `123-45-6789` | `***-**-****` |
| 6 | Email | `user@example.com` | `***@***.***` |
| 7 | Hex Secret (32+) | `a1b2c3d4e5f6...` (32+ hex chars) | `[HEX_REDACTED]` |

### Adding Custom Fields and Patterns

```yaml
masking:
  field_names:
    - my_internal_token
    - employee_id
  value_patterns:
    - 'MYSVC_[A-Za-z0-9]{32}'
```

### Partial Masking

Use `preserve_last` to keep the last N characters visible:

```yaml
masking:
  preserve_last: 4
```

This turns `"password": "hunter2"` into `"password": "***REDACTED***ter2"`.

## Sampling

Sampling reduces log volume in high-throughput services. When enabled, each log level gets an independent counter per one-second window:

1. **Burst** — the first `initial_rate` messages per second per level pass through unconditionally.
2. **Thereafter** — after the burst, only every `thereafter_rate`-th message is kept.

```yaml
sampling:
  enabled: true
  initial_rate: 100     # allow first 100/sec per level
  thereafter_rate: 10   # then keep every 10th
```

> **When to use:** Enable sampling on hot-path services producing thousands of log lines per second.
> Leave disabled for low-volume services or during debugging.

Sampling is implemented as a pure-Go `slog.Handler` decorator (`samplingHandler`) — no third-party
sampler. Each one-second window is tracked with an injected clock, so behavior is deterministic under
test. Inject the clock with `logging.WithClock(func() time.Time { ... })` when you need to drive the
window from a fake clock; it defaults to `time.Now`.

```go
// Deterministic sampling in a test:
now := fakeClock.Now
log := logging.MustNew(cfg, "svc", logging.WithClock(now))
```

## Module Levels

Override the global log level for specific components. Useful for silencing noisy dependencies or enabling debug output for a single subsystem.

```yaml
logging:
  level: info
  module_levels:
    database: debug     # verbose DB logs
    kafka: warn         # suppress Kafka noise
    auth: trace         # detailed auth tracing
```

```go
// Programmatic usage
log := logging.MustNew(cfg, "my-service")

// WithComponent applies the module-level override automatically
dbLog := log.WithComponent("database")   // → debug level
kafkaLog := log.WithComponent("kafka")   // → warn level
```

Module levels are configured via the `module_levels` map (config or `logging.Config`) and applied automatically by `WithComponent`. The underlying `ModuleLevelManager` is thread-safe; its `SetLevel()` is used internally when levels are established from configuration.

## OTLP Export

The OpenTelemetry Logs bridge sends log records to an OTLP collector alongside your local output. Logs are emitted via the OTel SDK `LoggerProvider` with batch processing.

### Setup

```yaml
otlp:
  enabled: true
  endpoint: "otel-collector:4317"
  protocol: grpc        # grpc | http
  insecure: true        # skip TLS for dev
  headers:
    Authorization: "Bearer my-token"
```

### Programmatic Usage

```go
cfg := &logging.Config{
    Level:       "info",
    Format:      "json",
    ServiceName: "my-service",
    OTLP: logging.OTLPConfig{
        Enabled:  true,
        Endpoint: "localhost:4317",
        Protocol: "grpc",
        Insecure: true,
    },
}
log, err := logging.New(cfg, "my-service")
if err != nil {
    return err
}
defer log.Close()  // flush pending OTLP logs on shutdown

log.Info("order created", logging.Fields("order_id", "abc-123"))
```

### Graceful Shutdown

Always call `Close()` before process exit to flush buffered log records:

```go
log, err := logging.New(cfg, "my-service")
if err != nil {
    return err
}
defer log.Close()
```

## Unified Schema

All three kits (gokit, pykit, rskit) share the same structured field names:

| Field | Constant | Description |
|-------|----------|-------------|
| `service` | `FieldService` | Service name |
| `environment` | `FieldEnvironment` | Deployment environment |
| `version` | `FieldVersion` | Service version |
| `component` | `FieldComponent` | Logical component |
| `trace_id` | `FieldTraceID` | Distributed trace ID |
| `span_id` | `FieldSpanID` | Span ID within trace |
| `correlation_id` | `FieldCorrelationID` | Cross-service correlation |
| `request_id` | `FieldRequestID` | HTTP request ID |
| `user_id` | `FieldUserID` | User identifier |
| `session_id` | `FieldSessionID` | Session identifier |
| `operation` | `FieldOperation` | Operation name |
| `status` | `FieldStatus` | Operation status |
| `error` | `FieldError` | Error message |
| `duration_ms` | `FieldDuration` | Duration in milliseconds |
| `timestamp` | `FieldTimestamp` | ISO 8601 timestamp |
| `level` | `FieldLevel` | Log level |
| `message` | `FieldMessage` | Log message |

### ServiceFields Helper

Attach standard service identification to any log entry:

```go
svcFields := logging.ServiceFields("order-svc", "production", "1.2.3")
log.Info("service started", svcFields)
// → {"service":"order-svc","environment":"production","version":"1.2.3",...}
```

### Field Helpers

```go
// Build fields from key-value pairs
logging.Fields("op", "save", "id", 42)

// Error fields
logging.ErrorFields("db.connect", err)

// Duration fields
logging.DurationFields("query", 150*time.Millisecond)

// Merge helpers
fields := logging.Fields("op", "save")
logging.MergeWithError(fields, err)
logging.MergeWithDuration(fields, elapsed)
```

## Custom Masker

Implement the `Masker` interface to provide your own masking logic:

```go
type Masker interface {
    MaskValue(key, value string) string
}
```

Masking is applied as a `slog.Handler` in the chain, not as a mutable field on the logger. Supply
your masker at construction with the `WithMasker` option — passing one also turns masking on even if
the config leaves it disabled:

```go
type MyMasker struct{}

func (m *MyMasker) MaskValue(key, value string) string {
    if key == "internal_id" {
        return "***"
    }
    return value
}

log := logging.MustNew(cfg, "my-service", logging.WithMasker(&MyMasker{}))
log.Info("event", logging.Fields("internal_id", "secret-123"))
// → internal_id=***
```

## API Reference

| Function / Type | Description |
|----------------|-------------|
| `New(cfg, name, ...Option)` | Create logger from config; returns `(*Logger, error)` (errors only on OTLP init) |
| `MustNew(cfg, name, ...Option)` | Like `New` but panics on error — for compile-safe/test configs |
| `NewDefault(name)` | Create a console logger with defaults; single return, never fails |
| `NewFromEnv(name)` | Create logger from `LOG_LEVEL`, `LOG_FORMAT`, … env vars; returns `(*Logger, error)` |
| `WithHandler(h)` | Option: add a BYO `slog.Handler` sink behind masking/sampling/module-levels |
| `WithBaseSink(h)` | Option: replace the default sink, keep the middleware chain |
| `WithWriter(w)` | Option: direct the default sink to any `io.Writer` |
| `WithMasker(m)` | Option: supply a custom `Masker` (also enables masking) |
| `WithClock(now)` | Option: inject the sampler clock for deterministic tests |
| `Logger.Slog()` | Escape hatch: the underlying `*slog.Logger` |
| `Logger.Handler()` | The root `slog.Handler` backing the logger |
| `WithContext(ctx)` | Enrich with trace/span/request IDs from context |
| `ContextWith*(ctx, id)` | Inject trace/span/request/user/correlation ID into a context |
| `ComponentSpan(ctx, name)` | Component-tagged logger enriched from context |
| `RequestSpan(ctx, method, path, requestID)` | Logger enriched with HTTP request metadata |
| `WithComponent(name)` | Tag with component + apply module level |
| `WithFields(map)` | Add structured fields |
| `WithError(err)` | Add error field |
| `SetLevel(level)` / `Level()` | Get/set the base level at runtime |
| `Close()` | Flush OTLP and shut down |
| `Debug` / `Info` / `Warn` / `Error` / `Trace` / `Fatal` (+ `*Ctx` variants) | Log at level |

---

[⬅ Back to main README](../README.md)
