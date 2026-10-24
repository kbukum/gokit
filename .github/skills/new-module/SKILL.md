---
name: new-module
description: "gokit: Add a Go package or module with layer, workspace, documentation, and test wiring."
user-invocable: true
---

# Add a package or module

Placement: [Architecture](../../../docs/ARCHITECTURE.md). Rules: [Placement and reuse](../../engineering.md#placement-and-reuse), [Public API and stability](../../engineering.md#public-api-and-stability), [Code style and structure](../../engineering.md#code-style-and-structure).

1. Find the [concern owner](../../../docs/concern-owners.md) first. Prefer a root-module package for stdlib or existing-kit dependencies; put genuinely heavy dependencies in a sub-module or nested adapter.
2. Pick the layer and domain (`toven groups`). Every import must point down the [layer table](../../../docs/ARCHITECTURE.md#package-layers); inject a lower contract instead of importing upward.
3. Create concern-named files and a declare-only `doc.go`.
4. For a new module: set its import path and the root `go` directive, run `make replace-sync`, and add it to `go.work` and `core.go.work` or `contrib.go.work`. Nested paths need the right replace depth.
5. Add it to its domain group in `toven.toml` (any Toven run checks `allow_groups`), run `make module-index`, and wire direct callers. Track sibling capabilities through [parity](../parity/SKILL.md).
6. Implement test first, with shared helpers and real integration proof where required.
7. Run [validate](../validate/SKILL.md) for the module: format, build, vet, lint, race + shuffle tests, `make structure`, and `make tidy-check`. Use `make tidy` only when manifests change.

Commit, push, open PRs, or publish only when authorized.
