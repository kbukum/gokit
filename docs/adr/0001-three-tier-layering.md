# 0001. Three-tier package layering

- Status: Accepted
- Date: 2026-04-25
- Authors: @kbukum

## Context

The OSS engineering review surfaced a layering inversion (worker → sse, F-012)
and noted that nothing prevented similar regressions (F-014).
We need a stable rule that engineers can apply without case-by-case debate.

## Decision

We will organize gokit packages into three tiers:

1. **Foundation** — transport-agnostic primitives. May depend on stdlib
   and other foundation packages only. Examples: `worker`, `errors`, `logger`, `validation`, `hook`,
   `util`, `version`, `encryption`, `schema`, `resilience`, `httpclient`.

2. **Transport** — protocol-specific adapters. May depend on foundation packages.
   Must NOT be imported from foundation. Examples: `sse`, `server`, `grpc`, `connect`.

3. **Integration** — composes foundation + transport into runnable units. May depend on either.
   Examples: `bootstrap`, `component`, `agent`, `llm`, `mcp`, `dag`, `pipeline`, `chain`.

When foundation needs a service that lives in transport (e.g. SSE broadcasting),
foundation declares a **local interface** for the operation it needs.
Transport types satisfy it implicitly via Go's structural interface satisfaction.

The `depguard` linter enforces tier-1 boundaries (`.golangci.yml`).

## Consequences

- New foundation packages must avoid transport imports. The lint rule fails CI if violated.
- Cross-tier wiring lives in integration packages;
  foundation/transport remain independently testable and reusable.
- A small upfront cost:
  foundation packages duplicate single-method interfaces (e.g. `worker.Broadcaster`) instead of importing `sse.Broadcaster`.
  This is a deliberate tradeoff for layering isolation.

## Alternatives considered

- **Single-tier (status quo)** — relied on convention; review showed it doesn't hold.
- **Move bridges to transport package** — just inverts the edge;
  same problem in the other direction.
- **Two tiers (foundation / everything else)** — collapses the distinction between protocol adapters
  and composers; gives no useful rule for where to put new code.
