// Package auth provides authentication building blocks.
//
// This is a Go module (github.com/kbukum/gokit/auth) with focused subpackages:
//
//   - auth/jwt        — Generic JWT token service using Go generics
//   - auth/password   — Password hashing (bcrypt, argon2id) and secure token generation
//   - auth/authctx    — Type-safe request context propagation for claims
//   - auth/oidc       — OIDC/OAuth2 building blocks (discovery, verification, PKCE)
//   - auth/apikey     — Indexed HMAC-protected header credentials
//   - auth/session    — Opaque browser sessions, CSRF, family revocation and stream lifetimes
//
// The top-level package provides shared contracts:
//
//   - TokenValidator[T] — typed, context-aware token validation
//   - TokenGenerator[T] — typed, context-aware signed token generation
//   - Registry[T]       — thread-safe registry of named typed validators
//   - Principal         — one identity with explicit credential ceilings
//   - RequestAuthenticator — a shared request authentication seam
//   - Config          — composable configuration with pointer sub-configs
//
// For authorization (permission checking, RBAC), see github.com/kbukum/gokit/authz.
//
// All packages follow gokit conventions: Config structs with ApplyDefaults()/Validate(),
// constructor functions, and mapstructure tags for config file loading.
//
// The top-level Config composes subpackage configs as pointers — only configure what you need:
//
//	auth:
//	  enabled: true
//	  jwt:
//	    method: "EdDSA"
//	    issuer: "https://auth.example.com"
//	    audience: ["api"]
//	    type: "at+jwt"
//	    max_lifetime: "15m"
//	  password:
//	    algorithm: "argon2id"
//
// Register validators for use with middleware:
//
//	reg := auth.NewRegistry[*MyClaims]()
//	reg.Register("jwt", jwtSvc)
//	validator, _ := reg.Default()
package auth
