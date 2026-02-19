# auth

Authentication building blocks with JWT, password hashing, OIDC verification, and shared token validation interfaces.

For authorization (permission checking, RBAC), see [authz](../authz/).

## Install

```bash
go get github.com/kbukum/gokit/auth@latest
```

## Quick Start

### Token Validation Interface

The `auth.TokenValidator` interface is the shared contract used by middleware and interceptors:

```go
import "github.com/kbukum/gokit/auth"

// From a JWT service
validator := auth.NewValidator(jwtSvc.ValidatorFunc())

// From a custom function
validator := auth.TokenValidatorFunc(func(token string) (any, error) {
    return myCustomValidation(token)
})
```

### Provider Registry

Register multiple validators and select by name:

```go
reg := auth.NewRegistry()
reg.Register("jwt", auth.NewValidator(jwtSvc.ValidatorFunc()))
reg.Register("apikey", auth.TokenValidatorFunc(myAPIKeyValidator))
reg.SetDefault("jwt")

// In middleware setup
validator, _ := reg.Default()
router.Use(middleware.Auth(validator))
```

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

### Composable Config

Only configure what you need — unused sections are nil:

```yaml
auth:
  enabled: true
  jwt:
    secret: "my-secret"
    access_token_ttl: "15m"
  # password and oidc are omitted — no validation or defaults applied
```

## Key Types & Functions

### `auth` (top-level)

| Symbol | Description |
|---|---|
| `TokenValidator` | Interface — `ValidateToken(token) (any, error)` |
| `TokenValidatorFunc` | Adapter for ordinary functions |
| `TokenGenerator` | Interface — `GenerateToken(claims) (string, error)` |
| `NewValidator(fn)` | Bridge helper for `ValidatorFunc()` |
| `Registry` | Thread-safe named validator registry |
| `NewRegistry()` | Constructor for Registry |
| `Config` | Composable config with pointer sub-configs |

### `auth/jwt`

| Symbol | Description |
|---|---|
| `Service[T]` | Generic JWT service parameterized by claims type |
| `NewService[T](cfg, newEmpty)` | Constructor with claims factory |
| `Generate(claims)` | Sign a token |
| `GenerateAccess(claims)` | Access token with configured TTL |
| `GenerateRefresh(claims)` | Refresh token with configured TTL |
| `Parse(tokenString)` | Parse and validate a token |
| `ValidatorFunc()` | Returns `func(string) (any, error)` for middleware |
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
