# Test layout

The single source of truth for **where tests live** in gokit and **how they are named**. It describes gokit as it already is, so the convention can be applied consistently as packages are hardened. This is the Go-idiomatic sibling of rskit's `docs/TEST-LAYOUT.md`: the same three tiers, expressed the Go way.

The reuse rule for shared test tooling — fakes, clocks, harnesses, assertions belong in the owning `testutil`, never inline — is governed together with the production concern owners in [`concern-owners.md`](concern-owners.md). Naming and file-organization rules for source live in [`naming-and-structure.md`](naming-and-structure.md); this document covers test placement only.

## The three tiers

gokit tests fall into three tiers by scope and visibility. There is **no mandate of one test file per source file** and no coverage-padding split — a tier is chosen by what the test proves, not to hit a number.

### Tier 1 — unit tests (white-box, co-located)

In-package tests that exercise a single concern, including unexported behavior. They live in a `foo_test.go` file **in the same package** (`package foo`), beside the `foo.go` they cover.

- File naming: `foo.go` → `foo_test.go`. One test file per source concern; do **not** add `_edge`, `_extra`, `_more`, `_additional`, or `_coverage` suffixes to pad coverage — extend the existing `foo_test.go` instead. A genuinely distinct concern gets its own concern-named source file (and therefore its own `_test.go`).
- Table-driven, `t.Parallel()`, deterministic: inject clocks (never `time.Sleep`), use `t.TempDir()`/fakes (no real network or filesystem), `t.Setenv` for env isolation. Green under `-race -shuffle=on -count=1`.
- `Example…` functions that double as documentation live alongside as `example_test.go`.

### Tier 2 — module tests (black-box, public API)

Tests that pin the package's **public contract** through its exported surface only. They live in the same directory in a separate `package foo_test`, in a `foo_test.go` (or a `<concern>_test.go`) file. Use black-box when the test should be insulated from internals, or to break an import cycle between the package under test and a `testutil` helper that imports it.

- Same naming and determinism rules as tier 1.
- Prefer black-box for anything a downstream consumer could also write, so the test guards the contract, not the implementation.

### Tier 3 — integration tests (adapter sub-modules, live backends)

Tests that drive a real backend or cross-module wiring. In gokit these live in the **adapter sub-module** that owns the SDK dependency (e.g. `messaging/kafka`, `storage/s3`, `database/sqlite`), not in the backend-agnostic core, and are named `<concern>_integration_test.go`.

- A test that needs a live service (broker, container, network) is guarded so the default `go test ./...` stays hermetic — behind a build tag or an early skip when the service/env is absent — never failing a developer who lacks the backend.
- The adapter sub-module's integration tests are what exercise the backend-agnostic **core** package. Core coverage is attributed back across module boundaries via `-coverpkg`: each module lists the packages to attribute in a `.coverpkg` file, which CI feeds to `go test -coverpkg=…`. A core package reading low in a plain per-module sweep is usually covered from its adapter — **re-measure the CI way before assuming a gap** (see the coverage caveat in `tmp/tdd-hardening/README.md`).

## Where shared test tooling lives

Fakes, clocks, spies/recorders, setup harnesses, and assertions are a **shipped product**, not throwaway scaffolding. They live in the owning `testutil` package — the root `testutil/` for cross-cutting component/lifecycle helpers, or the area's `<parent>/testutil/` (e.g. `messaging/testutil`, `database/testutil`, `git/testutil`) — and are reused across tiers.

- Never hand-roll a one-off fake inside a `_test.go` when a shared helper exists or the fake should live in a `testutil`. When a test needs a new fake/harness, **add or extend it in the owning `testutil` and reuse it** (see the reuse dimension in `tmp/tdd-hardening/README.md`).
- Each `testutil` keeps a declare-only `doc.go` and, where user-facing, a short README showing intended reuse. Helper naming is consistent across modules (see `naming-and-structure.md`).

## Non-goals

- No one-test-file-per-source-file mandate; no `_edge`/`_extra`/`_coverage` coverage-padding files.
- No forced black-box split — choose the tier by what the test proves.
- This document does not restate production naming/structure rules; those live in [`naming-and-structure.md`](naming-and-structure.md).

## Enforcement

Test placement is a **review pass**, not a hard gate — see `.github/skills/review/references/00-structure-placement.md` (Test placement). The advisory `make structure` (`scripts/check-structure.sh`) reports coverage-padding test-file names alongside the declare-only-aggregator and god-file/crowded-package advisories. Nothing here blocks CI; the review pass and reviewer judgment carry it, matching rskit's advisory approach.
