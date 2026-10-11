# Review and apply fixes

Use only when fixing is requested. Follow [review](../SKILL.md) and the [baseline](../../../copilot-instructions.md). Preserve edits/index; review directly unless the user requests an independent agent.

## Scope

Inspect Git state and the requested diff or tree. Include touched files, close callers/callees, and affected contracts. Flag workspace/domain/depguard changes, shared errors, and public surfaces that affect adapters or consumers. Plans and PR text define scope, not exceptions.

## Passes

Use [validate](../../validate/SKILL.md) for current commands and selectors. Run applicable mechanical checks without mutating formatting or tidiness during a read-only review. Load the detailed checklist only when its trigger applies.

| Pass | Trigger and checks | Detail |
|---|---|---|
| Placement/reuse | Package, module, ownership, or shared-helper changes; correct layers and canonical owners | [00](00-structure-placement.md), [01](01-canonical-reuse.md) |
| Correctness | Go behavior; typed causes, guarded assertions, no runtime panic/ignored error/success fallback; cleanup on all paths; context first and propagated | [02](02-principles.md), [04](04-quality.md) |
| Concurrency | Goroutines, channels, locks; owned cancellation/drain, bounded queues, race safety, no lock across blocking work, injected timing | [02](02-principles.md), [05](05-tests-tdd.md) |
| Composition | Registries, components, factories; injected policy, explicit registration, lifecycle order, no `init()` I/O or mutable globals | [00](00-structure-placement.md), [02](02-principles.md) |
| Security/config | External input, auth, paths, crypto, configuration; validation and read limits, parameterized SQL, argv-only processes, secure cookies/header credentials, JWT algorithm/claim checks, redaction | [03](03-security-privacy.md) |
| API/dependencies | Exported items or manifests; minimal typed APIs, documented opaque values, options, maintained dependencies, correct Go floor, workspace/domain wiring, read-only tidy checks | [01](01-canonical-reuse.md), [04](04-quality.md) |
| Tests/docs | Every scope; behavioral/failure proof, executable examples, current godoc, accurate claims | [05](05-tests-tdd.md), [06](06-docs-supply-chain.md), [07](07-comments-godoc.md) |

Tests use shared helpers, injected clocks, safe environment isolation, and temporary directories. Require race/shuffle and coverage: >=80% per package, >=85% overall and for security-load-bearing owners. Parser/validator/auth/codec/schema changes need fuzz targets. Unit doubles do not replace required real integration proof.

## Findings and acceptance

Report `severity — file:line — finding — principle — fix`, grouping related patterns. A principle violation, incorrect behavior, or broken contract blocks acceptance. Distinguish skipped passes from unrun checks and explain gaps.

For authorized fixes, choose Redesign/Align/Enhance/Drop, implement test-first, and rerun affected gates. Ask only for unresolved scope or behavioral decisions. Commit/amend/push/PR require separate authorization; a review-fix request is not publication approval.
