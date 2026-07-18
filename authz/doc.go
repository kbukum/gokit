// Package authz provides authorization building blocks with explicit default-deny semantics.
//
// The canonical model is RBAC + ABAC:
//   - RBAC roles contribute hierarchical resource/action permissions
//   - ABAC policies refine access using subject, resource, and request context
//   - deny policies override role grants
//
// Checker and MapChecker provide lightweight wildcard permission checks; Engine provides full Group 05 RBAC + ABAC evaluation.
package authz
