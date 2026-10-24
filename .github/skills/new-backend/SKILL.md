---
name: new-backend
description: "gokit: Add an opt-in, config-selected backend implementing the owning module's typed contract."
user-invocable: true
---

# Add a backend

A backend is a nested adapter module that implements an owner's typed contract. Placement: [Architecture](../../../docs/ARCHITECTURE.md#module-kinds). Rules: [Composition](../../engineering.md#composition), [Errors and resilience](../../engineering.md#errors-and-resilience), [Security and privacy](../../engineering.md#security-and-privacy).

1. Find the owner, its contract and registry, its callers, and sibling adapters. Keep the core package driver-independent; keep its local default only where the owner defines one (database selection is explicit).
2. Use [new-module](../new-module/SKILL.md) for module wiring: `contrib.go.work`, replaces, and the `toven.toml` domain group.
3. Define validated, typed config and a factory that captures it in a closure, never an opaque `any` parameter.
4. Implement the contract. Expose explicit `Register(registry)`; no `init()` or globals.
5. Add a declare-only `doc.go` and deterministic behavior and failure tests using shared fixtures and clocks.
6. Run [validate](../validate/SKILL.md) for the module and affected gates. Required [real adapter proof](../../engineering.md#real-adapter-proof) fails when its service is unavailable; only clearly optional tests may skip.

Remove superseded callers and guidance in the same change. Commit, push, or publish only when authorized.
