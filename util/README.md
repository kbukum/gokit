# util

Generic utility functions for slices, maps, pointers, string sanitization, and validation helpers.

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
    unique := util.Unique(nums)           // [1, 2, 3]
    even := util.Filter(nums, func(n int) bool { return n%2 == 0 }) // [2, 2]

    // String sanitization
    safe := util.IsSafeString("SELECT * FROM users") // false
    clean := util.SanitizeString("  hello\x00world  ") // "helloworld"
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Ptr[T]()` / `Deref[T]()` | Pointer creation and safe dereference |
| `Contains[T]()` / `Filter[T]()` / `Map[T,U]()` | Generic slice operations |
| `Unique[T]()` / `Keys[K,V]()` / `Values[K,V]()` | Collection utilities |
| `Coalesce[T]()` | Return first non-zero value |
| `SanitizeString()` / `SanitizeEnvValue()` | Input sanitization |
| `IsSafeString()` | SQL injection / XSS detection |
| `ValidateUUID()` / `ValidateNonEmpty()` | Input validation helpers |

---

[⬅ Back to main README](../README.md)
