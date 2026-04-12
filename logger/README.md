# gokit/logger

Production-ready structured logging built on [zerolog](https://github.com/rs/zerolog).

## Features

- Structured JSON / console output
- Sensitive data masking (**on by default**)
- Rate-based log sampling (burst + thereafter)
- Per-module log level overrides
- OpenTelemetry Logs bridge (OTLP export)
- Unified log schema (consistent across gokit, pykit, rskit)
- Context propagation (trace ID, span ID, correlation ID)

## Quick Start

```go
package main

import "github.com/kbukum/gokit/logger"

func main() {
    // Default logger — masking enabled, console format, info level
    log := logger.NewDefault("my-service")
    log.Info("server started", logger.Fields("port", 8080))

    // Component-scoped logger
    dbLog := log.WithComponent("database")
    dbLog.Debug("query executed", logger.DurationFields("select", elapsed))

    // Global logger (set once, use anywhere)
    logger.SetGlobalLogger(log)
    logger.Info("using global logger")

    // Sensitive data is automatically redacted
    log.Info("user login", logger.Fields("password", "hunter2"))
    // output: password=***REDACTED***
}
```

## Configuration

```yaml
logging:
  level: info              # debug | info | warn | error | fatal | trace
  format: json             # json | console | text
  output: stdout           # stdout | stderr
  no_color: false
  timestamp: true
  caller: false
  stacktrace: false
  service_name: my-service

  # File rotation (optional)
  max_size: 100            # megabytes
  max_backups: 3
  max_age: 28              # days
  compress: false

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

Masking is **enabled by default**. Every log field is checked against sensitive field names (case-insensitive) and value patterns (regex). If a match is found, the value is replaced before it reaches any output sink.

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

> **When to use:** Enable sampling on hot-path services producing thousands of log lines per second. Leave disabled for low-volume services or during debugging.

Sampling uses zerolog's `BurstSampler` under the hood:

```go
// Equivalent to:
&zerolog.BurstSampler{
    Burst:       100,
    Period:      time.Second,
    NextSampler: &zerolog.BasicSampler{N: 100},
}
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
log := logger.New(cfg, "my-service")

// WithComponent applies the module-level override automatically
dbLog := log.WithComponent("database")   // → debug level
kafkaLog := log.WithComponent("kafka")   // → warn level

// Dynamic update at runtime
log.moduleLevels.SetLevel("cache", "debug")
```

The `ModuleLevelManager` is thread-safe and supports runtime level changes via `SetLevel()`.

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
cfg := &logger.Config{
    Level:       "info",
    Format:      "json",
    ServiceName: "my-service",
    OTLP: logger.OTLPConfig{
        Enabled:  true,
        Endpoint: "localhost:4317",
        Protocol: "grpc",
        Insecure: true,
    },
}
log := logger.New(cfg, "my-service")
defer log.Close()  // flush pending OTLP logs on shutdown

log.Info("order created", logger.Fields("order_id", "abc-123"))
```

### Graceful Shutdown

Always call `Close()` before process exit to flush buffered log records:

```go
log := logger.New(cfg, "my-service")
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
svcFields := logger.ServiceFields("order-svc", "production", "1.2.3")
log.Info("service started", svcFields)
// → {"service":"order-svc","environment":"production","version":"1.2.3",...}
```

### Field Helpers

```go
// Build fields from key-value pairs
logger.Fields("op", "save", "id", 42)

// Error fields
logger.ErrorFields("db.connect", err)

// Duration fields
logger.DurationFields("query", 150*time.Millisecond)

// Merge helpers
fields := logger.Fields("op", "save")
logger.MergeWithError(fields, err)
logger.MergeWithDuration(fields, elapsed)
```

## Custom Masker

Implement the `Masker` interface to provide your own masking logic:

```go
type Masker interface {
    MaskValue(key string, value string) string
}
```

```go
type MyMasker struct{}

func (m *MyMasker) MaskValue(key, value string) string {
    if key == "internal_id" {
        return "***"
    }
    return value
}

log := logger.New(cfg, "my-service")
log = log.WithMasker(&MyMasker{})
log.Info("event", logger.Fields("internal_id", "secret-123"))
// → internal_id=***
```

## API Reference

| Function / Type | Description |
|----------------|-------------|
| `New(cfg, name)` | Create logger from config |
| `NewDefault(name)` | Create logger with defaults |
| `NewFromEnv(name)` | Create logger from `LOG_LEVEL`, `LOG_FORMAT` env vars |
| `Init(cfg)` | Initialize global logger |
| `SetGlobalLogger(l)` / `GetGlobalLogger()` | Global logger management |
| `WithContext(ctx)` | Enrich with trace/span/request IDs from context |
| `WithComponent(name)` | Tag with component + apply module level |
| `WithFields(map)` | Add structured fields |
| `WithError(err)` | Add error field |
| `WithMasker(m)` | Set custom masker |
| `WithOTLP(provider)` | Attach OTLP provider |
| `Close()` | Flush OTLP and shut down |
| `Debug` / `Info` / `Warn` / `Error` / `Fatal` | Log at level |

---

[⬅ Back to main README](../README.md)
