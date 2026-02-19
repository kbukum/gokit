// Package auth provides authentication building blocks.
//
// This is a Go module (github.com/kbukum/gokit/auth) with focused subpackages:
//
//   - auth/jwt        — Generic JWT token service using Go generics
//   - auth/password   — Password hashing (bcrypt, argon2id) and secure token generation
//   - auth/authctx    — Type-safe request context propagation for claims
//   - auth/oidc       — OIDC/OAuth2 building blocks (discovery, verification, PKCE)
//
// The top-level package provides shared contracts:
//
//   - TokenValidator  — interface for validating tokens (JWT, OIDC, API key, etc.)
//   - TokenGenerator  — interface for generating signed tokens
//   - Registry        — thread-safe registry of named TokenValidator instances
//   - Config          — composable configuration with pointer sub-configs
//
// For authorization (permission checking, RBAC), see github.com/kbukum/gokit/authz.
//
// All packages follow gokit conventions: Config structs with ApplyDefaults()/Validate(),
// constructor functions, and mapstructure tags for config file loading.
//
// The top-level Config composes subpackage configs as pointers — only configure
// what you need:
//
//	auth:
//	  enabled: true
//	  jwt:
//	    secret: "my-secret"
//	    access_token_ttl: "15m"
//	  password:
//	    algorithm: "bcrypt"
//	    bcrypt_cost: 12
//
// Register validators for use with middleware:
//
//	reg := auth.NewRegistry()
//	reg.Register("jwt", jwtSvc.AsValidator())
//	validator, _ := reg.Default()
package auth
