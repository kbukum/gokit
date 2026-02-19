# authz

Authorization building blocks with pluggable permission checking and wildcard pattern matching.

This module has **zero external dependencies** (standard library only).

## Install

```bash
go get github.com/kbukum/gokit/authz@latest
```

## Quick Start

### Permission Checking

```go
import "github.com/kbukum/gokit/authz"

checker := authz.NewMapChecker(map[string][]string{
    "admin":  {"*:*"},
    "editor": {"article:*", "media:read"},
    "viewer": {"*:read"},
})

checker.HasPermission("admin", "article:delete")  // true
checker.HasPermission("editor", "article:write")   // true
checker.HasPermission("viewer", "article:write")   // false
```

### Custom Checker

```go
// Function-based checker
checker := authz.CheckerFunc(func(subject, permission string) bool {
    return db.HasPermission(subject, permission)
})

// Or implement the interface
type MyChecker struct { /* ... */ }
func (c *MyChecker) HasPermission(subject, permission string) bool { /* ... */ }
```

### Pattern Matching

```go
authz.MatchPattern("article:*", "article:read")   // true
authz.MatchPattern("*:read", "user:read")          // true
authz.MatchPattern("*:*", "anything:here")         // true
authz.MatchPattern("article:read", "article:write") // false
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Checker` | Interface — `HasPermission(subject, permission) bool` |
| `CheckerFunc` | Adapter for ordinary functions |
| `NewMapChecker(permissions)` | In-memory map-backed checker with wildcard support |
| `MatchPattern(pattern, required)` | Wildcard matching (`article:*`, `*:read`) |
| `MatchAny(patterns, required)` | OR logic across multiple patterns |

---

[← Back to main gokit README](../README.md)
