# util

Small, generic helpers shared across gokit — collections, pointers, string casing and sanitization, secret redaction, environment access, hashing, byte/duration formatting, and templates. It is the scoped foundation owner for helpers too small for their own package; reach for the standard library (`slices`, `maps`, `cmp`) or a dedicated owner (`fs`, `codec`) first.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/kbukum/gokit/util"
)

func main() {
    // Pointer helpers
    name := util.Ptr("hello")
    fmt.Println(util.Deref(name)) // "hello"

    // Slice utilities
    nums := []int{1, 2, 3, 2, 1}
    unique := util.Unique(nums)                                     // [1, 2, 3]
    even := util.Filter(nums, func(n int) bool { return n%2 == 0 }) // [2, 2]

    // String sanitization and casing
    safe := util.IsSafeString("SELECT * FROM users") // false
    snake := util.ToSnakeCase("camelCaseString") // "camel_case_string"

    // Human-readable sizes and durations
    fmt.Println(util.FormatBytes(1536)) // "1.5 KiB"

    // Environment access with a typed fallback
    debug := util.GetEnvBool("DEBUG", false)
    _ = debug
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Ptr[T]()` / `Deref[T]()` | Pointer creation and safe dereference |
| `Contains[T]()` / `Filter[T]()` / `Map[T,U]()` / `Unique[T]()` | Generic slice operations |
| `Chunk[T]()` / `Partition[T]()` / `GroupBy[T,K]()` / `IndexBy[T,K]()` | Slice partitioning and indexing |
| `Keys[K,V]()` / `Values[K,V]()` / `DeepMerge()` / `Coalesce[T]()` | Map utilities and first-non-zero |
| `ToSnakeCase()` / `ToKebabCase()` / `ToCamelCase()` | String casing |
| `Truncate()` / `TruncateEllipsis()` | Byte-bounded, rune-safe truncation |
| `SanitizeString()` / `SanitizeEnvValue()` / `IsSafeString()` | Input sanitization and SQL/XSS detection |
| `SecretString` / `SecretKeyMatcher` / `MaskSecret()` | In-memory secret redaction (crypto lives in `encryption`) |
| `GetEnv()` / `GetEnvOr()` / `GetEnvBool()` / `GetEnvParsed[T]()` | Environment variable access |
| `ContentHasher` / `Sha256Hex()` / `HashHex()` / `Sha256Reader()` | Content hashing |
| `FormatBytes()` / `ParseBytes()` | Human-readable byte sizes |
| `FormatDuration()` / `ParseDuration()` / `TimeIt[T]()` | Duration formatting and timing |
| `Clock` / `NewFakeClock()` | Injectable clock for deterministic tests |
| `GlobMatch()` / `NewGlob()` / `HasWildcard()` | Glob pattern matching |
| `Nearest()` / `NearestWithin()` / `ResolveUnique[T]()` | Fuzzy suggestion and unique resolution |
| `ParseTemplate[P]()` / `ParseDynamicTemplate()` | Placeholder template parsing |

---

[⬅ Back to main README](../README.md)
