---
name: new-backend
description: "gokit: Add an opt-in, config-selected backend implementing the owning module's typed contract."
user-invocable: true
---

# Add a backend

Follow the [baseline](../../copilot-instructions.md) and the owner's existing typed registry/factory contract. Heavy adapters live in nested opt-in modules; core stays driver-independent. Preserve an existing local default only where the owner defines one—database selection is explicit.

1. Find the canonical owner, contract, callers, and sibling adapters. Use `new-module` for module wiring, including `contrib.go.work` and `domains.toml`.
2. Define validated typed configuration and factories. Capture provider configuration in factory closures, never opaque `any` parameters.
3. Implement the owner contract with bounded calls, cancellation, idempotent jittered retries, and typed failures preserving causes. Keep credentials in headers and resources explicitly owned.
4. Expose explicit caller-driven registration into an injected registry. No import-time setup or mutable globals.
5. Add focused package documentation and deterministic behavioral/failure tests using shared fixtures and clocks.
6. Use `validate` for the exact module selector and affected gates, including race/shuffle, tidiness, and dependency checks. Required adapter/integration acceptance fails when its service is unavailable; optional tests may skip only when clearly designated optional. Unit doubles never replace real backend proof.

Remove superseded callers and guidance with the change. Commit, push, or publish only when authorized.
