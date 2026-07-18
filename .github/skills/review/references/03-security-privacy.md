# Pass 03 — Security & privacy

A dedicated pass because a vibe-coded path that "just works" usually skips boundary validation,
and gokit is shared infrastructure — a gap here propagates to every consumer.
For a deeper sweep on security-sensitive changes, pair this with a dedicated security review;
this pass is the standing baseline.

> **Run in a separate, clean-context agent** — never inline in the session that wrote the code.
> An independent reviewer re-derives every judgment from the code
> and the principles instead of trusting prior reasoning.
> A plan/spec may be passed in as a scope checklist only; it never excuses a baseline violation.

**Scope note.** *Changes mode:* trace each new input path from its trust boundary to its use.
*Project mode:* audit the toolkit's external-facing surfaces (HTTP, process, storage/database adapters, auth, crypto) for the invariants below.

## Checks

- **Validate at every trust boundary.** Untrusted input is validated before use; least- privilege
  and secure-by-default. An input that flows into a query, a path, a command,
  or a deserialization without validation is a blocker.
- **Injection-safe data access.** Parameterized queries only —
  never string-built SQL (`fmt.Sprintf`/concatenation into a query).
  Argv-only subprocess execution via `process` —
  no `sh -c` / shell interpolation of untrusted input.
- **Safe token handling.** Tokens/credentials go in headers, never query strings; never logged.
  Redact sensitive fields in errors and logs. Auth is header-only; reject query-string tokens.
- **Current crypto.** No deprecated/weak algorithms (MD5, SHA-1 for security, DES, ECB, static IVs, hard-coded keys);
  use current primitives (AES-GCM, ChaCha20-Poly1305, Argon2id, Ed25519, RS256/ES256).
  Reject `alg: none` in JWT; require `exp`/`iss`/`aud`. Crypto belongs in `encryption` / `security`,
  not hand-rolled.
- **Data minimization.** Minimize, redact, and retention-bound sensitive data; do not persist
  or log more than needed.

## Detection starters

Read each hit to judge intent — these flag candidates, not verdicts.

```bash
# string-built SQL / shelled commands with interpolation
grep -rn --include=*.go 'fmt.Sprintf(.*SELECT\|fmt.Sprintf(.*INSERT\|"sh", "-c"\|"bash"' .
# secrets in URLs/logs, or logging a token/password/secret
grep -rn --include=*.go '(token\|secret\|password\|api_\?key)=' .
grep -rni --include=*.go 'slog\.\|logger\.' . | grep -i 'token\|secret\|password\|apikey'
# weak crypto / alg:none
grep -rni --include=*.go 'md5\|sha1\|"DES"\|ecb\|alg.*none' .
# hard-coded credentials
grep -rn --include=*.go '\(password\|secret\|apiKey\|token\)\s*[:=]\s*"' . | grep -v _test.go
```

Flag any unbounded read of untrusted input (set explicit size limits — `io.LimitReader`)
and any path/selector from an untrusted source flowing into filesystem
or process execution without validation.
Path-shaped values use `filepath.Join` (never hardcoded separators)
and a temp-dir helper over hardcoded `/tmp/...`.
