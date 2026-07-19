# Cross-Kit Parity Matrix — gokit

This matrix records the gokit side of reusable infrastructure and data/storage parity. The module-presence table is kept identical to rskit's counterpart at https://github.com/kbukum/rskit/blob/main/docs/PARITY-MATRIX.md; the capability tables below are gokit-specific.

## Module presence & naming (shared cross-kit)

Legend: ✅ present · ➖ absent · ⏳ planned (skeleton pending).

| Layer | Canonical concept | gokit | rskit | Note |
|---|---|---|---|---|
| L0 | errors | ✅ | ✅ | aligned |
| L0 | util | ✅ | ✅ | rskit `util` broader (time/env/hash/template/backoff) |
| L0 | version | ✅ | ✅ | build-info derived, immutable |
| L0 | codec | ✅ | ✅ | framing/value/json/toml |
| L0 | fs | ✅ | ✅ | safe paths, temp, atomic writes, permissions; watch is rskit-only |
| L1 | config | ✅ | ✅ | depends on `logging` for `LoggingConfig` |
| L1 | logging | ✅ | ✅ | naming aligned (was gokit `logger`) |
| L1 | validation | ✅ | ✅ | generic `Validate` seam |
| L1 | encryption | ✅ | ✅ | AES-GCM / ChaCha20 |
| L1 | schema | ✅ | ✅ | generics + compiled validator + limits |
| L2 | component / hook / provider / di | ✅ | ✅ | aligned lifecycle + 4 provider shapes |
| L3 | observability / resilience / security | ✅ | ✅ | aligned |
| L4 | bootstrap | ✅ | ✅ | aligned |
| L4 | stream | ✅ | ✅ | aligned: shared operator vocab (map/filter/fan-out/window/batch/parallel) + broadcaster + bounded backpressure (G4) |
| L4 | dag / chain / worker / process / stateful | ✅ | ✅ | aligned: `chain` is typed `Step[I,O]`/`Executor` cross-kit (no `any`) |
| L5 | server / httpclient / grpc / sse / discovery | ✅ | ✅ | aligned |
| L5 | http | ➖ | ✅ | intentional rskit-only: Axum-specific HTTP transport helper; gokit folds equivalent concerns into `server` (gin) |
| L5 | connect | ✅ | ➖ | intentional gokit-only: ConnectRPC (connect-go) adapter; rskit uses tonic/axum for the same role |
| L6 | auth / authz | ✅ | ✅ | aligned |
| L6 | database (+ sqlite) | ✅ | ✅ | aligned; both kits have a sqlite adapter |
| L6 | cache (+ redis) | ✅ | ✅ | aligned |
| L6 | storage (+ s3/gcs/supabase) | ✅ | ✅ | aligned; both have s3/gcs/supabase; gokit adds a local-fs adapter, rskit keeps the local default in core |
| L6 | vectorstore (+ qdrant) | ✅ | ✅ | aligned; both kits have memory default + qdrant adapter |
| L6 | messaging (kafka/nats/rabbitmq/memory) | ✅ | ✅ | aligned |
| L7 | ai / llm / embedding / inference | ✅ | ✅ | provider granularity differs (subdirs vs crates) |
| L7 | agent / tool / mcp / skill | ✅ | ✅ | mcp redesign to protocol-shaped tracked |
| L8 | media | ✅ | ✅ | gokit is a light **standalone module**: detection + metadata + cheap image ops + time/spatial + subtitle (SRT/VTT); heavy audio/video/matrix transcoding stays rskit by design |
| L9 | bench / git / testutil | ✅ | ✅ | aligned |
| L9 | workload | ✅ | ✅ | aligned: provider-based `Manager` + registry + component; backends stay in adapter crates |
| L9 | cli | ✅ | ✅ | aligned (light): theme/render/progress/prompt/signal + bounded live console; line + scripted prompt terminals with non-interactive fallback; raw-mode rich widgets stay rskit-only |
| L9 | dataset | ✅ | ✅ | cross-kit (light): one generic `Collector[T]` engine (bounded worker pool + `StreamBuffer` backpressure, per-source timeout/cancel, offset resume, real/AI stats, pluggable `Validator[T]`) over generic `Source`/`Transform`/`Target`; concrete item families for tabular `record.Record` (CSV/JSON-array/JSON-lines readers+writers, schema validator adapter) and blob `sample.Item` (labeled/offset, real/AI local disk target); manifest cache with one canonical `CacheStatusFor`. Deliberate divergences: gokit folds rskit's per-item `ItemSink<T>` and post-hoc directory `Target` into a single `stage.Target[T]` that publishes per source from the single-owner main loop (no shared sink); rskit's `MediaType`, rich `DataItem` metadata, and image/resize transforms stay rskit-only. pykit tracked separately |

## Infrastructure and pattern parity

| Concept | gokit shape | Cross-kit target | Status |
|---------|-------------|------------------|-------------|
| Provider shapes | `RequestResponse[I,O]`, `Stream[I,O]`, `Sink[I]`, `Duplex[I,O]` in `provider` | Same four concepts in every kit, idiomatic generics/protocols/traits | Aligned; keep all middleware/adapters within these shapes |
| Component lifecycle | `Start`, `Stop`, `Health` with health states from `component` | Same lifecycle semantics and state vocabulary cross-kit | Implementations share lifecycle semantics across cache, server, worker, storage, httpclient, and messaging |
| Hook contracts | `hook` package owns extension/event surface | Same hook concept and ordering semantics cross-kit | Extension callbacks use hook contracts where reusable ordering semantics are needed |
| DI registration | `di` plus constructor injection | Typed registration/resolve, no service locator | Typed constructors and explicit resolution avoid service-locator patterns |
| Resilience policies | retry/backoff/timeout/circuit-breaker/bulkhead/rate-limit through `resilience` and provider wrappers | Same policy names and composition order cross-kit | Boundary-specific wrappers use the resilience vocabulary |
| Config/source/format handling | `config` owns YAML/mapstructure loading, defaults, validation, masking | Config loading/source precedence/masking centralized per kit | Config loading remains centralized; structured schema work belongs in `schema` |
| Registry/backend selection | `provider` registry/selector plus package-local registries for concrete backends | Explicit injected registries, config-driven selection, no globals | Registries are application-owned and injected explicitly |
| Process execution | `process` owns argv/context/env/capture/shutdown execution | Subprocess execution only through `process` | Worker subprocess orchestration delegates to `process` |
| Cache | `cache` core with `Store`, typed JSON store, explicit `FactoryRegistry`, memory backend, and `cache/redis` adapter | Core abstraction + memory default + opt-in Redis backend | Aligned; no top-level `redis/` module remains |
| Storage | `storage` core with explicit registry/local default; `storage/s3` and `storage/gcs` nested adapters own their cloud SDK deps | Core object-store abstraction + local default + opt-in S3/GCS/Azure adapters | S3/GCS splits aligned; Supabase left in core only because it has no heavy SDK dependency |
| Database | GORM-backed contracts with explicit `DriverRegistry`/`WithDriver`; `database/sqlite` nested adapter; repository/tenant/transaction helpers | Core database contracts + opt-in drivers; tenant and transaction semantics tested | Runtime code is driver-agnostic; GORM remains the Go repository substrate for alpha |
| Vectorstore | `vectorstore` core with explicit registry, memory backend, metrics `cosine`, `dot`, `l2`, and opt-in `vectorstore/qdrant` nested adapter | In-memory default + config-driven selection + canonical metric names + opt-in Qdrant adapter | Aligned; Qdrant adapter is dependency-isolated in a nested module |
| Messaging | `messaging` core owns broker-neutral config, registry/selector, memory default, middleware, bridge, and testutil; Kafka/NATS/RabbitMQ live in opt-in subpackages with config-free `Register(registry)` and creation-time adapter config | Transport-agnostic producer/consumer + injected registry + memory default + opt-in broker adapters | Group 07 aligned — dependency-isolated adapters, secure-by-default config with explicit insecure-dev opt-ins, canonical DLQ vocabulary, and broker-neutral topic validation |

## AI / ML and agent surface parity

| Concept | gokit shape | Cross-kit target | Status |
|---------|-------------|------------------|--------|
| LLM core | `llm` owns messages, provider interface, capabilities, usage, tool calls, token counting, and stream events | Same public concepts across kits with provider-specific dialects hidden behind adapters | Aligned; streamed tool-call args are `json.RawMessage` with bounded accumulation, cross-provider stream events normalized |
| LLM providers | `llm/providers` has OpenAI, Anthropic, Gemini, and Ollama adapters (plus shared `common`) with explicit registration and no init side effects | OpenAI, Anthropic, Gemini, and Ollama in every kit; opt-in adapters with no init side effects | Aligned; request extensions are a typed JSON/YAML-authorable `RawJSON` carrier merged fail-closed via `common.MergeExtra` |
| Agent loop | `agent` has run/stream loops, hooks, memory, context compaction, token budget, commands, and read-only parallel tools | Bounded turns, wall-clock budget, token budget, cancellation propagation, backpressure, and identical hook/event semantics | Enhance: adopt canonical resilience/observability policy seams and bounded stream semantics |
| Tool definitions | `tool.Definition`, annotations, registry, batching, schema validation, and middleware | Typed tools with JSON Schema input/output, structured results, MCP annotations, explicit registry ownership | Aligned on typed I/O: untrusted input is `json.RawMessage` normalized/validated fail-closed, per-tool `*resilience.Policy` is typed (no `any`), destructive calls gated by human approval |
| MCP | `mcp` (own module) is a hardened, protocol-shaped wrapper over `modelcontextprotocol/go-sdk`: kit tools → MCP tools, remote MCP tools → kit callables | Protocol-shaped tools, prompts, resources/templates + subscribe, roots, sampling, elicitation, cancellation, progress, logging, stdio, Streamable HTTP | Redesigned to protocol-shaped module — split by concern into `types`/`server`/`capabilities`/`security`/`transport_*`/`handlers_*`; wire parsers fuzzed; ≥95% tested |
| MCP security | Fail-closed hardening chain on every `tools/call` (allow-list → input-size → schema → authz → registry HITL destructive gate → result-size → output-validate → audit); server→client helpers size-limit untrusted model/elicited content | Allow-list, authz, audit, payload/result limits, output validation, Origin validation, local bind defaults, HTTP auth | Aligned via `authz`, `security`, `observability`; Origin validation + constant-time bearer auth + localhost-default on Streamable HTTP |
| Schema | `schema` owns JSON Schema generation and validation consumed by tool/MCP paths | Schema owner for tool input/output, MCP prompts/resources/elicitation, structured LLM output, and inference APIs | Leave as owner; enhance for output/structured-content validation where needed |
| Embedding | `embedding` exposes provider abstraction and vector utilities | Provider abstraction, batch embeddings, dimensions, normalization, and endpoint ownership aligned with `llm-providers`/`inference` | Align endpoint ownership during Group 08 |
| Inference | `inference` is an independent module with neutral types, explicit registry/building, and an `openai_compatible` adapter with streaming for TGI and vLLM | Cross-kit inference module with registry/config-selected backends and adapter split | Align: extend backend set while preserving neutral module identity |
| Agent Skills packages | `skill` module owns `kit.skill.yaml` manifests, progressive-disclosure loading, injected-verifier activation with bounded reads (manifest/body/asset + aggregate), symlink/path-escape rejection, provider registry, and effective-envelope helpers | Lightweight Agent Skills-compatible discovery/loader over tools/prompts/resources/MCP; no custom runtime | Aligned with reference: manifest/loader/registry at parity, verifier wired into load (Verified/Warning/Denied), untrusted input fails closed, fuzzed manifest/activation parsers |

## Media parity (light by design)

`media` is a standalone module (`github.com/kbukum/gokit/media`) that mirrors the *surface concepts* of rskit's media core — typed `Type`/`Info`, a `Format`/`FormatInfo` catalog, an injected `Registry`, a `Prober` abstraction, the time/spatial vocabulary (`Timestamp`/`TimeRange`/`Resolution`/`FrameRate`), and subtitle handling (`SubtitleTrack`, SRT/WebVTT). Everything Go can do **without a heavy processing backend** is included; execution-only vocabulary is deliberately left out. This is a **capability decision, not a parity gap**: Go covers metadata, format/container inspection, cheap image ops, time/geometry math, and subtitle parse/serialize; the heavy path is **rskit or an external service**, never a Go reimplementation. gokit `media` has **no cgo, no ffmpeg, and no Go DSP/matrix code**.

| Capability | gokit `media` | rskit `media` | Note |
|---|---|---|---|
| Type/format detection (magic bytes) | ✅ | ✅ | fuzzed signature detector; `Detect`/`DetectReader`/`DetectFile` |
| Format catalog | ✅ `Format`/`FormatInfo` | ✅ | extension/MIME/container per format |
| Registry + injected probers | ✅ `NewRegistry`/`WithProber` | ✅ | explicit options, no globals, no `init()` side effects |
| Prober abstraction | ✅ typed `Prober`/`Metadata` | ✅ `MediaProbe` | light MediaProbe equivalent |
| Container/metadata read (dimensions) | ✅ stdlib `DecodeConfig` → `Resolution` | ✅ | JPEG/PNG/GIF |
| Cheap image ops (crop, nearest-neighbor thumbnail) | ✅ stdlib `image`/`image/draw` | ✅ | never upscales; bounded decode; pure Go |
| Time vocabulary (Timestamp/TimeRange/Segment) | ✅ pure Go | ✅ | overlap/merge/split/shift range math |
| Spatial vocabulary (Resolution/FrameRate) | ✅ pure Go | ✅ | presets, aspect ratio, scale-to-fit/fill |
| Subtitle parse/serialize (SRT, WebVTT) | ✅ pure Go, fuzzed | ✅ | tag strip, entity decode, shift, range filter |
| High-quality resampling / filters | ➖ by design | ✅ | rskit `contrib/media/image` |
| Audio/video transcoding, ffmpeg | ➖ by design | ✅ | rskit `contrib/media/ffmpeg`/`audio` |
| Matrix/DSP, scene detection, waveforms | ➖ by design | ✅ | Rust is stronger for these; rskit-only |
| Codec/color/filter/pipeline/output executor vocabulary | ➖ backend-only | ✅ | dead surface without a transcoding executor; omitted intentionally |
