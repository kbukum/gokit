# Naming and structure

The single source of truth for **naming** and **file/folder organization** of gokit source. It is the companion to [`concern-owners.md`](concern-owners.md) (which owns *where a concern lives*), [`ARCHITECTURE.md`](ARCHITECTURE.md) (which owns *layers and module kinds*), and [`TEST-LAYOUT.md`](TEST-LAYOUT.md) (which owns *where tests live*). The rules every change follows are in the [engineering standard](../.github/engineering.md); this doc holds the naming and organization detail.

## Naming

- **Package names** are lowercase, single-word, singular, and non-stuttering — `cache`, not `caches` or `cacheutil`; the package name never stutters with its import path (avoid `http.HTTPClient`). A misleading or stuttering name is a defect: rename it and migrate callers in the same change.
- **Exported identifiers read correctly at the call site.** Prefer `cache.New` over `cache.NewCache`; `messaging.Broker` over `messaging.MessagingBroker`. Drop the package name from the identifier when the qualified form already carries it.
- **Files are named after the concern they hold** — `client.go`, `options.go`, `registry.go`, `middleware.go`, `types.go`, `errors.go`. A file name describes its contents; catch-all names (`util.go`, `misc.go`, `helpers.go`) are a smell unless the package's concern genuinely is that.
- **Test files** follow `foo.go` → `foo_test.go`; no `_edge`/`_extra`/`_more`/`_coverage` suffixes (see [`TEST-LAYOUT.md`](TEST-LAYOUT.md)).
- **Test-helper naming** in `testutil` packages is consistent across modules for equivalent shapes: constructor fakes as `New<Thing>Fake`, recording/spy helpers as `Recording<Thing>`, one-call setup as `Setup`, and clock/dependency injection as functional options `With<Dep>`. An outlier form (`Fake<Thing>` constructor, `Spy<Thing>`, bespoke setup name) is renamed to the canonical shape and callers migrated.

## File and folder organization

Organize by focused, well-named files within a package; never pile unrelated concerns into one file.

- **One concern per file.** Split a package's non-test code by concern (types, options, registry, middleware, adapter, client) into concern-named sibling files so the next reader navigates by filename.
- **`doc.go` is declare-only.** It carries the package clause and package documentation comment only — no `func`/`type`/`var`/`const` declarations and no imports. Code in a `doc.go` is a structure violation; move it to a concern-named sibling. Reported (advisory) by `scripts/sg-rules/declare-only-aggregator.yml` via `make structure`.
- **Oversized-file signal.** A single non-test `.go` file past roughly **300–400 code lines** (code only — excluding `_test.go`, comments, blanks) is a prompt to check whether distinct concerns are piled together. Length alone is never the verdict: a cohesive single-concern file is fine at any size; **concern-mixing** is the real signal. When a change touches such a file, split it in the same change rather than postponing. Surfaced (advisory) by `make structure` (`GOD_FILE_LINES`, default 350).
- **Sub-package lift.** When a single package/folder accumulates **more than ~10 non-test files** (`CROWDED_PKG_FILES`) that fall into **2–3+ separable concern groups**, lift each cohesive group into its own concern-named sub-package (sub-folder with its own declare-only `doc.go`) — as in `agent/{memory}`, `cli/{theme,render}`, `dataset/{payload,record,stage}`. This is criteria-driven, not a file count: only split where the groups are genuinely separable and it improves readability, maintainability, and extendability without causing other issues (import cycles, over-fragmentation).
- **Placement and layering** (which module/package/layer code belongs in, acyclic downward dependency direction) are covered by [`ARCHITECTURE.md`](ARCHITECTURE.md) and [`concern-owners.md`](concern-owners.md); they are not restated here.

## Enforcement

These rules are **advisory**. Review checks them through the [placement and ownership lens](../.github/skills/review/references/placement-ownership.md), and `make structure` (`scripts/check-structure.sh`) reports declare-only `doc.go`, large-file, crowded-package, and coverage-padding test-file names without failing CI. This matches rskit's advisory `declare-only-aggregator` approach. A layer-hardening step fixes the violations in its scope against this convention.
