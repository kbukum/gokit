// Package sse provides scoped proto-JSON live events with epoch/sequence replay, priority reset/failure controls, bounded admission and buffers, and per-write deadlines.
//
// Bus owns single-instance delivery. Handler requires explicit authorization and an injected logger and clock. Access.Lifetime connects session expiry/revocation to stream teardown without importing authentication policy. Application adapters publish generated proto messages; controls use the shared errors vocabulary.
//
// Clients acknowledge applied events rather than received bytes. Filtered sequence gaps are valid. Reset invalidates snapshot generations; racing snapshots must not overwrite newer state. Cross-instance delivery requires a shared bus and a separate bounded revocation policy.
package sse
