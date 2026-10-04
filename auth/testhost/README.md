# Real HTTPS authentication host

**Use this test-only host to exercise the real session, SQLite, HTTP, Connect and SSE adapters.** It is not an identity provider or production application. Its scenario routes exist only in this separate module; production auth/server packages do not import it.

## Run the backend proof

From the repository root, with Go 1.27.1, a C compiler and Docker:

```sh
make auth-integration
```

This builds the actual host binary and runs the production SQL and process integration tests with race detection and shuffled order. PostgreSQL uses the kit's isolated, digest-pinned container fixture. Missing Docker or a failed/forced teardown is a failure, not a skipped pass. The browser consumer must run its own real browser journeys; this command does not certify a UI.

For an explicitly narrower local check:

```sh
go test ./auth/... ./auth/session/database/... ./auth/testhost/... -race -shuffle=on -count=1
```

Without `-tags=integration`, this does not certify PostgreSQL or the built-process proof.

## Start a trusted browser host

Install `mkcert` through your platform's supported package manager first. Installing a CA changes the selected trust stores: do it explicitly, only where you own those stores. CI should use a disposable home/container. Do not disable TLS verification in readiness probes, browsers, captures or clients.

```sh
make auth-host-build
mkdir -p auth/testhost/target
STATE="$(mktemp -d "$PWD/auth/testhost/target/run-XXXXXX")"
export CAROOT="$STATE/ca"
mkcert -install
mkcert -cert-file "$STATE/localhost.pem" -key-file "$STATE/localhost-key.pem" localhost 127.0.0.1 ::1
auth/testhost/target/auth-host -init-fixture "$STATE/fixture.json"
auth/testhost/target/auth-host \
  -origin https://localhost:4443 \
  -cert "$STATE/localhost.pem" -key "$STATE/localhost-key.pem" \
  -fixture "$STATE/fixture.json" -state "$STATE/sessions.db" \
  -run-id browser-worker-1 -build-id "$(git rev-parse HEAD)"
```

The process stays in the foreground. Stop it with SIGTERM or Ctrl-C; the runner must observe its exit. Startup gets 30 seconds; cleanup gets a fresh 10-second budget despite cancellation. A process owner may allow two more seconds for forced settling, but escalation is not graceful success.

For Node readiness tooling, set `NODE_EXTRA_CA_CERTS="$CAROOT/rootCA.pem"` **before** starting Node. For Go clients, load this CA into a scoped certificate pool; for curl use `--cacert "$CAROOT/rootCA.pem"`. Never use `ignoreHTTPSErrors`, `InsecureSkipVerify`, certificate-error browser flags or a global verification bypass.

After stopping every dependent host/browser, use the same `CAROOT` with `mkcert -uninstall` to remove only this run's trust installation. Remove only the resolved run directory you created. Never upload its fixture file, database, browser cookie state or private keys.

Use `-origin https://localhost:0` for an ephemeral listener. The `auth-host-ready` announcement contains its actual origin and non-secret run/build identity. It is a startup signal, **not** readiness proof: probe `GET /_test/ready` with TLS verification, require status 200 and the exact `protocol`, `runId`, `buildId` and `schemaVersion` below. Reject redirects, wrong identities, untrusted certificates and unavailable storage.

Supply `-assets /absolute/browser/build` to serve a browser consumer's build at this same origin through `server/spa`. The index uses the kit's strict nonce CSP and `{nonce}` placeholders; API, event and control paths never fall back to HTML. No fabricated frontend is bundled here.

## Public protocol

The production protocol is owned by [`auth/session`](../README.md). The browser never reads or stores its opaque credential. Its cookie is `__Host-session`, Secure, HttpOnly, SameSite=Strict, Path=/, with no Domain.

| Operation | Contract |
|---|---|
| Sign in | `POST /auth/login`, JSON username/password, exact host `Origin`; the fixture username is `fixture-user`, with its generated password read only from private runner state |
| Session status | `GET /auth/session`; authenticated identity, absolute `expiresAt` and signed `csrfToken`; never renews or sets a cookie |
| Sign out | `POST /auth/logout`, with `X-CSRF-Token`; revokes the session family and cancels local streams before returning 204 |
| Protected Connect | `/gokit.auth.v1.IdentityService/WhoAmI`, `google.protobuf.Empty` → `google.protobuf.StringValue`; returns `user:fixture-user` for both the cookie and API key |
| Protected SSE | `GET /events`; normal kit handshake/control framing and authoritative session lifetime |
| Refresh | None |

Unsafe cookie-authenticated requests, including Connect POSTs, require the signed `X-CSRF-Token`. Header automation uses `X-API-Key` without cookies or CSRF. The fixture's private `apiKey` field is the automation credential; do not combine it with a session cookie. Both callers use the same principal contract. Credential scopes/resources are a ceiling, not membership.

Expired/revoked/invalid sessions are terminal authentication failures. Unavailable storage is an operational failure, never authenticated success. Closing a protected stream requires a bounded, single-flight status check before reconnecting; terminal auth stops reconnect. A late status result cannot restore a client that has logged out. Only login sets the session cookie.

## Isolation and scenario controls

Each worker owns its fixture file, database, process and browser context. **Cookies are not isolated by port:** independent localhost hosts must not share a browser context. Reusing the same state/fixture files on an explicit restart preserves sessions; new files give a fresh environment. Restart does not silently restore deleted sessions.

`-init-fixture` creates a new 0600 file and never overwrites one. It contains synthetic password/API-key/control credentials and independent digest/CSRF keys. Treat it as private ephemeral state, not a shared wire fixture or retained artifact. Diagnostics are bounded by the process owner to 64 KiB per output stream.

| Test-only route | Required result |
|---|---|
| `GET /_test/ready` | 200 with `{"protocol":"gokit.session.v1","runId":"…","buildId":"…","schemaVersion":1}` after actual migration readiness; storage outage is not ready |
| `POST /_test/reset` | Runner `X-Test-Control`; quiescent requests/streams required; transactional fixture reset preserves mounted routes and migration metadata |
| `GET /_test/state` | Non-secret `statusPending` observation for the held-status journey |
| `POST /_test/scenario` | Runner `X-Test-Control`, JSON name `unavailable-store`, `healthy-store`, `expired`, `hold-status` or `release-status`; unknown names fail |

Controls require the private fixture's `controlToken`. Without that capability, they return 404. Reset occurs **before** sign-in, after protected requests/streams are closed; an active request yields a conflict instead of destructive reset. Reset has a five-second budget.

`unavailable-store` injects failure at the store port while retaining the real SQL adapter. It is not a claim that a PostgreSQL server was stopped. `expired` advances the injected domain clock by one hour; transport/shutdown clocks continue to run. Domain time is fixed within a scenario. Reset restores it and clears application fixture data, never schema metadata.

For a late-status journey, select `hold-status`, start one session-status request, require `statusPending: true`, then log out and select `release-status`. The held response reflects its earlier authoritative read and sets no cookie; a new status request must be unauthorized. The browser consumer must fence this late result against its logout generation. The hold buffer is 4 KiB and the hold budget is five seconds.

## Resource and publication boundaries

Sessions have a one-hour absolute lifetime. New authenticated requests always consult authoritative storage. Successful local logout cancels lifetime contexts before returning; the backend check requires stream release within one second. Cross-instance support also requires the session owner's tested fail-closed revalidation lease; shared SQLite files remain single-process only.

Wire fixtures contain shapes and expected failures, never usable credentials. Pin the eventual immutable source revision and fixture digests when another repository adopts the host. A dirty-tree build must be identified by its diff/build digest as well as its base revision. A merged commit or a passing local-linked consumer is not proof of package-registry availability.
