// Package fs provides local filesystem primitives for safe paths, temporary files and directories,
// atomic writes, permissions, metadata, symlinks and hard links, bounded archive
// create/extract (tar.gz and zip), debounced change watching, and OS-standard
// application directories.
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
//   - use [CanRead] / [CanWrite] capability checks before optional operations;
//   - extract untrusted archives with [ExtractTarGz] / [ExtractZip] under
//     [ExtractLimits], which bound entry count, per-entry and total size, and
//     reject path traversal and symlink escapes.
package fs
