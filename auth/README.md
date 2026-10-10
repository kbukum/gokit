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

// A JWT service is a TokenValidator[T] itself
var validator auth.TokenValidator[*MyClaims] = jwtSvc

// From a custom function
validator := auth.TokenValidatorFunc[*MyClaims](func(ctx context.Context, token string) (*MyClaims, error) {
    return myCustomValidation(ctx, token)
})
```

### Provider Registry

Register multiple validators and select by name:

```go
reg := auth.NewRegistry[*MyClaims]()
reg.Register("jwt", jwtSvc)
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

A `jwt.Config` is one token profile: method, issuer, audience set, `typ` header, maximum lifetime and explicit leeway. Key material is a separate `jwt.KeySet`; every token carries the `kid` of the key that signed it.

```go
import "github.com/kbukum/gokit/auth/jwt"

type MyClaims struct {
    jwt.Registered
    Scope string `json:"scope"`
}

signing, err := jwt.ParseKey("2026-10", privatePEM) // PKCS #8 "PRIVATE KEY" or PKIX "PUBLIC KEY"
svc, err := jwt.NewService(jwt.Config{
    Method:      jwt.EdDSA,
    Issuer:      "https://auth.example.com",
    Audience:    []string{"api"},
    Type:        "at+jwt",
    MaxLifetime: 15 * time.Minute,
    Leeway:      5 * time.Second,
}, jwt.KeySet{SigningKeyID: "2026-10", Keys: []jwt.Key{signing}}, util.SystemClock{}, func() *MyClaims { return &MyClaims{} })
token, err := svc.GenerateToken(ctx, &MyClaims{Scope: "read"})
claims, err := svc.ValidateToken(ctx, token)
```

Handle each error. Issuance fills `iat` (whole seconds), `nbf = iat`, `exp = iat + MaxLifetime`, the issuer, and the audience when exactly one is configured; claims outside the profile are rejected. Validation requires the configured algorithm, a known string `kid`, the configured `typ` (case-insensitive, optional `application/` prefix), `iat`, `nbf` and `exp` with `iat <= nbf < exp` and `exp - iat <= MaxLifetime`, the issuer, and at least one configured audience (exactly one with `SingleAudience`). Leeway is never implied: zero means none and the maximum is one minute. Tokens over 8 KiB are rejected before parsing.

Rotate keys by publishing the new key, moving `SigningKeyID` to it, and keeping the old key verify-only with `RetireAt` no earlier than the expiry of the last token it signed. At most one key may omit `RetireAt`, and a set holds at most eight keys. A `KeySet` without `SigningKeyID` is verify-only. Use one service per token profile; access and refresh tokens are separate profiles with separate types.

### Header API keys

`apikey.NewMemoryStore(0)` supplies the bounded core default: 1,024 keys, no eviction or background workers, and indexed HMAC digest lookup. Supply a positive capacity up to 4,096 when needed. Records are cloned on reads and writes; call `Delete` explicitly to reclaim capacity.

```go
store, err := apikey.NewMemoryStore(0)
hasher, err := apikey.NewHasher(apikey.HashingConfig{
    Pepper: injectedPepper,
    Random: rand.Reader,
})
keys, err := apikey.NewManager(apikey.Config{Store: store, Hasher: hasher, Clock: util.SystemClock{}})
issued, record, err := keys.IssueKey(ctx, apikey.IssueRequest{
    KeyID: "fixture-key", OwnerID: "service-123", Prefix: "fixture",
    Kind: auth.Service, RestrictionMode: auth.Unrestricted,
})
```

Handle each error before continuing. `issued.PlainKey` is one-time credential material: deliver it privately, never log it. `record` contains only protected digest and metadata. `keys.Authenticate(request)` reads `X-API-Key` and returns the same `auth.Principal` as session authentication; compose it with `auth.NewChain(sessionManager, keys)`. API-key restrictions are ceilings, not application membership grants. The memory store bounds each record to 16 KiB of metadata and 256 resources/scopes per list; persistent stores implement the same typed `apikey.Store` port and must pass the `apikey/apikeytest` contract suite (`apikeytest.Run(t, apikeytest.Harness{NewStore: ...})`).

`keys.Authenticate` sets `Principal.Reference` to the key id. `keys.ValidateKeyID(ctx, id, scopes...)` applies the same revocation, expiry, grace and scope rules without the plaintext, for trusted callers such as a delegation issuer revalidating the key behind a token. `keys.RotateKey(ctx, oldID, apikey.RotateRequest{NewKeyID: "...", Grace: time.Hour})` replaces a valid, unrotated key in one atomic `Store.Rotate`: the replacement inherits owner, name, prefix, kind and restrictions; `Grace` is explicit (zero ends the old key at once, at most `apikey.MaxGrace` of 30 days) and never outlives the old key's expiry. A revoked, expired or already rotated key returns `CONFLICT`, so rotation cannot race revocation or another rotation. `keys.RevokeKey(ctx, id)` is one-way and also ends any remaining grace.

Key validation performs only authoritative reads; it never writes `LastUsedAt`. Usage observation is an explicit, separately owned `Store.UpdateLastUsed` operation and cannot determine credential validity. HMAC digests use a versioned domain prefix: `HashingConfig.Domain` defaults to `"apikey"`; the session manager uses `"session"`. These domains remain cryptographically distinct even with the same pepper. The CSRF signing secret must still be independent.

### Password Hashing

```go
import "github.com/kbukum/gokit/auth/password"

hasher, err := password.NewHasher(password.Config{MinLength: 15})
pool, err := password.NewPool(hasher, password.PoolConfig{Workers: 4, Queue: 64, MaxWait: time.Second})
hash, err := pool.Hash(ctx, newPassword)
err = pool.Verify(ctx, presented, hash)
```

`NewHasher` validates its Config and defaults to argon2id; bounded bcrypt remains available. MinLength (default 15) and MaxLength (default 128, 72 for bcrypt) count characters and apply at Hash. Verify distinguishes wrong passwords (`PASSWORD_MISMATCH`), malformed stored encodings (`PASSWORD_HASH_INVALID`) and unsupported persisted costs (`PASSWORD_COST_UNSUPPORTED`). VerifyLimits defaults to the issuing parameters and bounds encoded hashes to 512 bytes; supported ceilings are Argon2id 256 MiB/6 passes/8 lanes and bcrypt cost 14. Issuance must fit verification limits.

Pool admits at most Workers KDF calls and Queue waiters for MaxWait, refusing extra work with `RATE_LIMITED`/`PASSWORD_BUSY`. Memory budgeting uses the supported verification envelope, not just issuance. Close(ctx) closes admission, cancels waiters and drains running non-preemptible work. A timed-out Close retains ownership; another Close can finish. Caller cancellation during KDF returns cancellation after completion and keeps capacity occupied until then. Composition drains the pool before releasing its dependencies.

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
    type: "at+jwt"
    max_lifetime: "15m"
    leeway: "5s"
  # password and oidc are omitted — no validation or defaults applied
```

## Key Types & Functions

### `auth` (top-level)

| Symbol | Description |
|---|---|
| `TokenValidator[T]` | Interface — `ValidateToken(ctx, token) (T, error)` |
| `TokenValidatorFunc[T]` | Adapter for typed context-aware functions |
| `TokenGenerator[T]` | Interface — `GenerateToken(ctx, claims) (string, error)` |
| `NewValidator(fn)` | Adapts a typed validation function |
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
| `Service[T]` | Issues and verifies one token profile; implements `TokenValidator[T]` and `TokenGenerator[T]` |
| `NewService(cfg, keys, clock, newClaims)` | Validates the profile and key set; the clock is required |
| `Config` | Token profile: method (`RS256`/`ES256`/`EdDSA`, `HS256` explicit-only), issuer, audience, single audience, `typ`, max lifetime, leeway |
| `KeySet`, `Key` | Verification keys by `kid`, the signing key, and per-key retirement |
| `ParseKey(id, pem)` | Reads one PKCS #8 private or PKIX public key PEM block |
| `Registered`, `Claims` | Embeddable registered claims and the claims constraint |

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
