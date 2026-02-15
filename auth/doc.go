// Package auth provides authentication and authorization building blocks.
//
// This package is organized into focused subpackages:
//
//   - auth/jwt       — Generic JWT token service using Go generics
//   - auth/password  — Password hashing (bcrypt, argon2id) and secure token generation
//   - auth/authctx   — Type-safe request context propagation for claims
//   - auth/permission — Permission checking interfaces and pattern matching utilities
//
// Each subpackage is independent — import only what you need.
// None of these packages enforce a specific claims structure, role model,
// or permission scheme. They provide building blocks that projects compose
// according to their own requirements.
package auth
