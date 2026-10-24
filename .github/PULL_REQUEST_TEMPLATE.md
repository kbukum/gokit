## Description

<!-- What changed and why? Group capabilities; omit implementation narration. -->

## Scope

<!-- Affected owners and necessary public-contract changes. Use full URLs for cross-repository references. -->

- [ ] Feature
- [ ] Fix
- [ ] Breaking change
- [ ] Refactor
- [ ] Documentation

## Validation

<!-- List exact commands and results. Check only verified evidence; distinguish unit, integration, and consumer proof. -->

- [ ] Affected build, vet, lint, and race/shuffle tests pass
- [ ] Required integration and coverage acceptance is verified
- [ ] Dependency vulnerability/license checks pass when applicable
- [ ] Documentation links and changed examples are checked

## Breaking changes

<!-- State the impact and replacement contract. Pre-stable changes need no compatibility shims. -->

## Sibling parity

<!-- Confirm capability parity, link tracking with a full URL, or explain an outstanding gap. -->

## Checklist

- [ ] Code is placed per [Architecture](../docs/ARCHITECTURE.md) and reuses the canonical owner
- [ ] Public API is minimal and typed; breaking changes are in the CHANGELOG
- [ ] Errors keep causes; no runtime panics, ignored errors, or success-shaped fallbacks
- [ ] Goroutines, queues, retries, and remote calls are owned, bounded, and cancellable
- [ ] Inputs are validated at trust boundaries; no secrets in source, logs, or URLs
- [ ] Behavior and failure paths have tests; fixes include regressions
- [ ] Godoc, READMEs, examples, MODULE-INDEX, and changelog notes match the change
- [ ] Meets the [Definition of Done](engineering.md#definition-of-done)

## Outstanding work

<!-- Real blockers or unverified acceptance only. Leave unchecked evidence honest. -->
