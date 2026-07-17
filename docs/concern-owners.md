# Concern owners

The canonical **concern → owning module** map for gokit. Before adding any shared helper,
type, or capability, find the concern below and **reuse or extend the named owner** — do not
fork a local copy. If the owner is inadequate, enhance it *generically* (so every consumer
benefits), never caller-specifically. Reimplementing a concern that already has an owner is a
review blocker.

This map names *who* owns each concern; the *how to judge* procedure (reuse / enhance / add /
justify) lives in the review pass
[`.github/skills/review/references/01-canonical-reuse.md`](../.github/skills/review/references/01-canonical-reuse.md).
Start here, then reconcile each low-level operation against that pass.

| Concern | Owner | Reuse this, not | Notes |
|---|---|---|---|
| Data formats (JSON/TOML/…) | `codec` | hand-rolled `encoding/json` / `BurntSushi/toml` wrappers, per-package marshal helpers | `codec/json.go`, `codec/toml.go`, `codec/framing`, `codec/value` |
| Generic helpers (slices/maps/clock/copy/ensure-dir) | `util` + modern stdlib (`slices`/`maps`/`cmp`) | a fresh local helper, `sort.Slice` where `slices.SortFunc` fits | scoped foundation owner, not a dumping ground |
| Filesystem / path safety / atomic writes | `fs` | raw `os` + `filepath.EvalSymlinks` + `Rel` escape checks, non-atomic `os.WriteFile` | path confinement, symlink-escape rejection, atomic writes |
| Config loading / precedence | `config` | bespoke env/flag/file precedence logic | |
| JSON Schema validation | `schema` | hand-rolled validation walks | |
| Errors | `errors` | fresh sentinels / bespoke error structs for shared concerns | `AppError`, RFC 9457, typed codes, `errors.Is/As/Join` |
| Logging | `logging` | `log`, `fmt.Print*` | `log/slog` via injected logger |
| Resilience (retry/timeout/circuit-break) | `resilience` | hand-rolled loops, scattered `context.WithTimeout` + bespoke backoff | idempotent ops only, bounded + jittered |
| HTTP client / server | `httpclient` / `server` | raw `http.Client{}` with bespoke retry/timeout | |
| Subprocess | `process` | bare `exec.Command` | argv-only, no shell |
| Dependency injection | `di` | service-locator / string-keyed resolution | typed resolution |
| Observability (traces/metrics) | `observability` | direct exporter wiring, package-global meters | injected tracer/meter |
| Encryption / secrets | `encryption` / `security` | ad-hoc crypto, custom header sets | current algorithms only |
| Git operations | `git` | bare `exec.Command("git", …)` | |
| Validation | `validation` | inline boundary checks duplicated per package | |

## How to use this map

1. Name the concern before writing the code.
2. Find its owner above; **consume or implement its contract** (see the
   [reuse review pass](../.github/skills/review/references/01-canonical-reuse.md)).
3. If the owner is close but inadequate, enhance it generically, then consume — never fork.
4. If a concern has genuinely no owner and is foundational, add it to the correct owning
   module (or a new correctly-layered one), with tests and docs — not locally.

The list is illustrative, not exhaustive: **any** gokit module is a potential owner, so a
capability that maps to an owner not named here still counts.
