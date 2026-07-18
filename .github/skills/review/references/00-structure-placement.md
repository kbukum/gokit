# Pass 00 — Structure and placement

Confirm every touched (or, in project mode, every existing) item lives in the right module, package,
and layer, and that the dependency direction stays acyclic. This is the first gate:
misplaced code makes every later pass moot, so reject on failure here before going further.

> **Run in a separate, clean-context agent** — never inline in the session that wrote the code.
> An independent reviewer re-derives every judgment from the code
> and the principles instead of trusting prior reasoning.
> A plan/spec may be passed in as a scope checklist only; it never excuses a baseline violation.

**Scope note.** *Changes mode:* check the packages the diff touches plus the affected area —
a change to a core package's public surface fans out to every sub-module, nested adapter,
and downstream consumer. *Project mode:* sweep each module's packages and dependency edges;
the placement and acyclicity rules below are invariants for the whole toolkit.

## The layering invariant

Dependency direction is explicit and acyclic; lower layers never import higher. A cycle
or an upward import is a **blocker**, enforced by `depguard` in `.golangci.yml`.
The module layering (downward dep direction only):

```
L0  errors, util, version               L5  server, httpclient, grpc, sse, discovery, connect
L1  config, logger, validation,         L6  auth*, authz, database, cache, storage,
    encryption, schema                      vectorstore, messaging
L2  component, hook, provider, di       L7  llm, embedding, agent, tool, mcp, inference, ai, skill
L3  observability, resilience, security L8  media
L4  bootstrap, pipeline, chain, dag,    L9  workload, cli, bench, dataset
    worker, process, stateful
```

## Module/package placement

gokit is a multi-module monorepo split by dependency weight and role:

| Kind | Location | Owns |
|------|----------|------|
| Root-module package | `github.com/kbukum/gokit/<name>` (shared root `go.mod`) | foundation + cross-cutting packages with no heavy deps |
| Sub-module | `<name>/` with its own `go.mod` | packages with heavy external deps (auth, database, messaging, server, …) |
| Nested adapter | `<parent>/<backend>/` (e.g. `storage/s3`, `cache/redis`, `messaging/kafka`) | an opt-in backend that owns its SDK dependency |
| Test util | `<parent>/testutil/` | test helpers for that area |

## Checks

- **Package placement.** No-heavy-deps foundation code → root module.
  Heavy-deps code → sub-module with its own `go.mod`.
  A backend adapter → nested `<parent>/<backend>/` owning its SDK.
  A foundation concern buried in a sub-module, or a heavy-dep import pulled into the root module,
  is a structure violation (blocker).
- **Acyclic, downward-only edges.** No lower-layer package imports a higher one; no cycle.
  This is gated by `depguard` — run `make lint`. An upward import is a blocker.
- **New sub-module wiring.** Own `go.mod`,
  a `replace github.com/kbukum/gokit => ../` directive for local dev,
  added to `go.work` / `core.go.work` / `contrib.go.work` as appropriate,
  and to `domains.toml` + the matching `make check-<domain>` set. Missing any is a should-fix.
- **doc.go present.** Every package has a `doc.go` with a package doc comment.
  Missing is a should-fix.
- **Declare-only aggregator.** `doc.go` carries package documentation only —
  no `func`/`type`/`var`/`const`. Code in a `doc.go`,
  or a package whose logic is piled into one oversized file instead of concern-named siblings,
  is a should-fix. Run `scripts/check-structure.sh` (`make structure`).
- **No misplaced concerns.** Each cross-cutting concern stays in its canonical package —
  e.g. gRPC status mapping belongs in `grpc`, not `errors`. (Reuse of those owners is pass `01`.)
- **Backend opt-in.** A nested adapter registers via an explicit `Register(registry)` call,
  not an `init()` side effect, and the core package keeps a lean in-memory/local default.

## Detection starters

These flag candidates, not verdicts — read each hit to judge intent.

```bash
# what each module actually depends on (intra-gokit edges)
for m in */go.mod; do echo "== $m =="; grep -E '^\s+github.com/kbukum/gokit' "$m"; done
# packages missing a doc.go
for d in $(find . -name '*.go' -not -name '*_test.go' | xargs -n1 dirname | sort -u); do \
  ls "$d"/doc.go >/dev/null 2>&1 || echo "no doc.go: $d"; done
# init() functions (import-time side effects — should be explicit Register)
grep -rn --include=*.go '^func init()' . | grep -v _test.go
# declare-only aggregator guard (doc.go docs-only + god-file advisory)
scripts/check-structure.sh
```

Then run `make lint` (depguard enforces the layer direction)
and `make check-<domain>` for the touched domain.
