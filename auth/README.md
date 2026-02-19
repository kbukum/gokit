# auth

Authentication and authorization building blocks with JWT, password hashing, OIDC, and permission checking.

## Install

```bash
go get github.com/kbukum/gokit/auth@latest
```

## Quick Start

### JWT Token Service

```go
import "github.com/kbukum/gokit/auth/jwt"

cfg := &jwt.Config{Secret: "my-secret", Method: "HS256", AccessTokenTTL: 15 * time.Minute}
cfg.ApplyDefaults()

svc, _ := jwt.NewService[*MyClaims](cfg, func() *MyClaims { return &MyClaims{} })
token, _ := svc.GenerateAccess(claims)
parsed, _ := svc.Parse(token)
```

### Password Hashing

```go
import "github.com/kbukum/gokit/auth/password"

hasher := password.NewHasher(password.Config{Algorithm: "bcrypt"})
hash, _ := hasher.Hash("my-password")
err := hasher.Verify("my-password", hash)
```

### OIDC Verification

```go
import "github.com/kbukum/gokit/auth/oidc"

verifier, _ := oidc.NewVerifier(ctx, "https://issuer.example.com", oidc.VerifierConfig{ClientID: "my-app"})
idToken, _ := verifier.Verify(ctx, rawIDToken)
```

## Key Types & Functions

### `auth/jwt`

| Symbol | Description |
|---|---|
| `Service[T]` | Generic JWT service parameterized by claims type |
| `NewService[T](cfg, newEmpty)` | Constructor with claims factory |
| `Generate(claims)` | Sign a token |
| `GenerateAccess(claims)` | Access token with configured TTL |
| `GenerateRefresh(claims)` | Refresh token with configured TTL |
| `Parse(tokenString)` | Parse and validate a token |
| `Config` | Secret, PrivateKeyPath, Method, Issuer, Audience, TTLs |

### `auth/password`

| Symbol | Description |
|---|---|
| `Hasher` | Interface — `Hash(password)`, `Verify(password, hash)` |
| `NewHasher(cfg)` | Factory from config (bcrypt or argon2id) |
| `GenerateToken(length)` | Cryptographically secure random token |
| `HashSHA256(input)` | SHA256 hex digest for token storage |

### `auth/authctx`

| Symbol | Description |
|---|---|
| `Set(ctx, claims)` | Store claims in context |
| `Get[T](ctx)` | Type-safe claims retrieval |
| `MustGet[T](ctx)` | Panic if claims missing |
| `GetOrError[T](ctx)` | Error-based retrieval |

### `auth/permission`

| Symbol | Description |
|---|---|
| `Checker` | Interface — `HasPermission(subject, permission)` |
| `NewMapChecker(permissions)` | In-memory map-backed checker |
| `MatchPattern(pattern, required)` | Wildcard matching (`article:*`, `*:read`) |

### `auth/oidc`

| Symbol | Description |
|---|---|
| `Provider` | Interface — `AuthURL`, `Exchange`, `UserInfo` |
| `Verifier` | OIDC token verification with JWKS caching |
| `NewVerifier(ctx, issuer, cfg)` | Create verifier with auto-discovery |
| `NewPKCE()` | Generate PKCE code verifier/challenge pair |
| `GenerateState()` | CSRF state token |

---

[← Back to main gokit README](../README.md)
