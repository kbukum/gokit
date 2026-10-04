# auth

Typed authentication with opaque browser sessions, indexed HMAC-protected API keys, locked JWT policy, Argon2id-first password hashing, and OIDC verification. Browser protocol and composition are documented in [session](session/README.md).

For authorization (permission checking, RBAC), see [authz](../authz/).

## Install

```bash
go get github.com/kbukum/gokit/auth@latest
```

## Quick Start

### Token Validation Interface

The `auth.TokenValidator[T]` interface preserves the caller's claims type and request cancellation:

```go
import "github.com/kbukum/gokit/auth"

// From a JWT service
validator := auth.NewValidator(jwtSvc.ValidatorFunc())

// From a custom function
validator := auth.TokenValidatorFunc[*MyClaims](func(ctx context.Context, token string) (*MyClaims, error) {
    return myCustomValidation(ctx, token)
})
```

### Provider Registry

Register multiple validators and select by name:

```go
reg := auth.NewRegistry[*MyClaims]()
reg.Register("jwt", auth.NewValidator(jwtSvc.ValidatorFunc()))
reg.SetDefault("jwt")

// In middleware setup
validator, _ := reg.Default()
// Auth injects validated claims via a ClaimsSetter (authctx.Set) so the
// transport layer stays decoupled from this module, and returns an error for
// invalid configuration.
authMW, err := middleware.Auth(validator, authctx.Set[*MyClaims])
if err != nil {
    log.Fatal(err)
}
router.Use(authMW)
```

### JWT Token Service

```go
import "github.com/kbukum/gokit/auth/jwt"

cfg := &jwt.Config{
    Method: "EdDSA",
    PrivateKey: ed25519PrivateKey,
    PublicKey: ed25519PublicKey,
    Issuer: "https://auth.example.com",
    Audience: []string{"api"},
    AccessTokenTTL: 15 * time.Minute,
}
cfg.ApplyDefaults()

svc, _ := jwt.NewService[*MyClaims](cfg, func() *MyClaims { return &MyClaims{} })
token, _ := svc.GenerateAccess(claims)
parsed, _ := svc.Parse(token)
```

### Header API keys

`apikey.NewMemoryStore(0)` supplies the bounded core default: 1,024 keys, no eviction or background workers, and indexed HMAC digest lookup. Supply a positive capacity up to 4,096 when needed. Records are cloned on reads and writes; call `Delete` explicitly to reclaim capacity.

```go
store, err := apikey.NewMemoryStore(0)
hasher, err := apikey.NewHasher(apikey.HashingConfig{
    Pepper: injectedPepper,
    Random: rand.Reader,
})
keys := apikey.NewManager(store, hasher, apikey.WithClock(util.SystemClock{}))
issued, record, err := keys.IssueKey(ctx, apikey.IssueRequest{
    KeyID: "fixture-key", OwnerID: "service-123", Prefix: "fixture",
    Kind: auth.Service, RestrictionMode: auth.Unrestricted,
})
```

Handle each error before continuing. `issued.PlainKey` is one-time credential material: deliver it privately, never log it. `record` contains only protected digest and metadata. `keys.Authenticate(request)` reads `X-API-Key` and returns the same `auth.Principal` as session authentication; compose it with `auth.NewChain(sessionManager, keys)`. API-key restrictions are ceilings, not application membership grants. The memory store bounds each record to 16 KiB of metadata and 256 resources/scopes per list; persistent stores implement the same typed `apikey.Store` port.

Key validation performs only authoritative reads; it never writes `LastUsedAt`. Usage observation is an explicit, separately owned `Store.UpdateLastUsed` operation and cannot determine credential validity. HMAC digests use a versioned domain prefix: `HashingConfig.Domain` defaults to `"apikey"`; the session manager uses `"session"`. These domains remain cryptographically distinct even with the same pepper. The CSRF signing secret must still be independent.

### Password Hashing

```go
import "github.com/kbukum/gokit/auth/password"

hasher := password.NewHasher(password.Config{})
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
    method: "EdDSA"
    issuer: "https://auth.example.com"
    audience: ["api"]
    access_token_ttl: "15m"
  # password and oidc are omitted — no validation or defaults applied
```

## Key Types & Functions

### `auth` (top-level)

| Symbol | Description |
|---|---|
| `TokenValidator[T]` | Interface — `ValidateToken(ctx, token) (T, error)` |
| `TokenValidatorFunc[T]` | Adapter for typed context-aware functions |
| `TokenGenerator[T]` | Interface — `GenerateToken(ctx, claims) (string, error)` |
| `NewValidator(fn)` | Bridge helper for `ValidatorFunc()` |
| `Registry[T]` | Thread-safe typed named validator registry |
| `NewRegistry[T]()` | Constructor for Registry |
| `Principal` | Subject, user/service kind, credential mechanism, protected reference, expiry and explicit ceilings |
| `RequestAuthenticator` | `Authenticate(*http.Request) (Principal, error)` |
| `NewChain(session, key)` | `Chain` that dispatches exactly one cookie or API key; reports genuine absence as `present == false` with no identity |
| `Authorize(ctx, policy, principal, resource, scopes...)` | Application membership intersected with the credential ceiling |
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
| `ValidatorFunc()` | Returns `func(context.Context, string) (T, error)` |
| `ValidateToken(ctx, token)` | Implements the typed validator structurally |
| `GenerateToken(ctx, claims)` | Implements the typed generator structurally |
| `Config` | Locked signing policy (`RS256`/`ES256`/`EdDSA`, `HS256` explicit-only), issuer, audience, TTLs |

### `auth/password`

| Symbol | Description |
|---|---|
| `Hasher` | Interface — `Hash(password)`, `Verify(password, hash)` |
| `NewHasher(cfg)` | Factory from config (Argon2id default, bcrypt fallback) |
| `GenerateToken(length)` | Cryptographically secure random token |
| `HashSHA256(input)` | SHA256 hex digest for token storage |

### `auth/authctx`

| Symbol | Description |
|---|---|
| `Set[T](ctx, claims)` | Store claims under an independent key for each type |
| `Get[T](ctx)` | Type-safe claims retrieval |
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
