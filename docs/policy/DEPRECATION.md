# Deprecation Policy

This policy applies once a module reaches `1.0.0`. While in `0.x.y` we may
remove APIs in any MINOR release (see `SEMVER.md`), but we still try to
follow the spirit of this document where practical.

## Lifecycle of a deprecated API

```
   stable ──► deprecated ──► removed
              ↑           ↑
              MINOR       MAJOR
              release     release (≥ 1 MINOR later)
```

1. **Deprecation** — the API is marked deprecated in a MINOR release.
2. **Cohabitation** — the new and old APIs coexist for at least one full MINOR
   release cycle (target: 6 months of calendar time, minimum: 1 MINOR).
3. **Removal** — the deprecated API is removed in the next MAJOR release.

We never remove a deprecated API in a PATCH or MINOR release after `1.0.0`.

## How we mark deprecation

Every deprecated symbol carries:

1. A `// Deprecated:` doc comment **on the line directly above the
   declaration** (this is the form `gopls`, `staticcheck`, `golangci-lint`,
   and `pkg.go.dev` recognise).
2. The version it was deprecated in.
3. The replacement (or `no replacement, will be removed in vX.Y.Z`).

```go
// Deprecated: since v1.2.0; use [auth.NewVerifier] which threads context.
// Will be removed in v2.0.0.
func NewAuthChecker(cfg Config) *Checker { … }
```

3. A CHANGELOG entry under `### Deprecated` for the release that introduced
   the deprecation.
4. (Where helpful) a runtime `slog.Warn` from the package's first call,
   gated by `sync.Once`, naming the replacement. This is optional — only
   do it for hot-path APIs where a doc comment is easy to miss.

## What counts as a deprecation-eligible change

- Removing a function, method, type, constant, or variable.
- Removing a field from a struct (when the struct is part of the public API).
- Adding a method to an exported interface.
- Tightening a parameter or return type.
- Changing observable runtime behaviour in a way callers might depend on.

The following are **not** deprecations and may ship in a single MINOR/PATCH:

- Adding a new method to a struct.
- Adding a new field to a struct (only if the struct is **not** required to be
  literal-constructed by callers — anything with `unkeyed-fields` warnings
  needs a deprecation).
- Tightening behaviour to fix a documented bug.

## Security exception

A vulnerability fix may break API in a PATCH release if no compatible fix
exists. This is the only exception. Such releases are flagged with `SECURITY:`
in the CHANGELOG and announced via GitHub Security Advisories.

## Deprecation checklist for maintainers

Before merging a deprecation PR:

- [ ] `// Deprecated:` comment on the symbol with version + replacement.
- [ ] CHANGELOG `### Deprecated` entry under `[Unreleased]`.
- [ ] Replacement API exists and is documented.
- [ ] If the replacement requires a non-trivial migration, add a
      `## Migration` block to the CHANGELOG entry showing before/after.
- [ ] Removal date / version recorded in `docs/policy/DEPRECATIONS.csv`
      (sortable list — create on first deprecation).
