# logging

Structured logging over `log/slog`, with masking, sampling, component levels, context fields, and optional OTLP export. Inject a logger at composition; custom sinks use the same governed handler chain.

## Quick Start

```go
package main

import "github.com/kbukum/gokit/logging"

func main() {
    log := logging.NewDefault("my-service")
    log.WithComponent("database").Info("connected", logging.Fields("pool", "primary"))
}
```

Use `New(cfg, name, ...Option)` for configurable output and handle its error. File creation and exporter initialization can fail. `MustNew` is only for known-good test or static configuration, not user-supplied runtime settings.

## Ownership

Close the root logger with `Close()` or deadline-bounded `Shutdown(ctx)`. Release is idempotent: it flushes OTLP, stops the exporter, and closes the file sink. An already-expired deadline skips flushing but still stops the exporter.

Derived loggers share sinks and do not release them. `WithComponent`, `WithFields`, `WithError`, and `WithContext` replace an existing attribute of the same key rather than duplicating it.

## Bring Your Own Sink / Logging Port

All sinks receive records through component-level filtering, sampling, masking, and context enrichment.

| Option | Purpose |
|---|---|
| `WithHandler` | Add a `slog.Handler` alongside the default sink |
| `WithBaseSink` | Replace the default sink while retaining middleware |
| `WithWriter` | Redirect the default sink; ignored with a custom base sink |
| `WithMasker` | Inject masking and enable it regardless of the config flag |
| `WithClock` | Inject sampling time |
| `Logger.Slog()` | Use the governed logger through the standard `slog` API |

## Configuration

```yaml
logging:
  level: info
  format: json
  output: stdout
  timestamp: true
  caller: false
  service_name: my-service
  masking:
    enabled: true
    replacement: "[REDACTED]"
    preserve_last: 0
    field_names: [internal_secret]
  sampling:
    enabled: false
    initial_rate: 100
    thereafter_rate: 100
  module_levels:
    database: debug
  otlp:
    enabled: false
    endpoint: "localhost:4317"
    protocol: grpc
    insecure: false
```

Formats are `json`, `console`, `text`, or `pretty`; levels include `trace` through `fatal`. File output uses an output mapping with `type: file` and `path`. `Fatal` logs but does not exit the process. See [Config](config.go) for the complete fields.

## Masking

Masking is enabled by default and applies to structured values before every sink. **Free-text messages are not masked:** keep dynamic data in fields, never interpolate secrets into the message.

Default rules cover credential and sensitive field names, JWTs, bearer credentials, AWS keys, payment identifiers, email addresses, and long hex values. Add field names or regex patterns through `MaskingConfig`; `preserve_last` exposes a chosen suffix and defaults to zero.

A valid W3C trace ID in `trace_id` is preserved for correlation; malformed values and values under other keys remain subject to masking. See [masking rules](masking.go) for the exact matchers.

### Custom Masker

Implement `Masker.MaskValue(key, value string) string` and pass it through `WithMasker`. The same middleware then applies it to local, OTLP, and custom sinks.

## Sampling

Sampling is opt-in. Each level has a one-second window: the first `initial_rate` records pass, then every `thereafter_rate`-th record passes. Use `WithClock` for deterministic tests.

## Module Levels

`module_levels` overrides the base level for names applied through `WithComponent`. This allows targeted debugging without increasing every component's output.

## OTLP Export

Enable `OTLPConfig` to export alongside local output. Choose `grpc` or `http`, configure the collector, and supply credentials through injected headers. TLS is the default; plaintext requires explicit `insecure`.

The logger owns its exporter. Flush and release it before process exit; derived loggers must not close shared resources.

## Unified Schema

Use [field constants and helpers](fields.go) for service, environment, version, component, trace/span/request IDs, operation, status, and duration. `Fields`, `ServiceFields`, `ErrorFields`, and `DurationFields` build structured attributes.

Use `WithContext` and the helpers in [context.go](context.go) for contextual correlation. Never turn raw credentials or private failure causes into fields merely because masking exists.

## API Reference

See [package documentation](doc.go), [logger APIs](logger.go), and [executable examples](example_test.go) for constructors, options, scoped logging, and lifecycle usage.

[Back to gokit](../README.md)
