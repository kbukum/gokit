# Group 05.5 Architecture Reuse Audit — gokit

## Scope

This audit records the reusable infrastructure and pattern decisions that later groups must carry forward in `gokit`. Group 07 messaging extends the audit with the messaging-specific findings below.

## Group 07 messaging findings

| Finding | Evidence | Owner | Layer | Reuse mode | Decision |
|---------|----------|-------|-------|------------|----------|
| Messaging backend registry is explicit | `messaging.Registry`, `NewRegistry`, and adapter `Register(registry)` functions select backends from broker-neutral config | `messaging` | L6 | Follow/Inject | **Enhance complete** — registries are application-owned, injected, config-driven, and core remains driver-agnostic. |
| Producer JSON API exposes untyped public payload | `Producer.PublishJSON(ctx, topic, key string, value interface{})` | `messaging` plus codec owner | L6 | Redesign/Consume | **Redesign** — replace with generic/typed JSON helpers or documented opaque byte payload seams; do not keep `interface{}` as the default public API. |
| Retry middleware already consumes canonical resilience | `messaging/middleware/retry.go` uses `resilience.RetryFunc` | `resilience` | L3 | Consume | **Leave** — keep retry/backoff ownership in `resilience` and use this as the cross-kit reference. |
| Runtime errors and config validation are plain `fmt.Errorf` in several paths | `messaging/memory`, `messaging/kafka/config.go`, and adapter helpers return plain errors | `errors` | L0 | Consume | **Align** — convert user/runtime failures to `errors.AppError` with typed codes while preserving wrapped causes. |
| Broker security defaults require explicit insecure development opt-in | Kafka validates TLS unless `AllowInsecureDev`; NATS requires `tls://`/`wss://`; RabbitMQ requires `amqps://` | `security` / `messaging/*` | L3/L6 | Consume/Follow | **Redesign complete** — secure defaults, typed credentials, and topic/subject/queue/group validation are documented. |
| DLQ envelope uses cross-kit vocabulary | `messaging/middleware.DeadLetterEnvelope` uses `original_topic`, `error`, `retry_count`, `timestamp`, `headers`, and `payload` with redaction | `messaging` | L6 | Follow | **Align complete** — DLQ routing is opt-in middleware behavior and adapter-managed DLQ is rejected when unsupported. |
| NATS and RabbitMQ adapters are opt-in | `messaging/nats` and `messaging/rabbitmq` exist as explicit-registration subpackages | `messaging` | L6 | Follow | **Enhance complete** — adapters are real opt-in implementations with isolated SDK dependencies and no core driver dependency. |

## Carry-forward rules for Group 07

- All retry, timeout, circuit breaker, rate limit, and bulkhead behavior must consume `resilience`.
- All spans, metrics, logs, and redaction hooks must be injected through canonical observability/logging APIs.
- All backend selection is application-owned, injected, and config-driven.
- Importing `messaging` or an adapter package must not register or dial anything implicitly.

## Group 08 AI/ML and agent findings

| Finding | Evidence | Owner | Layer | Reuse mode | Decision |
|---------|----------|-------|-------|------------|----------|
| MCP is currently a tool bridge, not a protocol-shaped module | `mcp.NewServer`, `Connect`, and converters wrap kit tools and remote MCP tools only | `mcp` | L7 | Redesign/Follow | **Redesign** — add kit-level protocol surfaces for tools, prompts, resources/templates, roots, sampling, elicitation, cancellation, progress, logging, stdio, and Streamable HTTP without leaking SDK types as the only API. |
| MCP server lacks explicit safety policy in the public construction path | Current server path delegates directly from MCP call to registry call | `mcp`, `authz`, `security`, `observability` | L7/L6/L3 | Consume/Inject | **Enhance** — require allow-list, per-call authz, audit logging, payload/result limits, structured output validation, Origin validation, local bind defaults, and HTTP auth for exposed endpoints. |
| Tool middleware owns reusable policy concerns | `tool` currently owns logging, timeout, recovery, validation, result limiting, retry, and metrics wrappers | `resilience`, `observability`, `validation`, `schema`, `authz`, `security`, `tool` | L3/L1/L6/L7 | Consume/Drop | **Align** — keep tool-domain composition local, but move reusable retry/timeout/metrics/logging/validation/security policy to canonical owners or document a narrow Leave decision. |
| Agent loop needs canonical run policy | `agent` has run/stream loops, hooks, memory, context compaction, and token budget but not a shared budget/deadline policy seam | `agent`, `resilience`, `observability` | L7/L3 | Consume/Inject | **Enhance** — thread deadline, token budget, cancellation, spans, and bounded/backpressured streaming through the loop and tool/provider calls. |
| Provider baseline is missing Ollama | `llm/providers` includes OpenAI, Anthropic, and Gemini adapters | `llm-providers` | L7 | Follow | **Enhance** — add Ollama as an explicit opt-in adapter with no init side effects. |
| Inference module is now present as a neutral core | `inference/` now exposes shared request/response types, provider interface, and explicit adapter registry/building | `inference` | L7 | Align/Follow | **Align** — keep extending from the new neutral module base while preserving adapter-driven backend selection. |
| Schema is already the correct JSON Schema owner | `schema` is consumed by `tool` and `mcp` paths | `schema` | L1 | Consume | **Leave/Enhance** — keep ownership; enhance for structured output and output-schema validation as needed. |
| Agent Skills discovery now exists as a thin loader | `mcp` now exposes lightweight skill packs plus `kit.skill.json`/`SKILL.md` discovery and loading | `mcp` | L7 | Enhance/Follow | **Enhance** — keep discovery/loading thin; future work is richer registration/import/export, not a separate runtime. |
