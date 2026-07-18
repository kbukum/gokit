// Package namedregistry provides a generic, name-keyed registry.
//
// [New] returns a [Registry] of T whose [Registry.Register] rejects empty names, nil values, and duplicates, and whose iteration is deterministic (sorted names). First-party registries wrap it instead of re-implementing ad-hoc name maps.
package namedregistry
