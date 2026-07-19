# fs

Local filesystem primitives for safe paths, temporary files and directories, atomic writes, permissions, and metadata. It stays deliberately below storage abstractions — higher-level packages (`storage`, `cache`, `httpclient`) reuse these primitives instead of each re-implementing path safety, temp files, and atomic replacement. Where the standard library already suffices (`os`, `io/fs`, `path/filepath`), this package builds on it.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "github.com/kbukum/gokit/fs"
)

func main() {
    // Reject symlink / traversal escapes for user-provided relative paths.
    full, err := fs.SafeJoin("/srv/data", userPath)
    if err != nil {
        panic(err)
    }

    // Write without ever exposing a partial file.
    if err := fs.WriteAtomicReplace(full, []byte("hello"), "tmp-"); err != nil {
        panic(err)
    }
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `SafeJoin(root, rel)` | Join a user-provided relative path, rejecting escapes |
| `ValidateRelativePath()` / `NormalizeRelativePath()` | Validate / normalize relative paths |
| `ConfinePath()` / `ConfineExistingPath()` | Reject symlink escapes for untrusted paths |
| `Absolute()` / `Canonicalize()` | Resolve absolute / symlink-canonical paths |
| `WriteAtomic()` / `WriteAtomicReplace()` | Atomic same-filesystem writes |
| `CanRead()` / `CanWrite()` / `IsReadonly()` | Capability checks before optional operations |
| `Mode()` / `SetMode()` / `IsExecutable()` | Permission inspection and updates |
| `FindInAncestors()` / `ParentDir()` | Walk ancestor directories |

---

[⬅ Back to main README](../README.md)
