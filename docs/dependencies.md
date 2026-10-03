# Dependency audit

gokit keeps its third-party surface small and permissively licensed. Every non-stdlib dependency is justified here, scanned for known vulnerabilities by `govulncheck` (see [`SECURITY.md`](../SECURITY.md) and `.github/govulncheck-suppressions.json`), and checked against a permissive license allow-list by [`scripts/check-licenses.sh`](../scripts/check-licenses.sh) in CI.

## Policy

- **Least surface.** Core packages (root `go.mod`) stay dependency-light; heavy dependencies live behind their own sub-module `go.mod` so consumers only pull what they use.
- **Permissive and weak-copyleft licenses only.** The license gate accepts Apache-2.0, MIT, BSD-2/3-Clause, ISC, and similarly permissive terms, plus MPL-2.0 (weak, file-level copyleft) and CC0-1.0 (public-domain dedication). Strong copyleft (GPL/LGPL/AGPL) and unknown licenses fail CI. Adding an SPDX id to the allow-list in `scripts/check-licenses.sh` requires a maintainer sign-off recorded here.
- **Maintained.** A direct dependency with no upstream release in over a year must carry a written rationale in this file or be replaced. Deprecated-by-design modules that we do not import are documented as suppressions rather than kept as live dependencies.
- **Audited on entry.** Every new direct dependency is justified in the table below when it is added.

## Direct dependency decisions

These third-party modules live in opt-in sub-modules so the core stays clean.

| Module (sub-module) | Dependency | License | Why |
|---|---|---|---|
| `storage/gcs` | `cloud.google.com/go/storage`, `cloud.google.com/go/auth`, `google.golang.org/api` | Apache-2.0 / BSD-3-Clause | Google's official Cloud Storage SDK — the only supported way to talk to GCS with correct auth, resumable uploads, and retries. |
| `vectorstore/qdrant` | `github.com/google/uuid` | BSD-3-Clause | Point-ID generation for the Qdrant adapter; the adapter itself reuses the first-party `httpclient`, so no vendor client SDK is pulled in. |
| `database/sqlite` | `gorm.io/driver/sqlite`, `gorm.io/gorm` (→ `github.com/mattn/go-sqlite3`) | MIT | SQLite backend for the shared GORM-based `database` layer. `mattn/go-sqlite3` is **CGO** (needs a C toolchain to build); it is isolated in this sub-module so CGO never leaks into the core. |
| root (`util`) | `github.com/zeebo/blake3` | CC0-1.0 | BLAKE3 content hashing (`util.ContentHasher`), the canonical content-identity hash across the toolkit. Licensed under the CC0-1.0 public-domain dedication (more permissive than MIT); allow-listed with maintainer sign-off. |

Dependency-free modules:

- `media` — pure standard library (`image`, `image/jpeg`, `image/png`, …); heavy audio/video/matrix work stays rskit-only, so no codec libraries are vendored.
- `cli` — standard library only (`fmt`, `os`, `flag`-free custom parsing); the CLI is the one place stdout is expected.
- `dataset` — standard library only.

## Version constraints

Go 1.27.1 is the minimum supported toolchain. Dependency updates are candidates, not automatic approvals: build, test, vulnerability, and license gates must pass before adopting them.

`toven vuln` and CI use the same suppression-aware scanner. It scans each selected module independently (`GOWORK=off`) using symbol-level source analysis and fails on unsuppressed imported advisories as well as reachable ones. The wrapper controls the scan level and mode and verifies them in the output, so reduced-detail scans cannot bypass reachable-advisory controls. Malformed or incomplete scanner output fails the gate. Security, licenses, fuzz smoke tests, and integration tests run on pull requests, merge queues, and main; the aggregate gate rejects skipped jobs when Go or CI inputs changed. Temporary Dependabot ignores prevent the known affected versions below from being proposed again; they do not replace the security gate or block future patched releases.

| Dependency | Selected version | Constraint |
|---|---|---|
| `google.golang.org/grpc` | `v1.83.2` | Patched stable release. Newer `v1.84.0` is affected by [GO-2026-6443](https://pkg.go.dev/vuln/GO-2026-6443); do not move to it or an unreleased development fix. |
| gRPC consumers | `grpc-gateway/v2 v2.30.0`, `proto/otlp v1.11.0`, `google.golang.org/api v0.297.0` | Retain versions compatible with the patched gRPC release. Newer versions require the affected line through minimum-version selection. |
| `k8s.io/kube-openapi` | `v0.0.0-20260721132016-d427ff9ee9ad` | The revision required by [Kubernetes apimachinery v0.37.1](https://github.com/kubernetes/apimachinery/blob/v0.37.1/go.mod). Newer development snapshots use structured-merge-diff/v7, incompatible with the released libraries' v6 schema types. |
| `github.com/segmentio/asm` | `v1.1.5` | Latest MIT-licensed patch used by MCP's JSON dependency. [v1.2.1 uses MIT-0](https://github.com/segmentio/asm/blob/v1.2.1/LICENSE), which the current scanner does not classify and the allow-list does not approve. This is a tooling/policy constraint, not a missing upstream license; adoption needs scanner support and maintainer approval. |

Validation tools are pinned in CI: golangci-lint v2.14.0, govulncheck v1.8.0, and go-licenses/v2 v2.0.1. Build local copies with Go 1.27.1 or newer so the tools can analyze the repository's language version.

## Known suppressions

`GO-2026-5932` (`golang.org/x/crypto/openpgp`) is suppressed across all modules: the flagged package is deprecated-by-design and never imported by gokit, so it is not reachable. See `.github/govulncheck-suppressions.json` for the full rationale and expiry.
