// Package auth provides authentication and authorization building blocks.
//
// This is a Go module (github.com/kbukum/gokit/auth) with focused subpackages:
//
//   - auth/jwt        — Generic JWT token service using Go generics
//   - auth/password   — Password hashing (bcrypt, argon2id) and secure token generation
//   - auth/authctx    — Type-safe request context propagation for claims
//   - auth/permission — Permission checking interfaces and pattern matching utilities
//   - auth/oidc       — OIDC/OAuth2 building blocks (discovery, verification, PKCE)
//
// All packages follow gokit conventions: Config structs with ApplyDefaults()/Validate(),
// constructor functions, and mapstructure tags for config file loading.
//
// The top-level Config composes all subpackage configs for convenience:
//
//	auth:
//	  enabled: true
//	  jwt:
//	    secret: "my-secret"
//	    access_token_ttl: "15m"
//	  password:
//	    algorithm: "bcrypt"
//	    bcrypt_cost: 12
//	  oidc:
//	    issuer: "https://accounts.google.com"
//	    client_id: "my-client-id"
package auth
