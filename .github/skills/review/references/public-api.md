# Public API and stability

Is the exported surface small, typed, and pleasant to call, and is every break visible to consumers?

Rules: [Public API and stability](../../../engineering.md#public-api-and-stability). Policies: [SemVer](../../../../docs/policy/SEMVER.md), [deprecation](../../../../docs/policy/DEPRECATION.md), [versioning](../../../../docs/VERSIONING.md).

## Look for

| Check | Severity |
|---|---|
| Breaking change (removed or changed exported identifier, changed behavior contract) without a CHANGELOG entry | blocker |
| Breaking change in a patch release | blocker |
| `any`/`interface{}` in a public signature without a documented opaque reason | should-fix |
| Exported identifier callers do not need; concrete type exported where an interface and constructor would do | should-fix |
| Large interface where a small one plus opt-in capability interfaces would do | should-fix |
| Long positional signature instead of options or a request struct; `context.Context` not first or stored in a struct | should-fix |
| Compatibility shim, wrapper, or "keep the old path" hedge in pre-1.0 code | should-fix |
| Error that does not tell the caller what failed or what to do | should-fix |
| Stuttering name (`cache.NewCache`) | nit |
| Change to a shared abstraction (`AppError`, `Component`, `Provider`, `Stream`) without an rskit parity note | should-fix |

## Detection starters

```bash
# exported surface touched by the change
git diff <base>...HEAD -- '*.go' | grep -E '^[-+](func|type|var|const) [A-Z]|^[-+]func \([^)]*\) [A-Z]'
# any/interface{} in exported signatures
grep -rnE --include=*.go '^func (\([^)]*\) )?[A-Z][^(]*\(.*(\bany\b|interface\{\})' <path> | grep -v _test.go
# breaking changes recorded?
git diff <base>...HEAD -- CHANGELOG.md
```

Compare the exported surface before and after with `go doc -all <pkg>` when a public type changes.
