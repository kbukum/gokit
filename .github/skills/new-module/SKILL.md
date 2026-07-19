---
name: new-module
description: >-
    Scaffold a new package or module in the gokit multi-module monorepo the canonical way —
    decide root package vs sub-module, wire go.mod + replace directive, doc.go, domains.toml,
    the right go.work file, and the parity matrix. Use when adding a new capability, package, or
    module to gokit, or when unsure whether new code belongs in the root module or its own go.mod.
user-invocable: true
---

# Adding a package or module to gokit

gokit is a multi-module monorepo. Core packages share the root `go.mod` (`github.com/kbukum/gokit`); packages with heavy external dependencies get their own `go.mod` as sub-modules. Getting placement and wiring right up front avoids layering violations and pseudo-version breakage later.

## Step 1 — Decide: root package or sub-module

- **No heavy third-party deps** (stdlib + existing gokit only) → add a package **under the root module**. No new `go.mod`.
- **Heavy deps** (a cloud SDK, a driver, a broker client, an ML runtime) → create a **sub-module** with its own `go.mod` so consumers who don't need it don't pay for the dependency.

When in doubt, prefer the root module;
promote to a sub-module only when a real heavy dependency forces it.

## Step 2 — Pick the layer and confirm dependency direction

gokit layers depend **downward only** (enforced by `depguard` in `.golangci.yml`).
Consult `domains.toml` for the domain→module map and each domain's `depends_on`:

- core → patterns → crosscutting → composition → transport → auth → {data, ai} → media → infra

Your new package may only import lower or same-layer packages. A lower layer importing a higher one is a **blocker**. Transport (server/grpc/connect/sse) specifically must not import auth/authz — inject local interfaces instead.

## Step 3 — Create the package

Every package gets a `doc.go` with a package comment describing what it owns:

```go
// Package foo provides <one-line responsibility>.
// <2–3 lines on the model, invariants, and what it deliberately does not do>.
package foo
```

Conventions: package names are lowercase, single-word, no plurals.
Exported interfaces (1–3 methods) + factory functions; concrete implementations unexported.
Constructors take `...Option` (functional options).
No `interface{}`/`any` in public APIs except a documented opaque value. Organize by focused, concern-named files (types, options, registry, middleware, adapter) from the start — the `doc.go` aggregator stays docs-only, never a monolithic starter file. Before adding a shared helper, check [`docs/concern-owners.md`](../../../docs/concern-owners.md) so the new module does not re-own an existing concern.

## Step 4 — Sub-module wiring (only if you created a new go.mod)

```bash
cd <newmodule>
go mod init github.com/kbukum/gokit/<newmodule>
```

Then in the new `go.mod` add a `replace` back to the root for local dev (nested sub-modules use `../../`):

```
replace github.com/kbukum/gokit => ../
```

Register the module in the correct workspace file:

- Core-ish sub-modules (auth, database, media, …) → add `./<newmodule>` to **`core.go.work`**.
- Backend/contrib adapters (redis, s3, qdrant, kafka, …) → add to **`contrib.go.work`**.
- The aggregate `go.work` ties both together.

Then tidy: `toven tidy-fix --module go:<newmodule>` (see the `validate` skill).

## Step 5 — Register in domains.toml

Add the module name (directory name without any kit prefix, sub-modules as `parent/child`) to the correct `[domains.<domain>].modules` list
so the Makefile/CI `check-<domain>` gates and generated docs pick it up.

## Step 6 — Parity matrix

If this capability exists (or should be tracked) in rskit,
add/adjust its row in `docs/parity-matrix.md` (✅ present · ➖ absent · ⏳ planned) with a short note.
See the `parity` skill for the capability-not-blind mirroring policy.

## Step 7 — Validate

```bash
gofumpt -w <newmodule>
toven build --module go:<newmodule>
toven lint  --module go:<newmodule>
toven test  --module go:<newmodule> -- -race -count=1 -shuffle=on
toven tidy  --module go:<newmodule>
```

## Checklist

- [ ] Placement decided (root vs sub-module) and justified by real deps
- [ ] Layer confirmed; imports only go downward (depguard clean)
- [ ] `doc.go` present (docs-only); files split by concern; `make structure` green
- [ ] Public API typed/generic, options-based, no `any`
- [ ] (sub-module) `go.mod` + `replace`, added to the right `*.go.work`
- [ ] `domains.toml` updated
- [ ] `docs/parity-matrix.md` updated if it has a cross-kit counterpart
- [ ] build/lint/test/tidy green for the module

Per repo workflow, **create the branch and make edits only** — the maintainer commits and pushes.
