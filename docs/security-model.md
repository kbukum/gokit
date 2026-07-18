# gokit security model

## Scope

This document covers the security baseline for `auth`, `authz`, `security`,
and their interaction with `encryption`.

## Threat model

gokit assumes:

- attackers can replay bearer tokens if transport or nonce/state handling is weak
- attackers may attempt JWT algorithm-confusion, `alg:none`, or weak symmetric-secret abuse
- attackers may attempt privilege escalation through missing default-deny
  or cross-tenant authorization gaps
- attackers may exploit missing browser response headers where gokit powers HTTP transports

## Authentication

### JWT

- default signing algorithm: `RS256`
- allowed signing algorithms: `RS256`, `ES256`, `EdDSA`
- `HS256` is supported only with explicit `allow_symmetric_hmac=true` for internal-only deployments
- required claims on parsed tokens: `iss`, `aud`, `exp`, `iat`, `nbf`
- clock skew is explicit and capped at 60 seconds; default is 30 seconds
- `alg:none` and algorithm mismatch are rejected

### API keys

- keys are generated with random entropy and a validated prefix
- keys are stored only as **HMAC-SHA-256 digests** keyed by a required pepper
- validation uses prefix lookup plus **constant-time** digest comparison
- rotation persists a replacement key ID and bounded grace window

### Password hashing

- default password hash: **Argon2id**
- default parameters: memory `64 MiB`, iterations `3`, parallelism `4`
- bcrypt is retained only as a migration fallback and must use cost `>= 12`

### OIDC

- verifier allow-list defaults to `RS256`, `ES256`, `EdDSA`
- public clients require PKCE and must not configure a client secret
- redirect URIs must be exact absolute URIs with no wildcards or fragments
- state, nonce, and PKCE validation helpers are provided for OAuth 2.1 flows

## Authorization

- canonical authorization model: **RBAC + ABAC**
- RBAC roles support hierarchy and wildcard resource/action grants
- ABAC policies evaluate subject, resource, and request-context attributes
- deny policies override allow policies and role grants
- unmatched requests are rejected with explicit **default deny**

## Transport and browser hardening

### TLS

- TLS 1.2 is the minimum allowed version
- TLS 1.3 is negotiated by default when both peers support it
- insecure floors below TLS 1.2 are rejected during validation

### HTTP security headers

`security.HeadersConfig` defaults to:

- `Strict-Transport-Security`
- `Content-Security-Policy`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Permissions-Policy` with a deny-by-default posture

## Token lifecycle

- access and refresh tokens have separate TTLs
- OIDC refresh flows support refresh-token rotation responses
- API consumers should treat refresh tokens as one-time rotation candidates
  and persist replacements atomically
