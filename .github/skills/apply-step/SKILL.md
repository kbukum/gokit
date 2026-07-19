---
name: apply-step
description: >-
    Apply a single step of a tmp/ plan — read the plan README and all previous steps for
    accumulated context and decisions, then implement the current step test-first against
    gokit's engineering baseline, validate the affected modules, and mark the step done. Use to
    execute one specific plan step, or as the per-step unit that apply-plan drives.
user-invocable: true
---

# Applying one plan step, in context

`apply-step` implements exactly one step of a plan folder (from the `create-plan` skill).
It is the unit of work that `apply-plan` calls per step,
and it can also be run directly on a single step file.

## Input

A path to one step file, e.g. `tmp/storage-gcs-backend/02-registry.md`.

## 1. Load full context before editing

A step is not self-contained — earlier steps make naming, layering,
and API decisions this step depends on. Read, in order:

1. **`README.md`** of the plan folder — goal, dependency order,
   and the cross-cutting baseline rules that bind every step.
2. **Every previous step** (`NN-*.md` with a lower number) — for the decisions
   and files they already established. Honor them; do not re-litigate
   or contradict a completed step.
3. **The current step** — its scope, numbered actions, files touched, and acceptance criteria.

Confirm the current step's *Depends on* steps are `done` before starting.
If a dependency is unfinished, stop and say so.

## 2. Implement the step against the baseline

Apply the current step's actions **test-first**, honoring gokit's engineering baseline —
the plan does not override it,
and the authority is [`../../copilot-instructions.md`](../../copilot-instructions.md):

- **TDD.** For each behavior: failing test → minimal code → refactor while green,
  failure paths included. Never write the production code first and add tests after.
- **Placement & layering.** Right module (root vs sub-module vs nested adapter);
  acyclic imports (lower layers never import higher); new packages get `doc.go`.
- **Canonical reuse.** Before writing a new type/helper, open [`docs/concern-owners.md`](../../../docs/concern-owners.md), find the concern's owner, and reuse or extend it — never duplicate a shared concern (errors, config, logging, path safety, retries, HTTP, registries). Put new logic in concern-named files; never in `doc.go`.
- **Typed & minimal.** Generics-first; no public `interface{}`/`any` (documented opaque exceptions only); typed errors preserving cause; timeout + cancellation on remote calls.
- **Root-cause, no shims.** Redesign cleanly; remove the old path completely (pre-stable, no back-compat).
- **Readable files & injected composition.** Split by concern into focused files; when you touch a package that has piled its non-test code into one oversized or concern-mixed file, split it into concern-named siblings (`client.go`, `options.go`, `types.go`, `doc.go`) **in this step** — do not defer the reorg. Structure work travels with the change; only split where it genuinely improves discoverability, and leave cohesive single-concern files alone. Inject registries/config; no `init()` side effects or mutable package globals.

Keep the edit scoped to *this* step's `Files touched`; if you discover the step is mis-scoped, report it rather than silently expanding.

## 3. Validate, review, and mark done

- **Validate** the affected modules with the `validate` skill (toven, scoped),
  green under `-race -shuffle`. A step does not land red.
  Run `make structure` (declare-only aggregator guard) before marking the step done.
- **Review** the step's diff with the relevant `review` passes (structure/placement, canonical reuse, principles, security, quality, tests, docs, comments)
  — ideally in a fresh agent.
- Only when acceptance criteria are genuinely met, flip the step's progress signal
  so `apply-plan` can resume: set `**Status:** done` and check its `- [x]` boxes.
  Do not mark a step done on a partial or red result.

## Repo workflow

Work on a branch (`create-branch` skill), leave edits **uncommitted** for the maintainer to commit
and push; open a PR only when explicitly asked. When that change becomes a branch/PR,
name it by the change — never `step-2` or a plan/batch number.
