# Architecture reuse audit — gokit

## Scope

This audit records reusable infrastructure and pattern decisions that every `gokit` module follows.

## Messaging findings

| Finding | Evidence | Owner | Layer | Reuse mode | Decision |
|---------|----------|-------|-------|------------|----------|
| Messaging backend registry is explicit | `messaging.Registry`, `NewRegistry`, and adapter `Register(registry)` functions select backends from broker-neutral config | `messaging` | L6 | Follow/Inject | **Enhance complete** — registries are application-owned, injected, config-driven, and core remains driver-agnostic. |
| Producer JSON API exposes an opaque payload seam | `Producer.PublishJSON(ctx, topic, key string, value any)` | `messaging` plus codec owner | L6 | Consume | **Leave** — keep the opaque JSON value documented and prefer typed event publishers at application boundaries. |
| Retry middleware consumes canonical resilience | `messaging/middleware/retry.go` uses `resilience.RetryFunc` | `resilience` | L3 | Consume | **Leave** — keep retry/backoff ownership in `resilience` and use this as the cross-kit reference. |
| Runtime errors and config validation are plain `fmt.Errorf` in several paths | `messaging/memory`, `messaging/kafka/config.go`, and adapter helpers return plain errors | `errors` | L0 | Consume | **Align** — convert user/runtime failures to `errors.AppError` with typed codes while preserving wrapped causes. |
| Broker security defaults require explicit insecure development opt-in | Kafka validates TLS unless `AllowInsecureDev`; NATS requires `tls://`/`wss://`; RabbitMQ requires `amqps://` | `security` / `messaging/*` | L3/L6 | Consume/Follow | **Redesign complete** — secure defaults, typed credentials, and topic/subject/queue/group validation are documented. |
| DLQ envelope uses cross-kit vocabulary | `messaging/middleware.DeadLetterEnvelope` uses `original_topic`, `error`, `retry_count`, `timestamp`, `headers`, and `payload` with redaction | `messaging` | L6 | Follow | **Align complete** — DLQ routing is opt-in middleware behavior and adapter-managed DLQ is rejected when unsupported. |
| NATS and RabbitMQ adapters are opt-in | `messaging/nats` and `messaging/rabbitmq` exist as explicit-registration subpackages | `messaging` | L6 | Follow | **Enhance complete** — adapters are real opt-in implementations with isolated SDK dependencies and no core driver dependency. |

## Messaging reuse rules

- All retry, timeout, circuit breaker, rate limit, and bulkhead behavior must consume `resilience`.
- All spans, metrics, logs, and redaction hooks must be injected through canonical observability/logging APIs.
- All backend selection is application-owned, injected, and config-driven.
- Importing `messaging` or an adapter package must not register or dial anything implicitly.

## AI/ML and agent findings

| Finding | Evidence | Owner | Layer | Reuse mode | Decision |
|---------|----------|-------|-------|------------|----------|
| MCP exposes protocol-shaped surfaces | `mcp` wraps kit tools, remote MCP tools, prompts, resources/templates, roots, sampling, elicitation, cancellation, progress, logging, stdio, and Streamable HTTP | `mcp` | L7 | Follow | **Align complete** — keep protocol surfaces in `mcp` without leaking SDK types as the only API. |
| MCP server construction applies explicit safety policy | Server calls pass through allow-list, authz, audit logging, payload/result limits, structured output validation, Origin validation, local bind defaults, and HTTP auth for exposed endpoints | `mcp`, `authz`, `security`, `observability` | L7/L6/L3 | Consume/Inject | **Enhance complete** — keep safety policy injected and fail-closed. |
| Tool middleware composes reusable policy concerns | `tool` owns tool-domain composition while retry/timeout/metrics/logging/validation/security policy comes from canonical owners | `resilience`, `observability`, `validation`, `schema`, `authz`, `security`, `tool` | L3/L1/L6/L7 | Consume | **Align complete** — keep tool-domain composition local and reusable policy in canonical owners. |
| Agent loop uses canonical run policy | `agent` has run/stream loops, hooks, memory, context compaction, token budget, resilience policy, and observability spans | `agent`, `resilience`, `observability` | L7/L3 | Consume/Inject | **Enhance complete** — thread deadline, token budget, cancellation, spans, and bounded/backpressured streaming through the loop and tool/provider calls. |
| Provider baseline includes Ollama | `llm/providers` includes OpenAI, Anthropic, Gemini, and Ollama adapters | `llm-providers` | L7 | Follow | **Enhance complete** — keep Ollama as an explicit opt-in adapter with no init side effects. |
| Inference module is a neutral core | `inference/` exposes shared request/response types, provider interface, and explicit adapter registry/building | `inference` | L7 | Align/Follow | **Align** — keep extending the neutral module while preserving adapter-driven backend selection. |
| Schema is the JSON Schema owner | `schema` is consumed by `tool` and `mcp` paths | `schema` | L1 | Consume | **Leave/Enhance** — keep ownership; enhance for structured output and output-schema validation as needed. |
| Agent Skills discovery is a thin loader | `skill` exposes lightweight skill manifests plus `kit.skill.json`/`SKILL.md` discovery and loading | `skill` | L7 | Enhance/Follow | **Enhance** — keep discovery/loading thin and avoid a separate runtime. |
