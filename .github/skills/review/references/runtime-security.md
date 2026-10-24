# Runtime and security

Does the code fail safely, stop cleanly, stay bounded, and treat input as untrusted? These defects pass the build and show up in production.

Rules: [Errors and resilience](../../../engineering.md#errors-and-resilience), [Concurrency and lifecycle](../../../engineering.md#concurrency-and-lifecycle), [Composition](../../../engineering.md#composition), [Security and privacy](../../../engineering.md#security-and-privacy), [AI and model features](../../../engineering.md#ai-and-model-features). Threats: [security model](../../../../docs/security-model.md).

## Look for

| Check | Severity |
|---|---|
| `panic`, `log.Fatal`, ignored error, or unchecked type assertion on a runtime path | blocker |
| Zero value with `nil` error on a real failure | blocker |
| Lost cause (no `%w`), or untyped error on a public path | should-fix |
| Remote call without a context timeout; retry of non-idempotent work; unbounded or un-jittered retry; two retry owners | blocker |
| Goroutine without owner, cancellation, or shutdown | blocker |
| Unbounded channel, queue, or buffer on an ingest path; no drain on shutdown | blocker |
| Lock held across blocking work; shared state without a guard | blocker |
| Mutable package-level registry, `init()` I/O, or a global logger/tracer | blocker |
| Untrusted input reaching SQL, a path, a subprocess, or a decoder without validation or a size limit | blocker |
| String-built SQL, `sh -c`, credentials in URLs or logs, weak crypto, JWT without `alg`/`exp`/`iss`/`aud` checks | blocker |
| Model output or retrieved text trusted without validation; destructive tool call without a human gate | blocker |
| Sensitive data kept or logged beyond need | should-fix |

## Detection starters

```bash
grep -rn --include=*.go 'panic(\|log\.Fatal' . | grep -v _test.go
grep -rnE --include=*.go '^\s*_ = |_, _ = ' . | grep -v _test.go
grep -rn --include=*.go 'go func\|make(chan ' <path> | grep -v _test.go
grep -rn --include=*.go '^var .*[Rr]egistry\|^func init()' . | grep -v _test.go
grep -rn --include=*.go 'Sprintf(.*\(SELECT\|INSERT\|UPDATE\|DELETE\)\|"sh", "-c"' .
grep -rni --include=*.go 'md5\|sha1\|InsecureSkipVerify\|alg.*none' . | grep -v _test.go
grep -rni --include=*.go '\(token\|secret\|password\|apikey\)' <path> | grep -i 'log\|slog\|url\|query'
```

`make lint` covers errcheck, gosec, contextcheck, bodyclose, and SQL close checks. Panics, globals, goroutine ownership, and bounds are judged here.
