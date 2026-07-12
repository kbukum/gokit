// Package fs provides local filesystem primitives for safe paths, temporary
// files and directories, atomic writes, permissions, and metadata.
//
// It stays deliberately below storage abstractions: higher-level packages such
// as storage, cache, and httpclient reuse these primitives instead of each
// re-implementing path safety, temp files, and atomic file replacement. Where
// the Go standard library already suffices (os, io/fs, path/filepath), this
// package builds on it rather than replacing it.
//
// Security defaults:
//   - use [SafeJoin] for user-provided relative paths before touching disk, and
//     [ConfineExistingPath] / [ConfinePath] to reject symlink escapes for
//     untrusted absolute or existing paths;
//   - use [WriteAtomic] for same-filesystem writes without exposing partial
//     files, and [WriteAtomicReplace] when an existing file should be replaced;
//   - use [CanRead] / [CanWrite] capability checks before optional operations.
package fs
