## Description

<!-- Provide a clear and concise description of your changes -->

## Motivation

<!-- Why is this change needed? What problem does it solve? -->
<!-- Link to related issues: Fixes #123 or Closes #456 -->

## Type of Change

<!-- Mark the relevant option with an 'x' -->

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Refactoring (no functional changes)
- [ ] Performance improvement
- [ ] Test coverage improvement

## Module(s) Affected

<!-- List the modules this PR changes (e.g., server, database, cache) -->

-

## Changes Made

<!-- List key changes in bullet points -->

-
-
-

## Testing

<!-- Describe how you tested your changes. Prefer affected-scope commands. -->

- [ ] `make test` (or `make test M=<module>` / `make test-affected`) passes locally
- [ ] `make lint` is clean (includes `vet` + `depguard` layering)
- [ ] `make fmt` reports no diffs (`gofmt -s`)
- [ ] `govulncheck ./...` passes for the affected module(s)
- [ ] `make check-<domain>` passes for the affected domain
- [ ] Manual testing performed (describe below if applicable)

### Test Evidence

<!-- Optional: show test output, screenshots, or logs demonstrating your changes work -->

```
$ make test M=<module>
...
```

## Breaking Changes

<!-- Pre-stable: breaking changes are allowed and preferred over compat shims.
If this is breaking, describe the impact and the redesign (not a migration shim). -->

## Sibling Parity

<!-- gokit mirrors rskit (the reference kit) and pykit. If this change touches a
public abstraction (error codes, Component lifecycle, Provider shapes, stream, etc.),
confirm parity or link the corresponding sibling item as a full URL, e.g.
https://github.com/kbukum/rskit/issues/123 -->

- [ ] Sibling-parity not required (internal change)
- [ ] Sibling-parity tracked: rskit <url>, pykit <url>
- [ ] Reveals an upstream rskit gap (tagged **IMPROVE-RSKIT** in the description)

## Checklist

- [ ] Code follows the [coding standards](../CONTRIBUTING.md#coding-standards) in CONTRIBUTING.md
- [ ] Behavior was developed test-first; new/changed behavior has tests (failure paths included)
- [ ] Bug fixes include a regression test that fails without the fix
- [ ] Suite is green under `-race -shuffle=on`; coverage meets the per-package floors (≥80% pkg, ≥85% security-load-bearing) and the CI codecov gate (project 80% / patch 85%)
- [ ] No public `interface{}`/`any` (generics/typed); documented exceptions only
- [ ] No `panic`/`log.Fatal`/ignored errors/unchecked type assertions on runtime paths
- [ ] Dependencies (logger/tracer/policies/registries) injected — no globals or `init()` side effects
- [ ] Code split by concern into focused files — not piled into one file
- [ ] Exported items have godoc; each package has a `doc.go`; docs updated
- [ ] `make tidy` run so go.mod/go.sum are clean for affected modules
- [ ] CHANGELOG entry added under `[Unreleased]`
- [ ] New dependencies (if any) are justified, minimal, and pass `govulncheck`

## Additional Notes

<!-- Any extra context, screenshots, or information for reviewers -->
