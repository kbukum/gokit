# fs

Local filesystem primitives for safe paths, atomic writes, secret reads, locks, archives, and change watching. Higher-level storage packages reuse this owner; standard-library filesystem APIs remain the foundation.

Typed failures expose safe operation messages. Local paths and raw OS diagnostics stay in the error cause for logging, not in public messages or validation details. Plain sentinel errors remain available for `errors.Is`; normalize errors at transport boundaries rather than serializing `err.Error()`.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package example

import (
    "github.com/kbukum/gokit/fs"
)

func writeFile(root, relativePath string, data []byte) error {
    full, err := fs.SafeJoin(root, relativePath)
    if err != nil {
        return err
    }
    return fs.WriteAtomicReplace(full, data, "tmp-")
}
```

Replacement is atomic on Unix. On Windows, `WriteAtomicReplace` removes the destination before renaming. Path checks do not protect against arbitrary concurrent filesystem mutation; use trusted directory ownership or an OS-confined filesystem.

## Key Types & Functions

| Name | Description |
|------|-------------|
| `SafeJoin(root, rel)` | Join a user-provided relative path, rejecting escapes |
| `ValidateRelativePath()` / `NormalizeRelativePath()` | Validate / normalize relative paths |
| `ConfinePath()` / `ConfineExistingPath()` | Reject symlink escapes for untrusted paths |
| `Absolute()` / `Canonicalize()` | Resolve absolute / symlink-canonical paths |
| `WriteAtomic()` / `WriteAtomicReplace()` | Atomic same-filesystem writes |
| `WriteAtomicCreate()` | Publish an owner-only file without overwriting an existing destination; requires an existing parent and hard-link support |
| `WithFileLock(ctx, path, operation)` | Coordinate cooperating processes around a caller-owned lock file |
| `ReadSecret(path, minimum, limit)` | Bounded regular-file secret read, preserving bytes except one trailing LF or CRLF; supports mounted symlinks and never includes contents in errors |
| `CanRead()` / `CanWrite()` / `IsReadonly()` | Capability checks before optional operations |
| `Mode()` / `SetMode()` / `IsExecutable()` | Permission inspection and updates |
| `FindInAncestors()` / `ParentDir()` | Walk ancestor directories |
| `TarGz()` / `Zip()` / `ExtractTarGz()` / `ExtractZip()` | Create and bounded-extract tar.gz / zip archives |
| `ExtractLimits` / `DefaultExtractLimits()` | Bound entry count and total uncompressed bytes; reject non-local paths and link escapes |
| `HardLink()` / `SymlinkFile()` / `SymlinkDir()` / `ReadLink()` | Hard and symbolic links |
| `NewFsWatcher()` / `FsWatcher` | Debounced filesystem change watching |
| `AppCacheDir(app)` | OS-standard per-application cache directory |

---

[⬅ Back to main README](../README.md)
