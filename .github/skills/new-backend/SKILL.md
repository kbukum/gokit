---
name: new-backend
description: >-
    Add a pluggable backend/adapter (storage, vectorstore, messaging, cache, llm, inference, …)
    to gokit the canonical way — a nested contrib sub-module with an explicit typed
    Register(registry, cfg) factory, no init() side effects, and the in-memory/local default kept
    in core. Use when integrating a provider like S3, GCS, Qdrant, Kafka, NATS, Redis, or a new
    LLM/inference provider into an existing gokit registry.
user-invocable: true
---

# Adding a backend adapter to gokit

gokit's data/ai layers use a registry + factory pattern so a core module ships an in-memory or local default and heavy provider backends live in opt-in **contrib sub-modules**. Follow the existing owners exactly — do not invent a new registration mechanism.

## The binding rules

1. **Nested contrib sub-module.** The adapter lives under its owner as its own `go.mod` (`storage/s3`, `storage/gcs`, `vectorstore/qdrant`, `messaging/kafka`, `cache/redis`, `llm/providers`, `inference/vllm`, …). It carries the heavy SDK dependency so core stays light.
2. **Explicit typed `Register`.** Each backend exposes a typed `Register(registry, cfg Config)` (or `RegisterX(registry, cfg)`) that captures provider config in the **factory closure**. Factories are typed funcs — `func(cfg, log) (T, error)` / `func(cfg) (T, error)` — **never** a `providerCfg any` / `adapterCfg any` parameter.
3. **No `init()` side effects, no mutable package globals.** Registration is explicit and caller-driven; the registry is injected, not a global. (See the composition principle in `.github/copilot-instructions.md`.)
4. **Core keeps the default.** The in-memory / local backend stays in the core module and remains the zero-config default; contrib backends are selected via config.

Reference owners to copy from: `storage/factory.go` (`StorageFactory`, typed `Register`), `vectorstore/factory.go` (`Factory`, `RegisterMemory`), `messaging/registry.go` (typed `ProducerFactory`/`ConsumerFactory`), and the existing backends `storage/gcs/gcs.go`, `vectorstore/qdrant/qdrant.go`.

## Steps

1. **Create the nested sub-module** (see the `new-module` skill for full go.mod wiring):

   ```bash
   cd <owner>/<backend>            # e.g. storage/s3
   go mod init github.com/kbukum/gokit/<owner>/<backend>
   # replace github.com/kbukum/gokit => ../../   (nested → two levels up)
   ```

Add `./<owner>/<backend>` to **`contrib.go.work`** and the module to the owner's domain in `domains.toml`.

2. **Define a typed `Config`** for the backend (endpoint, credentials source, timeouts, bucket/ topic names). No `any`. Validate it at construction — this is a trust boundary.

3. **Implement the adapter** against the owner's interface. Timeout every remote call via `context.Context`; bounded jittered retries for idempotent ops only; degrade/circuit-break rather than success-shaped fallbacks. Tokens go in headers, not query strings. Split code by concern into focused files (config, client, adapter, mapping).

4. **Expose `Register`** that closes over the config and installs the typed factory into the passed registry. No global registry, no `init()`.

   ```go
   // Register installs the <backend> factory into the registry using cfg.
   func Register(reg *Registry, cfg Config) { /* reg.Add(name, func(...) { ... cfg ... }) */ }
   ```

5. **`doc.go`** describing the backend, its config, and its failure modes.

6. **Tests** — behavioral, deterministic, injected clock (never `time.Sleep`), cover failure paths; fixtures over embedded config; green under `-race -shuffle`. Integration tests that need a live broker/store are gated/skipped without it.

## Validate

```bash
gofumpt -w <owner>/<backend>
toven build --module go:<owner>-<backend>       # selectors use '-', e.g. go:storage-s3
toven lint  --module go:<owner>-<backend>
toven test  --module go:<owner>-<backend> -- -race -count=1 -shuffle=on
toven tidy  --module go:<owner>-<backend>
```

(Confirm the exact selector name with `toven modules`.)

## Checklist

- [ ] Nested contrib sub-module with own `go.mod` + `replace ../../`, added to `contrib.go.work`
- [ ] `domains.toml` updated under the owner's domain
- [ ] Typed `Config`, validated at construction; no `any` in the factory
- [ ] Explicit `Register(registry, cfg)`; no `init()`, no mutable package-global registry
- [ ] Core in-memory/local default untouched and still the zero-config path
- [ ] Context timeouts, bounded retries (idempotent only), no success-shaped fallbacks
- [ ] `doc.go` + behavioral tests green under race/shuffle

Per repo workflow, **create the branch and make edits only** — the maintainer commits and pushes.
