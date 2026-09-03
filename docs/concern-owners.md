# Concern owners

The canonical **concern → owning module** map for gokit. Before adding any shared helper, type, or capability, find the concern below and **reuse or extend the named owner** — do not fork a local copy. If the owner is inadequate, enhance it *generically* (so every consumer benefits), never caller-specifically. Reimplementing a concern that already has an owner is a review blocker.

This map names *who* owns each concern; the *how to judge* procedure (reuse / enhance / add / justify) lives in the review pass [`.github/skills/review/references/01-canonical-reuse.md`](../.github/skills/review/references/01-canonical-reuse.md). Start here, then check each low-level operation against that pass.

| Concern | Owner | Reuse this, not | Notes |
|---|---|---|---|
| Data formats (JSON/TOML/…) | `codec` | hand-rolled `encoding/json` / `BurntSushi/toml` wrappers, per-package marshal helpers | `codec/json.go`, `codec/toml.go`, `codec/framing`, `codec/value` |
| Generic helpers (slices/maps/clock/copy/casing/env/hashing/templates) | `util` + modern stdlib (`slices`/`maps`/`cmp`) | a fresh local helper, `sort.Slice` where `slices.SortFunc` fits | scoped foundation owner, not a dumping ground |
| Filesystem / path safety / atomic writes | `fs` | raw `os` + `filepath.EvalSymlinks` + `Rel` escape checks, non-atomic `os.WriteFile` | path confinement, symlink-escape rejection, atomic writes |
| Config loading / precedence | `config` | custom env/flag/file precedence logic | |
| JSON Schema validation | `schema` | hand-rolled validation walks | |
| Errors | `errors` | fresh sentinels / custom error structs for shared concerns | `AppError`, RFC 9457, typed codes, `errors.Is/As/Join` |
| Logging | `logging` | `log`, `fmt.Print*` | `log/slog` via injected logger |
| Resilience (retry/timeout/circuit-break) | `resilience` | hand-rolled loops, scattered `context.WithTimeout` + custom backoff | idempotent ops only, bounded + jittered |
| HTTP client / server | `httpclient` / `server` | raw `http.Client{}` with custom retry/timeout | |
| Subprocess | `process` | bare `exec.Command` | argv-only, no shell |
| Dependency injection | `di` | service-locator / string-keyed resolution | typed resolution |
| Observability (traces/metrics) | `observability` | direct exporter wiring, package-global meters | injected tracer/meter |
| Encryption / secrets | `encryption` / `security` | ad-hoc crypto, custom header sets | current algorithms only; non-crypto secret redaction/masking lives in `util` (`SecretString`, `SecretKeyMatcher`) |
| Git operations | `git` | bare `exec.Command("git", …)` | |
| Validation | `validation` | inline boundary checks duplicated per package | |
| Token/identity validation | `auth` (`TokenValidator`) | a transport owning token validation or defining divergent per-transport validation semantics; reading credentials from the URL query string as the default | injected into transports (server/grpc/connect/sse) via a local structural interface — L5 never imports `auth` (L6); 401 missing/invalid vs 403 not-permitted. The identical structural injection seam repeated per transport is the approved layering exception, not a divergent contract. `grpc`/`connect`/`sse` are header-only; `server/middleware` adds an opt-in, path-whitelisted, audited query-token fallback (`WithQueryTokenParam`) for endpoints that cannot set headers |
| Database persistence / GORM drivers | `database` (+ adapter sub-modules) | hand-wiring `gorm.io/driver/*` + a `golang-migrate` driver per consumer | `DialectRegistry`, repositories, `migration`; drivers live in adapters (`database/sqlite`, `database/postgres`), registered explicitly, no `init()` |
| Eval run provenance / reproducibility | `bench` | ad-hoc seed/commit/host capture on benchmark results | `RunProvenance`, injected `ProvenanceProbe`, seeded `math/rand/v2`, order-independent dataset hash |
| Tokenization / token counting | `llm` | per-package `len(text)/4` estimates, ad-hoc BPE/HF wrappers | `TokenCounter` port + `HeuristicTokenCounter` default; real tokenizers as contrib sub-modules (`llm/tokenizer/tiktoken`, `llm/tokenizer/huggingface`); heuristic shares `ai/chat.ApproxTokens` |

## How to use this map

1. Name the concern before writing the code.
2. Find its owner above; **consume or implement its contract** (see the [reuse review pass](../.github/skills/review/references/01-canonical-reuse.md)).
3. If the owner is close but inadequate, enhance it generically, then consume — never fork.
4. If a concern has genuinely no owner and is foundational, add it to the correct owning module (or a new correctly-layered one), with tests and docs — not locally.

The list is illustrative, not exhaustive: **any** gokit module is a potential owner, so a capability that maps to an owner not named here still counts.
