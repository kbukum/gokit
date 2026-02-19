// Package authz provides authorization building blocks.
//
// It defines a Checker interface that projects implement according to their
// needs — whether that's a simple in-memory map, a database-backed system,
// Casbin, Oso, or any other authorization engine.
//
// The package also provides pattern matching utilities for wildcard-based
// permission schemes (e.g., "pipeline:*" matches "pipeline:read").
//
// This module has zero external dependencies (standard library only),
// so it can be used in any project without pulling in authentication
// or cryptography libraries.
//
// Usage:
//
//	checker := authz.NewMapChecker(map[string][]string{
//	    "admin":  {"*:*"},
//	    "editor": {"article:*", "media:read"},
//	    "viewer": {"*:read"},
//	})
//
//	allowed := checker.HasPermission("admin", "article:delete") // true
package authz
