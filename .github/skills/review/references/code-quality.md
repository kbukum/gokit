# Code quality

Can the next reader find, understand, and change this code? Correctness lenses can pass while the code is still a liability.

Rules: [Code style and structure](../../../engineering.md#code-style-and-structure), [Change workflow](../../../engineering.md#change-workflow). Detail: [naming and structure](../../../../docs/naming-and-structure.md).

## Look for

| Check | Severity |
|---|---|
| One file mixing unrelated concerns (types, transport, helpers) | should-fix |
| Package with many files in 2–3 separable groups that should be sub-packages | should-fix |
| Code or imports in `doc.go` | should-fix |
| Function too long or deeply nested to follow; clever one-liners hiding logic | should-fix |
| Copy-pasted blocks with small tweaks | should-fix |
| Dead, commented-out, or speculative code; TODO without a tracked issue | should-fix |
| Symptom patch where a root-cause redesign is due | should-fix |
| One-off style that differs from neighboring code (options, constructors, errors) | should-fix |
| Comment that restates code, narrates history, or no longer matches behavior | should-fix |
| Hard-wrapped comment prose | nit |

File length is a signal, not a verdict. A cohesive file may be long. Name the exact split: which file, into which files, along which concern.

## Detection starters

```bash
make structure                                              # doc.go, large files, crowded packages (advisory)
find <path> -name '*.go' -not -name '*_test.go' | xargs wc -l | sort -rn | head -20
grep -rn --include=*.go 'TODO\|FIXME\|HACK' <path> | grep -v _test.go
grep -rniE --include=*.go '//.*(previously|used to|no longer|now we|changed to)' <path> | grep -v _test.go
```

`make lint` covers formatting and many bug patterns. Organization and readability are judged here.
