# Dependency audit

gokit keeps its third-party surface small and permissively licensed.
Every non-stdlib dependency is justified here,
scanned for known vulnerabilities by `govulncheck` (see [`SECURITY.md`](../SECURITY.md) and `.github/govulncheck-suppressions.json`),
and checked against a permissive license allow-list by [`scripts/check-licenses.sh`](../scripts/check-licenses.sh) in CI.

## Policy

- **Least surface.** Core packages (root `go.mod`) stay dependency-light;
  heavy dependencies live behind their own sub-module `go.mod` so consumers only pull what they use.
- **Permissive and weak-copyleft licenses only.** The license gate accepts Apache-2.0, MIT,
  BSD-2/3-Clause, ISC, and similarly permissive terms, plus MPL-2.0 (weak, file-level copyleft).
  Strong copyleft (GPL/LGPL/AGPL) and unknown licenses fail CI.
  Adding an SPDX id to the allow-list in `scripts/check-licenses.sh` requires a maintainer sign-off recorded here.
- **Maintained.** A direct dependency with no upstream release in over a year must carry a written rationale in this file
  or be replaced.
  Deprecated-by-design modules that we do not import are documented as suppressions rather than kept as live dependencies.
- **Audited on entry.** Every new direct dependency is justified in the table below when it is added.

## New direct dependencies from the parity work

These are the third-party modules introduced while bringing gokit to rskit parity.
Each lives in an opt-in sub-module so the core stays clean.

| Module (sub-module) | Dependency | License | Why |
|---|---|---|---|
| `storage/gcs` | `cloud.google.com/go/storage`, `cloud.google.com/go/auth`, `google.golang.org/api` | Apache-2.0 / BSD-3-Clause | Google's official Cloud Storage SDK — the only supported way to talk to GCS with correct auth, resumable uploads, and retries. |
| `vectorstore/qdrant` | `github.com/google/uuid` | BSD-3-Clause | Point-ID generation for the Qdrant adapter; the adapter itself reuses the first-party `httpclient`, so no vendor client SDK is pulled in. |
| `database/sqlite` | `gorm.io/driver/sqlite`, `gorm.io/gorm` (→ `github.com/mattn/go-sqlite3`) | MIT | SQLite backend for the shared GORM-based `database` layer. `mattn/go-sqlite3` is **CGO** (needs a C toolchain to build); it is isolated in this sub-module so CGO never leaks into the core. |

Modules added during the same parity work that introduced **no** new third-party dependency,
by design:

- `media` — pure standard library (`image`, `image/jpeg`, `image/png`, …);
  heavy audio/video/matrix work stays rskit-only, so no codec libraries are vendored.
- `cli` — standard library only (`fmt`, `os`, `flag`-free custom parsing);
  the CLI is the one place stdout is expected.
- `dataset` — standard library only.

## Known suppressions

`GO-2026-5932` (`golang.org/x/crypto/openpgp`) is suppressed across all modules:
the flagged package is deprecated-by-design and never imported by gokit, so it is not reachable.
See `.github/govulncheck-suppressions.json` for the full rationale and expiry.
