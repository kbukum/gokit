# observability

Instance-owned OpenTelemetry tracing, metrics and health checks.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
cfg := observability.DefaultTracerConfig("my-service")
tracing, err := observability.NewTraceComponent(cfg, log)
if err != nil {
    return err
}
// Register before listeners; bootstrap honors its telemetry shutdown phase.
if err := app.RegisterComponent(tracing); err != nil {
    return err
}
tracer := tracing.Tracer("my-service")
// Inject tracer and propagation.TraceContext{} into HTTP/Connect instrumentation.
// For root kit operations (providers, DAGs, agents, tools, messaging):
ctx, err = observability.WithTracerProvider(ctx, tracing)
if err != nil {
    return err
}
```

Construction starts no exporter or worker, so pre-start installation failure has no tracing resources to release. `Start` creates the exporter; `Stop` flushes after application resources and reserves one quarter of its budget to join a stalled exporter. Do not close an injected logger or client from this component.

Defaults: 512 queued spans (maximum 65,536), 64 per batch, one-second batch interval, two-second export timeout and five-second flush budget (clamped by the caller). A full SDK queue drops spans rather than blocking requests. Export failures degrade tracing health and emit safe diagnostics; raw exporter errors never enter logs. HTTP export owns and closes its connection pool, uses system TLS trust and explicit request budgets; SDK HTTP-client environment overrides do not configure this pool. `WithTraceExporter` injects a cancellation-aware, resource-owning factory for custom TLS, another exporter or deterministic tests.

`InitTracer` is the lower-level, immediately started provider API: its caller owns shutdown. Both construction paths sanitize exporter failures while preserving their underlying causes. Neither constructor registers OTel globals. `WithTracerProvider(ctx, tracing)` accepts either a `TraceComponent` or an SDK provider and injects it for root kit operations and extracted remote parents. `Tracer(ctx, name)`, `StartSpan`, and `StartNamedSpan` use that provider; without explicit injection they use the active span's provider, so instrumentation beneath an instance-owned HTTP/Connect span records real children. Without an injected provider or local span, tracing is a no-op. `InjectTraceContext` and `ExtractTraceContext` use W3C TraceContext directly, never global propagation or baggage; callers authenticate peers before accepting remote parents.

At public ingress, ignore incoming parent/sampling and baggage. Internal parents are accepted only after peer authentication, and an accepted parent's sampling decision is honored; `SampleRate` applies to new root traces. Use registered procedures or route templates, never raw paths, payloads, credentials or error text. Providers disable automatic SDK panic recording without swallowing panics. `Correlate(ctx)` derives logging identifiers from the active span; `TraceID(ctx)` supplies transport error correlation.

## Key Types & Functions

`NewPrometheusComponent` owns one private Prometheus registry, OTel pull meter provider, and runtime/process collectors. Inject `Meter` into transports/application instruments and `Handler` into the private administration listener; register the component in the telemetry shutdown phase. It never changes OTel or Prometheus globals. Collection has a 512-datapoint cardinality limit per instrument, at most two concurrent scrapes and a two-second application-callback deadline. Scrapes collect application metrics one at a time; a callback failure, deadline, or stopped component rejects the scrape with HTTP 500 and a fixed message rather than serving partial metrics or callback error text. Observable callbacks must honor cancellation. Use fixed route and outcome labels, never tenant IDs, credentials, bodies or raw paths. Shutdown cancels active application collections and drains them within the caller's deadline before releasing readers. A callback that ignores cancellation keeps teardown incomplete and retryable rather than reporting success; there is no background exporter queue.

| Name | Description |
|------|-------------|
| `NewTraceComponent()` / `TracerConfig` | Instance-owned tracing and telemetry-phase shutdown |
| `InitTracer()` | Immediately started provider with caller-owned shutdown |
| `WithTracerProvider()` / `Tracer(ctx, name)` | Context-owned provider injection and named tracing |
| `Correlate()` / `TraceID()` | Span-derived logging and error identifiers |
| `InitMeter()` / `MeterConfig` | OpenTelemetry metrics setup |
| `NewPrometheusComponent()` | Isolated application/runtime scrape and lifecycle-owned pull reader |
| `StartSpan()` / `SpanFromContext()` | Distributed tracing helpers |
| `SetSpanAttributes()` / `SetSpanError()` | Span enrichment |
| `ObserveGauges()` / `Int64Gauge()` / `Float64Gauge()` | Observable gauges that read one snapshot per collection |
| `Metrics` | Pre-built metric instruments for requests, operations, errors |
| `OperationContext` | Combines tracing + metrics for tracked operations |
| `ServiceHealth` / `HealthChecker` | Service health aggregation |

---

[⬅ Back to main README](../README.md)
