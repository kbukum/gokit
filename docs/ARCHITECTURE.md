# Architecture

Where code goes in gokit. The rules every change follows are in the [engineering standard](../.github/engineering.md). Why the original layering was chosen is in [ADR 0001](adr/0001-three-tier-layering.md).

## Module kinds

gokit is a multi-module repository split by dependency weight.

| Kind | Location | Holds |
|---|---|---|
| Root-module package | `<name>/` sharing the root `go.mod` | Foundation and cross-cutting packages with no heavy dependencies |
| Sub-module | `<name>/` with its own `go.mod` | A capability with heavy external dependencies (server, database, messaging, LLM…) |
| Nested adapter | `<parent>/<backend>/` with its own `go.mod` (`storage/s3`, `cache/redis`) | One opt-in backend and its SDK dependency |
| Test-support module | `<parent>/testutil/` or `testutil/` | Shared fakes, fixtures, and harnesses for that area |

- No heavy dependencies → root-module package. Heavy dependencies → sub-module. One vendor SDK → nested adapter under the capability it implements.
- A foundation concern buried in a sub-module, or a heavy dependency pulled into the root module, is misplaced.
- Each sub-module has a `replace` to its local parents for development, and joins `go.work` plus either `core.go.work` (capability modules) or `contrib.go.work` (nested adapters). `make replace-check` verifies the replace graph.
- `toven modules` lists every module. [MODULE-INDEX](MODULE-INDEX.md) is generated from it.

## Package layers

Imports point down. A package may import its own layer or lower ones, never higher, and never in a cycle.

| Layer | Packages |
|---|---|
| L0 | `errors`, `util`, `version` |
| L1 | `config`, `logging`, `validation`, `encryption`, `schema`, `codec`, `fs` |
| L2 | `component`, `hook`, `provider`, `di` |
| L3 | `observability`, `resilience`, `security` |
| L4 | `bootstrap`, `stream`, `chain`, `dag`, `worker`, `process`, `stateful` |
| L5 | `server`, `httpclient`, `grpc`, `connect`, `sse`, `discovery` |
| L6 | `auth`, `authz`, `database`, `cache`, `storage`, `vectorstore`, `messaging` |
| L7 | `ai`, `llm`, `embedding`, `inference`, `agent`, `tool`, `mcp`, `skill` |
| L8 | `media` |
| L9 | `workload`, `cli`, `bench`, `dataset`, `git` |

`ai` is the shared AI vocabulary; the other L7 modules build on it.

When a lower package needs something a higher package provides, it declares a small local interface and the higher type satisfies it. Wiring happens at composition.

Only some of these edges are gated. See [Enforcement](../.github/engineering.md#enforcement) and [Known gaps](../.github/engineering.md#known-gaps).

## Domains

Every module belongs to one Toven group in [`toven.toml`](../toven.toml). Groups are domains for running gates (`make check-<domain>`, `toven <task> --group <name>`, `toven groups`). Each group's `guardrails.allow_groups` limits which other groups its modules may depend on, and Toven fails any run whose direct module graph breaks it.

In short: `foundation` depends on nothing; `transport`, `composition`, `devtools`, and `media` build on `foundation`; `auth` adds `transport`; `data` and `ai` add `auth`; `infra` may use any of them. `toven.toml` is the source of truth.

Domain names are not layer names. The `foundation` group holds the whole root module, and the `composition` group holds only `stateful` and DAG test helpers.

## Composition

- **Providers.** `provider` defines `RequestResponse`, `Stream`, `Sink`, and `Duplex` with registry, manager, and selector. Cross-cutting behavior wraps them as `Middleware[I, O]` chains.
- **Components.** `component` owns `Start`/`Stop`/`Health` with deterministic ordering through a registry. `bootstrap` composes components into an application.
- **Adapters.** A capability package defines the contract and a lean in-memory or local default. Nested adapters implement it and register through an explicit `Register(registry)` call. Config selects the adapter.
- **Pipelines.** `stream` composes pull-based operators; `dag` runs providers in dependency order.

## Where to look next

- Which package owns a shared concern: [concern owners](concern-owners.md).
- Naming and file layout: [naming and structure](naming-and-structure.md).
- Test placement: [test layout](TEST-LAYOUT.md).
- Every package: [PACKAGES](PACKAGES.md) and [MODULE-INDEX](MODULE-INDEX.md).
