// Package authz provides authorization building blocks with explicit default-deny
// semantics.
//
// The canonical model is RBAC + ABAC:
//   - RBAC roles contribute hierarchical resource/action permissions
//   - ABAC policies refine access using subject, resource, and request context
//   - deny policies override role grants
//
// The legacy Checker and MapChecker helpers remain available for lightweight
// wildcard-based permission checks, but Engine is the preferred Group 05 shape.
package authz
