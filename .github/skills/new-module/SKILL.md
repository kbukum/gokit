---
name: new-module
description: "gokit: Add a Go package or module with layer, workspace, documentation, and test wiring."
user-invocable: true
---

# Add a package or module

Follow the [baseline](../../copilot-instructions.md) and [module structure](../../engineering.md#module-structure).

1. Find the [concern owner](../../../docs/concern-owners.md) before creating shared behavior. Prefer a root package for stdlib/existing-kit dependencies; isolate genuinely heavy dependencies in a nested module.
2. Choose the domain from `domains.toml` and confirm depguard permits every import. Transport does not import auth/authz; inject a lower contract instead.
3. Create focused files and a documentation-only `doc.go`. Use small typed interfaces, cohesive options, cause-preserving errors, and documented genuinely opaque values; no runtime panics, globals, or import-time I/O.
4. For a new module, initialize its correct import path, add local replacements, and register it in `core.go.work` or `contrib.go.work` as appropriate. Nested paths need correctly resolved replacement depths.
5. Add the module to its domain and wire directly dependent callers. Track relevant sibling capabilities through `parity`, without a cross-kit runtime dependency.
6. Implement test-first with shared helpers, deterministic failures, bounded ownership, and real integration proof where required.
7. Use `validate` for the discovered selector: formatting, build, lint, vet, race/shuffle tests, structure, and read-only tidiness. Use the existing mutating tidy task only when manifest changes require it.

Commit, push, open PRs, or publish only when authorized.
