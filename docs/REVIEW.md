# OSS Lifecycle & Engineering Review — `kbukum/gokit`

> Date: 2026-04-25 · Reviewer: GitHub Copilot CLI (Claude Opus 4.7)
> Scope: full repository at HEAD (`main`), all 34 modules, all CI/docs/release artifacts.
> Ground rules: pre-stable; backward compatibility **not required**; redesigns preferred over patches; intentionally critical.
>
> **Per-dimension deep-dives** (with full evidence) live in:
> - `gokit-dim1-code-arch-concurrency.md` (34 findings)
> - `gokit-dim2-security-errors-obs.md` (33 findings)
> - `gokit-dim3-testing-perf-lint.md` (22 findings)
> - `gokit-dim4-ci-toolchain-docs-release-hygiene.md` (52 findings)
>
> Tooling baseline log: `tooling-gokit.log` (+ `.lint`, `.tests`, `.vuln`, `.gosec` siblings).

---

## 1. Executive Summary

1. **One Critical, sixteen High** findings block any path to v1.0 today. The two largest classes are **supply-chain hygiene** (Go 1.26.0 with 8 reachable stdlib CVEs; no SHA pinning; no SBOM/cosign/SLSA; `securego/gosec@master` in CI) and **architectural debt that will cement bugs after v1.0** (untyped DI god-object, 6 inconsistent registry implementations, layering inversion `worker → sse`, no `depguard`).
2. **Toolchain drift in disguise.** Every `go.mod` declares `go 1.26.0` with no `toolchain` directive — CI happily builds on the patched-but-still-vulnerable compiler. Bumping to `go 1.26.2` + `toolchain go1.26.2` is a one-line, repo-wide critical fix.
3. **Concurrency correctness is the silent v1.0 blocker.** Three goroutine-leak patterns are reachable today (`provider.MergeIterators`, `agent.Stream`, `sse.Hub.Broadcast`), all stemming from sends without `ctx.Done()` guards. None are caught by tests because shuffle is on but the failing test (logger) masks them.
4. **Auth surface has multiple high/medium issues** — alg-confusion risk in OIDC verifier, no clock skew, no nonce check, no JWKS single-flight (DoS amplifier), opt-in query-token middleware never strips token from logs, JWT HMAC secret length unchecked, "missing vs invalid credential" semantically conflated, raw `gin.H` errors instead of ProblemDetail.
5. **Lint configuration is permissive enough that real bugs already pass `lint exit=0` in CI** — `provider/streaming.go:184` shadowed `err`, `validation/struct_validator.go:44` errorlint type-assertion. The test-file blanket exclusion of 7 linters (errcheck, gosec, noctx, unparam, unused, gocritic, errorlint) is too broad and hides errcheck lapses in helpers.
6. **Testing & fuzz are anemic for the attack surface.** Five fuzz targets total; **no fuzz** for JWT verify, JWKS parse, OIDC, API key parse, RFC 7807 problem JSON, schema decode, MCP, config decode. `auth/oidc` 13.1% coverage. `server/endpoint` 0%. 252 test files, 0 with `//go:build integration`. 101 production `time.Now()` calls, 0 `Clock` interface.
7. **Performance discipline is absent.** Five benchmarks for the entire kit. Zero `sync.Pool`. No `net/http/pprof` hook. No `benchstat` regression gate. The `bench/` directory is a model-eval harness (precision/recall/ROC), not a perf harness — naming is misleading. `server/middleware/tracing.go:28` does `fmt.Sprintf` per HTTP request.
8. **Release hygiene is broken end-to-end.** 23 git tags exist (`v0.2.0`, `<module>/v0.2.0`, …) but `gh release list` returns "no releases found". CHANGELOG ends at `[0.1.5]` — every breaking change since lives in `[Unreleased]`. No `.goreleaser.yml`, no cosign signing, no SLSA provenance, no SBOM. `tag-modules.sh` lockstep-tags every module to the same version, defeating multi-module SemVer benefits.
9. **Dependabot config has invalid keys** — `group-by: dependency-name` is silently ignored. Recently consolidated `directories: ["/", "/*", "/*/*"]` is good, but the grouping logic doesn't actually group cross-module bumps. No `golang.org/x/*` group, no `cooldown:`, no security-only fast lane.
10. **Bus factor = 1.** `MAINTAINERS.md` and `CODEOWNERS` both name only `@kbukum`. Pre-1.0 this is acceptable; for v1.0 stabilization it is a release blocker. Combined with no pre-commit hooks, no `actionlint`/`zizmor`/`gitleaks` workflows, no ADRs, no SemVer/deprecation policy, the repo is not yet "lifecycle-ready" for external adoption at scale.

---

## 2. Severity-Ordered Findings

### 2.1 Findings Table (id · severity · category · location)

| ID | Sev | Category | Evidence | Effort | Sibling? |
|----|-----|----------|----------|--------|----------|
| **F-001** | **Critical** | Toolchain | `go.mod:3` + 33 sub-modules + `go.work:1` — `go 1.26.0` w/ 8 reachable stdlib CVEs (`tooling-gokit.log.vuln`) | S | Y |
| F-002 | High | Security | `auth/oidc/verifier.go:95-112` — accepts header `alg` w/o binding to JWKS `jwk.alg` (alg-confusion risk) | M | Y |
| F-003 | High | Security | `.github/workflows/ci.yml:175-178` — `securego/gosec@master`; mutable supply-chain dep | S | Y |
| F-004 | High | CI/CD | `.github/workflows/ci.yml:35,51,68,89,109,135,160,170,179,189` — 0/10 actions pinned by SHA | S | Y |
| F-005 | High | Security | `.golangci.yml:64-74` — gosec excludes `G402,G404,G306` globally — neutralises annotations the project relies on | S | Y |
| F-006 | High | CI/CD | `.github/workflows/ci.yml:165-168` — `govulncheck@latest`; non-reproducible; vuln findings don't fail the build | S | Y |
| F-007 | High | CI/CD | (no file) — no release / CodeQL / SBOM / cosign / SLSA workflows | L | Y |
| F-008 | High | Concurrency | `provider/streaming.go:71` — `MergeIterators` sends to `errCh` w/o `ctx.Done()` guard; goroutine leak on cancellation | S | Y |
| F-009 | High | Concurrency | `agent/agent.go:191-260` — `Stream` event sends w/o `ctx.Done()` guard | S | Y |
| F-010 | High | Concurrency | `sse/hub.go` (`Broadcast`) — unguarded blocking send to subscriber channels | M | Y |
| F-011 | High | Concurrency | `logger/logger.go:39` + `logger_test.go:556,583` — `zerolog.SetGlobalLevel` mutated globally; root cause of `TestJSONFormat_AllLevelsHaveCorrectLevelField` shuffle flake | S | Y |
| F-012 | High | Architecture | `worker/sse_bridge.go:9` — layering inversion: `worker` (transport-agnostic) imports `sse` (transport) | M | Y |
| F-013 | High | Architecture | `di/container.go:29-46,304-336` — DI is stringly-typed `interface{}` god-object; `Resolve[T]` is a thin reflect wrapper | XL | Y |
| F-014 | High | Architecture | `.golangci.yml` — no `depguard`; nothing prevents the layering rot in F-012 from spreading | S | Y |
| F-015 | High | Architecture | `auth/registry.go`, `discovery/component.go`, `storage/factory.go`, `tool/registry.go`, `workload/factory.go`, `llm/registry.go` — 6 duplicate registry implementations with **inconsistent** semantics (some panic on duplicate, some return error, `auth` silently overwrites) | L | Y |
| F-016 | High | Code Quality | `di/resolve.go:7,15`, `auth/authctx/context.go:49,52`, `auth/registry.go:59`, `discovery/component.go:34-87`, `server/middleware/tenant.go:61,64`, `agent/prompt.go:74,77,168`, `tool/registry.go:41`, `storage/factory.go:30-39`, `workload/factory.go:57` — 9+ `Must*`/panic helpers in lib code, each documented "do not call from request scope" — design smell | M | Y |
| F-017 | High | Testing | `server/endpoint` 0%, `auth/oidc` 13.1%, `auth/apikey` 17.9%, `server/middleware` 57.2%, `grpc/resolver/discovery` 0% (coverage profile) — security-critical packages well below the 80%/85% gate the prompt requires | L | Y |
| F-018 | High | Testing | `grep -rn "^func Fuzz"` returns 5 targets; no fuzz for JWT verify, JWKS parse, OIDC verifier, API key parse, problem+JSON, MCP, schema, config decode | M | Y |
| F-019 | High | Testing | 0 `//go:build integration` tags across 252 test files; `integration_test.go` (913 lines, root) runs in every fast CI step | S | Y |
| F-020 | High | Performance | Only 5 `Benchmark*` total; missing DI resolve, validation, middleware chain, DAG engine, chain executor, codecs, JWKS, registries | L | Y |
| F-021 | High | Performance | (no file) — no `benchstat` regression gate in CI | M | Y |
| F-022 | High | Lint | `.golangci.yml:31-39` — `_test\.go` blanket-excludes 7 linters (errcheck/gosec/noctx/unparam/unused/gocritic/errorlint); too broad | S | Y |
| F-023 | High | Lint | Missing critical linters: `paralleltest`, `tparallel`, `testifylint`, `contextcheck`, `nilnil`, `nestif`, `revive`, `depguard`, `tagliatelle`, `gofumpt` | S | Y |
| F-024 | High | Lint | `provider/streaming.go:184`, `provider/interaction_test.go:199`, `sse/hub_test.go:593` — `govet shadow` finds real bugs already; CI returns `lint exit=0` because the rule is enabled but findings aren't gating | S | Y |
| F-025 | High | Release | (no file) — no `.goreleaser.yml`, no cosign config, no SLSA provenance | L | Y |
| F-026 | High | Release | `gh release list -R kbukum/gokit -L 5` → empty, despite 23 git tags incl. `v0.2.0` | S | Y |
| F-027 | High | Release | `CHANGELOG.md:168` — latest entry `[0.1.5] - 2026-03-01`; tag `v0.2.0` exists with no CHANGELOG entry | S | Y |
| F-028 | High | Release | (no file) — no SemVer / deprecation policy doc; GOVERNANCE silent on v1.x rules | M | Y |
| F-029 | High | Hygiene | `MAINTAINERS.md:7` + `.github/CODEOWNERS` — bus factor 1; code-owner review cannot be a meaningful gate | M | Y |
| F-030 | High | Observability | (no file) — no HTTP `/healthz`/`/readyz` handler shipped despite full `Health` taxonomy in `observability/` | M | Y |
| F-031 | Med | Errors | `errors/response.go:14-16` — `init()` mutates package-global URI base | S | Y |
| F-032 | Med | Errors | `server/middleware/auth.go:90,96,128` — emits `gin.H{"error":...}` instead of ProblemDetail | S | Y |
| F-033 | Med | Errors | `server/middleware/auth.go:90,96,128` — missing-vs-invalid credentials conflated; no `WWW-Authenticate` header | S | Y |
| F-034 | Med | Errors | `errors/errors.go` (`Wrap`) — collapses `context.DeadlineExceeded`, `sql.ErrNoRows`, etc. to `INTERNAL_ERROR` 500 | M | Y |
| F-035 | Med | Errors | `errors/errors.go` (`AppError.Error`) — flattens `cause` into `err.Error()` → PII leak through `slog`/JSON encoders | S | Y |
| F-036 | Med | Security | `auth/oidc/verifier.go` — no clock-skew leeway, no `nbf` check, no `iat` sanity | S | Y |
| F-037 | Med | Security | `auth/oidc/verifier.go` — parses `nonce` claim but never validates against expected | S | Y |
| F-038 | Med | Security | `auth/oidc/jwks.go` — no single-flight on cache miss; full unauthenticated re-fetch on unknown `kid` (DoS amplifier) | S | Y |
| F-039 | Med | Security | `server/middleware/auth.go` (`WithQueryTokenParam`) — token leaks into access logs / Referer / spans even with allow-list (no URL stripping after consumption) | S | Y |
| F-040 | Med | Security | `security/tls.go` (`TLSConfig.hasSettings`) — excludes `MinVersion`; setting only `min_version` returns nil config silently | S | Y |
| F-041 | Med | Security | `auth/jwt/config.go` — HMAC `Secret` length unchecked (RFC 8725 §3.2 violation) | S | Y |
| F-042 | Med | Security | `bootstrap/summary.go:211` — writes ANSI control codes unconditionally to a configurable writer (also OB-03) | S | Y |
| F-043 | Med | Observability | `observability/tracer.go`, `observability/meter.go` (`InitTracer`/`InitMeter`) — mutate otel globals, second call races; not idempotent | M | Y |
| F-044 | Med | Observability | `observability/span.go` (`SetSpanAttribute`) — silently drops unsupported types | S | Y |
| F-045 | Med | Observability | `observability/tracer.go` — exporter init/shutdown ignores caller `ctx` | S | Y |
| F-046 | Med | Testing | ~30% table-driven adoption — many serial `t.Run` blocks instead of `[]struct` | M | Y |
| F-047 | Med | Performance | `server/middleware/tracing.go:28` — `fmt.Sprintf("%s %s", method, path)` per HTTP request | S | Y |
| F-048 | Med | Performance | Zero `sync.Pool` in production code | M | Y |
| F-049 | Med | Performance | No `net/http/pprof` integration hook | S | Y |
| F-050 | Med | Lint | Standalone `gosec` job duplicates golangci-lint's gosec; ignores `//nolint:gosec` markers (3 G101 false positives) | S | Y |
| F-051 | Med | Lint | CI pins `golangci-lint v2.0.0`; baseline ran v2.9.0 — version drift between dev and CI | S | Y |
| F-052 | Med | Lint | `depguard` absent — module dependency drift unchecked | S | Y |
| F-053 | Med | CI/CD | `.github/workflows/ci.yml:104-119` — degraded matrix; only single Go version, no `linux/arm64` runner, only 4 cross-OS includes for 3 of 34 modules | M | Y |
| F-054 | Med | CI/CD | `.github/workflows/ci.yml:140-148` — codecov upload not gated; `fail_ci_if_error: false` | S | Y |
| F-055 | Med | CI/CD | `.github/workflows/ci.yml:130-134` — Windows test job runs without `-race` (undocumented) | S | Y |
| F-056 | Med | CI/CD | `.github/workflows/ci.yml:158-168` — `vuln` job has no `permissions:` block | S | Y |
| F-057 | Med | CI/CD | `.github/workflows/ci.yml:18-23` — comment about "actions … will be upgraded to commit SHAs by Dependabot" is aspirational; not configured | S | Y |
| F-058 | Med | Toolchain | `.github/dependabot.yml:43-65` — `group-by: dependency-name` is **not a valid Dependabot key** — silently dropped | S | Y |
| F-059 | Med | Toolchain | `.github/dependabot.yml` — no `golang.org/x/*` group, no `cooldown:`, no security-only fast lane | S | Y |
| F-060 | Med | Toolchain | repo layout — 34-module split (8 are `*/testutil`) not justified in any doc; largest source of `go mod tidy` cost and dependabot fan-out | M | Y |
| F-061 | Med | Toolchain | `go.work:1` — declares `go 1.26.0`; no separate `toolchain` directive | S | Y |
| F-062 | Med | Documentation | `media/` — only top-level package missing `doc.go` (44/45 covered) | XS | Y |
| F-063 | Med | Documentation | 8 `example*_test.go` for ~45 packages — thin runnable-example coverage on agent/tool/pipeline/dag/provider/worker/mcp/llm | L | Y |
| F-064 | Med | Documentation | `docs/` — no `docs/adr/`; breaking changes lack rationale documents | M | Y |
| F-065 | Med | Documentation | `README.md` — no coverage/vuln/SLSA badges; no quickstart runnable snippet | S | Y |
| F-066 | Med | Documentation | `MAINTAINERS.md:31-32` — points to `docs/release-process.md` "(when present)" — file does not exist | S | Y |
| F-067 | Med | Release | `tag-modules.sh:1-40+` — lockstep-tags every module to the same version; defeats multi-module SemVer | M | Y |
| F-068 | Med | Release | `tag-modules.sh` — no clean-tree check, no CHANGELOG-non-empty check, no signed tags (`-s`), no release-notes generation | M | Y |
| F-069 | Med | Release | `CHANGELOG.md` — all `[Unreleased]` items are large breaking changes since `0.1.5`; should already be cut as `0.2.0` | S | Y |
| F-070 | Med | Hygiene | (no file) — no `.pre-commit-config.yaml` | S | Y |
| F-071 | Med | Hygiene | (no file) — no `actionlint` / `zizmor` workflow | S | Y |
| F-072 | Med | Hygiene | (no file) — no `gitleaks` / explicit secret-scan workflow | S | Y |
| F-073 | Med | Hygiene | branch protection — out of file scope; verify `CI Status` aggregate gate, signed commits, linear history are required | S | Y |
| F-074 | Med | Code Quality | `errors/errors.go` — `%v`-flattening of error slices instead of `errors.Join` | S | Y |
| F-075 | Med | Code Quality | `bootstrap/app.go`, `component/registry.go` — hardcoded shutdown deadlines clobber caller ctx | M | Y |
| F-076 | Med | Concurrency | `worker/keyedpool.go` (`SubmitOrAttach`) — holds lock through `pool.Submit` | M | Y |
| F-077 | Med | Concurrency | `messaging/.../consumer.go` (`ManagedConsumer.Stop`) — takes no ctx; clamps to 10 s | S | Y |
| F-078 | Med | Architecture | `component/registry.go` (`StartAll`) — holds write lock for entire boot sequence | M | Y |
| F-079 | Low | Lint | `.golangci.yml:42-44` — bootstrap-summary errcheck exclusion is too coarse | XS | Y |
| F-080 | Low | Lint | (config) — no `gofumpt` | XS | Y |
| F-081 | Low | Performance | `bench/` directory is a model-eval harness, not a perf harness — misleading name | M | Y |
| F-082 | Low | Testing | `worker.test` 18MB stale local artifact (gitignored, not committed); needs `make clean` target | XS | Y |
| F-083 | Low | Errors | `validation/struct_validator.go:44` — type-asserts on a wrapped error (caught by errorlint, not fixed) | XS | Y |
| F-084 | Low | Errors | (config) — no exported sentinel errors from low-level packages | M | Y |
| F-085 | Low | Errors | panic in lib code beyond `MustGet` (factories) | M | Y |
| F-086 | Low | Security | `auth/registry.go:59` — `MustGet` panics in library code | S | Y |
| F-087 | Low | Security | `auth/oidc/providers/apple.go` — Apple secret signer ignores `json.Marshal` errors | S | Y |
| F-088 | Low | Documentation | `CONTRIBUTING.md:5,98,103` — says Go 1.25.0; reality is Go 1.26.0 everywhere (TC-08 sibling) | XS | Y |
| F-089 | Low | Documentation | `README.md` — no Table of Contents, no architecture diagram | M | Y |
| F-090 | Low | Documentation | `README.md:24-26` — Compatibility Matrix has only one stale row (`1.25+ → v0.1.2+`) | XS | Y |
| F-091 | Low | Documentation | `docs/VERSIONING-QUICK-FIX.md` — should be merged into `VERSIONING.md` or deleted | XS | Y |
| F-092 | Low | CI/CD | `.github/workflows/ci.yml:97-101` — `check` matrix `include:` hard-codes `storage` and `server` for macOS only — brittle list | S | Y |
| F-093 | Low | CI/CD | `.github/workflows/ci.yml:83-84` — `chmod +x ./gomod.sh` in CI — script should be checked-in executable | XS | Y |
| F-094 | Low | CI/CD | `.github/workflows/ci.yml:175-181` — gosec runs over the whole tree from root with `-exclude` flags; missing module-local context (re-creates G101 false positives) | S | Y |
| F-095 | Low | CI/CD | (no file) — no fuzz corpus persistence (`actions/cache` for `testdata/fuzz/`) | S | Y |
| F-096 | Low | CI/CD | `.github/workflows/ci.yml:185-225` — Fuzz job hard-codes the four targets it knows about; new `FuzzXxx` silently skipped | S | Y |
| F-097 | Low | Toolchain | `go.sum` (root) is 11.7 KB, `go.work.sum` is 18.6 KB — `// indirect` audit not in scope (informational) | M | Y |
| F-098 | Low | Release | `kafka/v0.2.0` and `kafka/testutil/v0.2.0` tags exist but kafka provider lives under `messaging/kafka/` (no module there) — orphan tag or renamed module | S | Y |
| F-099 | Low | Release | `Makefile` — verify `make tag VERSION=…` and `make tag-push VERSION=…` targets stay aligned with `tag-modules.sh` | XS | Y |
| F-100 | Low | Hygiene | `.github/ISSUE_TEMPLATE/` — no `config.yml` to disable blank issues / link Discussions / SECURITY.md | XS | Y |
| F-101 | Low | Hygiene | `CODEOWNERS` — `chain/`, `embedding/`, `explain/`, `hook/`, `mcp/`, `media/`, `schema/`, `sse/`, `stateful/`, `tool/`, `vectorstore/` not listed (silently fall through to `* @kbukum`) | S | Y |
| F-102 | Low | Hygiene | `.gitignore:31` — `docs/testutil*.md` carve-out unexplained | XS | Y |
| F-103 | Low | Hygiene | `.gitignore` — missing `*.prof`, `*.cover`, `*.coverprofile`, `*.fuzz`, `__debug_bin*`, `.cache/`, `.envrc`, `.direnv/` | XS | Y |
| F-104 | Low | Code Quality | `worker/`, `pipeline/` — error-comparison anti-patterns: `!= context.Canceled` instead of `errors.Is` (3 sites) | XS | Y |
| F-105 | Low | Code Quality | `chain/`, `worker/` — `cancelled`/`behaviour`/`favour` US-locale misspellings (9 hits per `tooling-gokit.log.lint`) | XS | Y |
| F-106 | Low | Concurrency | `worker/`, `bootstrap/` — `time.Sleep` ignoring ctx in 2 sites (CC-04) | S | Y |
| F-107 | Low | Concurrency | `provider/`, `agent/` — fragile select-loop logic (CC-12) | S | Y |
| F-108 | Low | Observability | `OB-07` — no span around OIDC discovery / JWKS fetch | XS | Y |
| F-109 | Low | Observability | `errors/response.go` (`RespondWithError`) — does not call `SetSpanError` / mark span as errored | XS | Y |
| F-110 | Low | Observability | `observability/health.go` — `Health` type lacks timestamp / latency fields | XS | Y |
| F-111 | Nit | Code Quality | small inconsistencies (`ProblemDetail` field tagging, error string punctuation) — see dim1 §1 | XS | Y |

> "Sibling?" column: every finding is treated as **likely applicable** to `rskit` and `pykit` until proven otherwise during their own reviews. Cross-sibling parity matrix lives in `cross-sibling-matrix.md` (Phase D).

### 2.2 Per-finding detail blocks (Critical and High only inline)

Detailed Med/Low/Nit blocks live in the four dimension reports; only the inline IDs map back here.

#### F-001 — Critical · Toolchain
- **Evidence**: `go.mod:3` (`go 1.26.0`) and 33 sibling `go.mod` files; `go.work:1`. `tooling-gokit.log.vuln` reports 8 reachable stdlib CVEs incl. `GO-2026-4866` (case-sensitive `excludedSubtrees` Auth Bypass in `crypto/x509`), `GO-2026-4870` (TLS DoS), `GO-2026-4865` (XSS in `html/template`), `GO-2026-4601` (IPv6 host literal parsing in `net/url`), `GO-2026-4600`/`4599` (`crypto/x509` panics / email constraint mis-enforcement). Reachable via `media/media.go:88`, `bootstrap/summary.go:211`, `logger/logger.go:305`, `errors/errors.go:256`, `observability/meter.go:55`, `validation/struct_validator.go:39`, `resilience/degradation.go:82`.
- **Impact**: cert-validation auth bypass and TLS DoS in any deployed binary built from this code today.
- **Recommendation**: bump every `go.mod` and `go.work` to `go 1.26.0` + `toolchain go1.26.2`. Add `tools/check-toolchain.sh` step to CI that diffs Go directive across all manifests.
- **Effort**: S.
- **Sibling**: Y (Rust/Python toolchain currency must be re-verified per repo).

#### F-002 — High · Security · OIDC alg confusion
- **Evidence**: `auth/oidc/verifier.go:95-112` — token's header `alg` is used to select verification path without comparing against the JWKS key's `jwk.alg`. Today only RSA/EC keys are wired, but introducing HMAC anywhere in the kit would enable a header-driven downgrade.
- **Impact**: alg-confusion / key-confusion CVE class. Prevents safe addition of HMAC-keyed providers.
- **Recommendation**: require `header.alg == jwk.alg`; reject `none`; reject any `alg` not in a per-issuer allow-list configured at verifier construction time.
- **Effort**: S.
- **Sibling**: Y.

#### F-003 — High · Security · `securego/gosec@master`
- **Evidence**: `.github/workflows/ci.yml:175-178`. Mutable branch reference; future malicious commit goes straight into PR check runs.
- **Impact**: supply-chain compromise of the security gate itself.
- **Recommendation**: pin to a SHA matching a tagged release; combine with F-004 sweep.
- **Effort**: S.
- **Sibling**: Y.

#### F-004 — High · CI/CD · No SHA pinning
- **Evidence**: `.github/workflows/ci.yml` lines 35,51,68,89,109,135,160,170,179,189 — all 10 `uses:` are `@vN` or `@master`. Contradicts `SECURITY.md` claim about pinning.
- **Impact**: any compromised maintainer of `actions/checkout`, `actions/setup-go`, `golangci/golangci-lint-action`, `codecov/codecov-action`, `securego/gosec` can execute arbitrary code in CI with the workflow's `GITHUB_TOKEN`.
- **Recommendation**: pin every action by 40-char SHA; add `tools/zizmor.sh` (or run `zizmor` in CI) to enforce it.
- **Effort**: S.
- **Sibling**: Y.

#### F-005 — High · Security · gosec excludes neutralise opt-in posture
- **Evidence**: `.golangci.yml:64-74` excludes `G115,G404,G306,G304,G117,G704`. The intent comment says "G402 (TLS InsecureSkipVerify) is intentionally NOT excluded — per-site `//nolint:gosec` documents opt-in." But the **standalone gosec** in CI (F-050) does not honour `//nolint:gosec` (golangci-style). Net effect: the project's documented opt-in safety net for `InsecureSkipVerify`, weak-rand, file-perm 0644 is not enforced by either tool consistently.
- **Impact**: silent weakening of the auth/TLS posture (the very thing the review headlines).
- **Recommendation**: drop the standalone gosec job; rely on golangci-lint's gosec with the existing exclude list; add a `gosec.toml` to honour `// #nosec` markers for the few legitimate sites.
- **Effort**: S.
- **Sibling**: Y (analogous: clippy/ruff opt-in exclusions consistent across local & CI).

#### F-006 — High · CI/CD · `govulncheck@latest`
- **Evidence**: `.github/workflows/ci.yml:165-168` — `go install golang.org/x/vuln/cmd/govulncheck@latest`; result not used to fail the build.
- **Impact**: unreproducible runs; vuln gate is decorative.
- **Recommendation**: pin `govulncheck` version; output SARIF; upload to code-scanning; fail on `>= medium`.
- **Effort**: S.
- **Sibling**: Y (`cargo audit`, `pip-audit`).

#### F-007 — High · CI/CD · No release / CodeQL / SBOM / cosign / SLSA workflows
- **Evidence**: `.github/workflows/` contains only `ci.yml`.
- **Impact**: no signed releases; no SAST beyond gosec; no provenance; downstream consumers can't verify integrity.
- **Recommendation**: add `release.yml` (GoReleaser library mode + cosign keyless + syft SBOM), `codeql.yml` (Go), `vuln.yml` (scheduled govulncheck), `actionlint.yml`. See §5 below.
- **Effort**: L.
- **Sibling**: Y.

#### F-008/F-009/F-010 — High · Concurrency · goroutine leaks on cancellation
- **Evidence**: `provider/streaming.go:71` (`MergeIterators`), `agent/agent.go:191-260` (`Stream`), `sse/hub.go` (`Broadcast`) all do unguarded `ch <- v` style sends. Receivers may have already returned on `ctx.Done()`; senders block forever.
- **Impact**: production goroutine leaks under load; cascades into FD/memory exhaustion.
- **Recommendation**: every send becomes `select { case ch <- v: case <-ctx.Done(): return }`. Centralise in a `runtime.Runner` helper (see Redesign §4).
- **Effort**: S each.
- **Sibling**: Y (Rust `tokio::sync::mpsc` shutdown discipline; Python `asyncio.Queue` cancellation).

#### F-011 — High · Concurrency · logger global mutation root cause
- **Evidence**: `logger/logger.go:39` calls `zerolog.SetGlobalLevel(...)`; tests at `logger_test.go:556,583` rely on it. Under `-shuffle=on`, the "trace" subtest's Debug write is dropped → empty buffer → `TestJSONFormat_AllLevelsHaveCorrectLevelField/debug` JSON parse fails. Already reproduced in `tooling-gokit.log.tests`.
- **Impact**: every consumer of the kit shares the global zerolog level; concurrent test/runtime config races; CI flake hides real signal.
- **Recommendation**: drop global level mutation; use per-instance level on `zerolog.Logger`; the `resetGlobalForTest()` helper added previously becomes obsolete.
- **Effort**: S.
- **Sibling**: Y.

#### F-012 — High · Architecture · `worker → sse` layering inversion
- **Evidence**: `worker/sse_bridge.go:9` imports `github.com/kbukum/gokit/sse`. `worker` is documented as transport-agnostic.
- **Impact**: any consumer of `worker` pulls in HTTP/SSE deps; future renames of `sse` cascade.
- **Recommendation**: move bridge to `sse/worker_bridge.go` or a new `bridges/` package; enforce with `depguard`. See F-014.
- **Effort**: M.
- **Sibling**: Y.

#### F-013 — High · Architecture · DI is stringly-typed god-object
- **Evidence**: `di/container.go:29-46` (`Get`, `MustGet` return `interface{}`), `:304-336` (reflect-driven storage). The generic `Resolve[T]` is a thin wrapper that does an unchecked type assertion.
- **Impact**: every consumer of DI has runtime type panics latent at boundary; static analysis finds nothing.
- **Recommendation**: replace with type-parameterised provider/scope API (sketch in §4). Breaking change is welcome.
- **Effort**: XL.
- **Sibling**: Y (parallel rework in pykit/rskit).

#### F-014 — High · Architecture · No `depguard`
- **Evidence**: `.golangci.yml` enables many linters but not `depguard`.
- **Impact**: F-012 will recur; tier-violations between `core` and `transport` packages have no automated gate.
- **Recommendation**: add `depguard` with three-tier rule set (foundation / transport / integration). Sketch in §6.
- **Effort**: S.

#### F-015 — High · Architecture · 6 inconsistent registry implementations
- **Evidence**: `auth/registry.go` (panics on missing, **silently overwrites duplicates**), `discovery/component.go` (panics on duplicate AND missing), `storage/factory.go` (panics on duplicate), `tool/registry.go` (`panic(err)` from `Register`), `workload/factory.go:57` (panics), `llm/registry.go` (returns error). Five different policies for the same concept.
- **Impact**: integration developers cannot reason about what `Register` will do; corrupts plugin behaviour across the kit.
- **Recommendation**: extract `internal/registry/registry.go` with a single typed implementation; every domain registry wraps it. Standardise on **return error on duplicate**, **return error on missing**, no panics. Sketch in §4.
- **Effort**: L.
- **Sibling**: Y.

#### F-016 — High · Code Quality · 9+ `Must*` panic helpers
- **Evidence**: see Findings Table row for full file list.
- **Impact**: encourages copy-paste of panicking patterns into request paths.
- **Recommendation**: replace `MustResolve[T]`, `MustGet[T]`, etc. with two-return-value variants; reserve `Must*` for `init()` / test / CLI; document non-use with `revive` rule.
- **Effort**: M.

#### F-017 — High · Testing · coverage holes in security-critical packages
- **Evidence**: per-package coverage from baseline run: `auth/oidc 13.1%`, `auth/apikey 17.9%`, `server 56.0%`, `server/middleware 57.2%`, `server/endpoint 0%`, `grpc/resolver/discovery 0%`. Spec target ≥80% per package, ≥85% overall.
- **Impact**: the two packages where bugs map to CVEs are the least covered.
- **Recommendation**: per-package Codecov floors; add table-driven + fuzz coverage to `auth/oidc` (verifier, JWKS cache, providers).
- **Effort**: L.

#### F-018 — High · Testing · fuzz coverage anemic
- **Evidence**: `grep -rn "^func Fuzz" --include="*.go"` returns 5: `server/middleware/auth_fuzz_test.go:FuzzExtractToken` and 4 others. No fuzz in `auth/jwt`, `auth/oidc`, `auth/apikey`, `errors` (problem JSON), `schema`, `mcp`, `config`.
- **Impact**: parser-level CVEs are the easiest class to catch with fuzz; these packages are unprotected.
- **Recommendation**: add `Fuzz*` tests for: JWT parse+verify, JWKS JSON parse, OIDC discovery JSON, API key parse, problem JSON encode/decode, schema JSON-Schema validate, MCP framing, config TOML/YAML decode. Add corpus persistence in CI (F-095).
- **Effort**: M.

#### F-019 — High · Testing · no integration tag gating
- **Evidence**: `grep -rln "//go:build integration"` returns 0; `integration_test.go` (913 lines, root) runs in every fast CI step.
- **Impact**: 913-line integration suite slows every PR; brittle external deps fail unrelated PRs.
- **Recommendation**: tag all integration tests with `//go:build integration`; gate behind a separate workflow / Make target; default `go test ./...` skips them.
- **Effort**: S.

#### F-020/F-021 — High · Performance · benchmark gap
- **Evidence**: `grep -rn "^func Benchmark" --include="*.go" | wc -l` → 5. No `benchstat` step in CI.
- **Impact**: regressions land silently; v1.0 with no perf SLO.
- **Recommendation**: bench harness in §6; `benchstat` job comparing PR vs base; threshold breakdown.
- **Effort**: M (harness) + L (per-package coverage).

#### F-022/F-023/F-024 — High · Lint
- **Evidence**: `.golangci.yml:31-39` for blanket exclusion; missing linter list in §6; `lint exit=0` despite 15 outstanding issues incl. real `govet shadow` bugs in `provider/streaming.go:184`, `provider/interaction_test.go:199`, `sse/hub_test.go:593`.
- **Impact**: lint is currently performative; high-signal findings go unaddressed.
- **Recommendation**: tighten exclusions to specific rules per file; enable the missing 10 linters; add a CI job that fails when `golangci-lint run` reports any issue. Replace standalone gosec (F-050).
- **Effort**: S each.

#### F-025/F-026/F-027/F-028 — High · Release
- See evidence in 2.1; no release pipeline + no GitHub Releases despite tags + CHANGELOG drift + no SemVer policy. Together they block any "stable" claim.
- **Recommendation**: §5 release workflow; cut `0.2.0` immediately; write `docs/policy/SEMVER.md` and `docs/policy/DEPRECATION.md`; rework `tag-modules.sh` (F-067).
- **Effort**: M for cuts; L for full automation.

#### F-029 — High · Hygiene · bus factor 1
- **Recommendation**: recruit at least one reviewer per major area (`server`, `auth`, `messaging`, `agent`/`llm`, `bench`, `infra/CI`). Minimum 2 maintainers before v1.0.
- **Effort**: organisational; treat as v1.0 blocker.

#### F-030 — High · Observability · no `/healthz` handler
- **Evidence**: `observability/health.go` defines `Health`, `Status`, etc. but no `http.Handler` is shipped.
- **Impact**: every consumer has to roll their own.
- **Recommendation**: ship `healthhttp.Registry` (sketch in §4); thread through `bootstrap/`.
- **Effort**: M.

---

## 3. Dimension-by-Dimension Assessment

For each dimension, full What's Good / Findings / Redesign Proposals are in the dim files.
This section gives the executive view.

### 3.1 Code Quality
**Good**: typed `AppError` foundation; structured zerolog logger; no blank-import side effects; `errors.Is/As` used in many places; generic `Resolve[T]` exists.
**Problems**: F-013 (DI any-typed), F-016 (9+ Must helpers), F-035, F-074, F-075, F-104, F-105, F-111. Pattern: panic-helpers proliferate; error-comparison anti-patterns; %v-flattening.
**Redesigns**: typed DI generics; `errors.Join` everywhere; ban `Must*` in non-init code via `revive`.

### 3.2 Architecture & Design
**Good**: sub-modules avoid pulling heavy deps into `core`; `component` lifecycle exists; `bootstrap` is a single composable entry point.
**Problems**: F-012 (worker→sse inversion), F-013 (DI), F-014 (no depguard), F-015 (6 registries with 5 policies), F-031 (init globals), F-060 (34-module justification), F-078 (StartAll write-lock).
**Redesigns**: `internal/registry`; depguard tiered rules; explicit `Registry` injection; pure-vs-IO split per package.

### 3.3 Concurrency & Safety
**Good**: `context.Context` propagation in most public APIs; `go.work` gives unified race testing.
**Problems**: F-008/9/10 (goroutine leaks), F-011 (zerolog global), F-076 (lock through Submit), F-077 (Stop ignores ctx), F-106 (`time.Sleep` ignores ctx), F-107 (fragile select).
**Redesigns**: `runtime.Runner` consolidating background goroutine pattern; per-instance logger level.

### 3.4 Security
**Good**: SECURITY.md exists; auth opt-in posture documented; no token-in-query default on server side.
**Problems**: F-001/2/3/5/6/7 plus F-036–F-042, F-086, F-087.
**Redesigns**: §4 — `Mode` enum replacing `AcceptMissing`/`AcceptInvalid` booleans; URL-strip after consuming opt-in query token; single-flight JWKS; `AuthChallenge` (RFC 6750).

### 3.5 Errors & Observability
**Good**: ProblemDetail (RFC 7807) implementation present; full Health taxonomy; otel wired.
**Problems**: F-031 (init), F-032/3 (auth bypasses ProblemDetail), F-034 (Wrap collapses), F-035 (cause leak), F-043/4/5 (otel globals + drop), F-030 (no /healthz handler).
**Redesigns**: `errors.Renderer` killing `init()`; `AppResult[T]`; idempotent `Telemetry.Init/Shutdown`.

### 3.6 Performance
**Good**: zerolog is fast; some hot paths use plain string ops.
**Problems**: F-020/21/47/48/49/81.
**Redesigns**: `sync.Pool`-backed buffers; `benchstat` gate; pprof hook.

### 3.7 Testing
**Good**: 252 test files; race+shuffle in CI; some fuzz; codecov per-module flags.
**Problems**: F-017/18/19/46; logger flake (F-011); 101 production `time.Now()` w/o Clock (TS-05).
**Redesigns**: Clock interface; integration tag; corpus persistence.

### 3.8 Lint & Static Analysis
**Good**: golangci-lint v2 config; misspell/errorlint/sqlclosecheck/rowserrcheck enabled.
**Problems**: F-022/23/24/50/51/52/79/80; bug-shadowing tolerated.
**Redesigns**: see §6.

### 3.9 CI/CD
**Good**: per-module test matrix; codecov per-module flags; existing fuzz smoke.
**Problems**: F-003/4/6/7/53/54/55/56/57/92/93/94/95/96.
**Redesigns**: §5.

### 3.10 Toolchain & Dep Hygiene
**Good**: 34 modules tidied consistently to `go 1.26.0`; consolidated dependabot recently.
**Problems**: F-001/58/59/60/61/97 + dependabot config bug.
**Redesigns**: `toolchain go1.26.2`; valid Dependabot grouping; document multi-module rationale.

### 3.11 Documentation
**Good**: 44/45 packages have `doc.go`; CONTRIBUTING/MAINTAINERS/GOVERNANCE all present.
**Problems**: F-062/63/64/65/66/88/89/90/91; no ADRs; thin examples; README badges.
**Redesigns**: ADR template; quickstart snippet; per-major-package `Example*` tests.

### 3.12 Release & Versioning
**Good**: CHANGELOG follows Keep-a-Changelog shape; tag-modules.sh exists.
**Problems**: F-025/26/27/28/67/68/69/98/99.
**Redesigns**: GoReleaser library mode + cosign keyless + SBOM; per-module independent SemVer; SemVer + Deprecation policies.

### 3.13 Repository Hygiene
**Good**: editorconfig/gitattributes/gitignore present; ISSUE_TEMPLATE folder present; CODEOWNERS present.
**Problems**: F-029/70/71/72/73/100/101/102/103.
**Redesigns**: pre-commit, actionlint+zizmor+gitleaks workflows, ISSUE_TEMPLATE/config.yml, tighten CODEOWNERS, recruit maintainers.

### 3.14 Cross-Sibling Consistency
**Good**: stated mirror intent across `gokit/rskit/pykit`.
**Problems**: cannot be confirmed until rskit and pykit reviews complete (Phase B/C). Open Question: whether `AppError`, `Component`, `Registry`, `Provider`, `Pipeline` shapes drift.
**Redesigns**: cross-sibling matrix in `cross-sibling-matrix.md` after all three reviews.

---

## 4. Redesign Proposals — Compileable Sketches

Full sketches live in the four dim files. The "biggest" ones recapitulated here:

### 4.1 Typed DI (`F-013`)

```go
// di/container.go (rewritten)
package di

import (
    "context"
    "fmt"
    "reflect"
    "sync"
)

type Key[T any] struct{ name string }

func NameKey[T any](name string) Key[T] { return Key[T]{name: name} }

type Provider[T any] func(ctx context.Context, c *Container) (T, error)

type Container struct {
    mu        sync.RWMutex
    providers map[reflect.Type]map[string]any // any = Provider[T]
    cache     map[reflect.Type]map[string]any // any = T
}

func New() *Container {
    return &Container{
        providers: make(map[reflect.Type]map[string]any),
        cache:     make(map[reflect.Type]map[string]any),
    }
}

func Provide[T any](c *Container, k Key[T], p Provider[T]) {
    c.mu.Lock()
    defer c.mu.Unlock()
    t := reflect.TypeOf((*T)(nil)).Elem()
    m, ok := c.providers[t]
    if !ok { m = map[string]any{}; c.providers[t] = m }
    m[k.name] = p
}

func Resolve[T any](ctx context.Context, c *Container, k Key[T]) (T, error) {
    var zero T
    t := reflect.TypeOf((*T)(nil)).Elem()
    c.mu.RLock()
    if cached, ok := c.cache[t][k.name]; ok { c.mu.RUnlock(); return cached.(T), nil }
    p, ok := c.providers[t][k.name]
    c.mu.RUnlock()
    if !ok { return zero, fmt.Errorf("di: %s/%s not provided", t.String(), k.name) }
    v, err := p.(Provider[T])(ctx, c)
    if err != nil { return zero, err }
    c.mu.Lock()
    if c.cache[t] == nil { c.cache[t] = map[string]any{} }
    c.cache[t][k.name] = v
    c.mu.Unlock()
    return v, nil
}
```

### 4.2 Generic Registry (`F-015`)

```go
// internal/registry/registry.go
package registry

import (
    "fmt"
    "sort"
    "sync"
)

type Registry[T any] struct {
    mu    sync.RWMutex
    items map[string]T
}

func New[T any]() *Registry[T] { return &Registry[T]{items: map[string]T{}} }

func (r *Registry[T]) Register(name string, v T) error {
    if name == "" { return fmt.Errorf("registry: name must not be empty") }
    r.mu.Lock(); defer r.mu.Unlock()
    if _, ok := r.items[name]; ok { return fmt.Errorf("registry: %q already registered", name) }
    r.items[name] = v
    return nil
}

func (r *Registry[T]) Get(name string) (T, error) {
    r.mu.RLock(); defer r.mu.RUnlock()
    v, ok := r.items[name]
    if !ok { var zero T; return zero, fmt.Errorf("registry: %q not registered", name) }
    return v, nil
}

func (r *Registry[T]) Names() []string {
    r.mu.RLock(); defer r.mu.RUnlock()
    out := make([]string, 0, len(r.items))
    for k := range r.items { out = append(out, k) }
    sort.Strings(out)
    return out
}
```
Each domain registry becomes `type Registry = registry.Registry[Factory]` and stops panicking.

### 4.3 Auth Mode (`F-039`, ER-03)

```go
// server/middleware/authmode.go
package middleware

type Mode int

const (
    ModeRequire Mode = iota // missing or invalid → 401
    ModeOptional            // missing → continue anonymously; invalid → 401
    ModeAcceptInvalid       // missing → continue; invalid → continue (debugging only)
)

type AuthOptions struct {
    Mode      Mode
    Verifier  Verifier
    Challenge AuthChallenge // RFC 6750 WWW-Authenticate
}
```
Replaces the `AcceptMissing`/`AcceptInvalid` boolean pair; eliminates the missing-vs-invalid conflation (F-033).

### 4.4 `runtime.Runner` (`F-008/9/10`)

```go
// runtime/runner.go
package runtime

import "context"

func Send[T any](ctx context.Context, ch chan<- T, v T) error {
    select {
    case ch <- v: return nil
    case <-ctx.Done(): return ctx.Err()
    }
}

func Recv[T any](ctx context.Context, ch <-chan T) (T, error) {
    var zero T
    select {
    case v, ok := <-ch:
        if !ok { return zero, ErrClosed }
        return v, nil
    case <-ctx.Done(): return zero, ctx.Err()
    }
}
```
Every `ch <- v` in `provider/`, `agent/`, `sse/` is rewritten to `runtime.Send(ctx, ch, v)`.

### 4.5 `errors.Renderer` killing `init()` (`F-031`)

```go
// errors/renderer.go
package errors

type Renderer struct{ TypeBase string }

func (r Renderer) ToProblem(err error) ProblemDetail { /* uses r.TypeBase */ }
```
Caller (typically `bootstrap`) constructs a `Renderer` once; `errors/response.go` no longer has `init()` and no longer reads a package global.

### 4.6 `healthhttp.Registry` (`F-030`)

```go
// observability/healthhttp/registry.go
package healthhttp

import (
    "encoding/json"
    "net/http"
    "sync"
)

type Check func(ctx context.Context) (Status, error)

type Registry struct{
    mu     sync.RWMutex
    checks map[string]Check
}

func (r *Registry) Add(name string, c Check) { /* ... */ }

func (r *Registry) Handler() http.Handler { /* aggregates statuses, returns RFC 7807 on failure */ }
```

---

## 5. CI/CD Blueprint

Files (paths shown, content abbreviated to key steps; SHA placeholders to be filled in):

### `.github/workflows/ci.yml` (updated)
```yaml
name: ci
on:
  pull_request:
  push: { branches: [main] }
permissions:
  contents: read
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  build-test:
    strategy:
      fail-fast: false
      matrix:
        go: ["1.26.x", "1.25.x"]
        os: [ubuntu-latest, macos-latest, windows-latest]
        include:
          - { os: ubuntu-24.04-arm, go: "1.26.x" }
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@<SHA> # v6.x
      - uses: actions/setup-go@<SHA> # v6.x
        with: { go-version: ${{ matrix.go }}, check-latest: true }
      - run: go build ./...
      - run: go test -race -shuffle=on -count=1 -coverprofile=cov.out -covermode=atomic ./...
      - uses: codecov/codecov-action@<SHA>
        with: { files: cov.out, fail_ci_if_error: true }
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<SHA>
      - uses: actions/setup-go@<SHA>
        with: { go-version: "1.26.x" }
      - uses: golangci/golangci-lint-action@<SHA>
        with: { version: v2.9.0, args: --timeout=10m }
  vuln:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<SHA>
      - uses: actions/setup-go@<SHA>
        with: { go-version: "1.26.x" }
      - run: go install golang.org/x/vuln/cmd/govulncheck@v1.3.0
      - run: govulncheck -format sarif ./... > govulncheck.sarif
      - uses: github/codeql-action/upload-sarif@<SHA>
        with: { sarif_file: govulncheck.sarif }
  fuzz-smoke:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<SHA>
      - uses: actions/setup-go@<SHA>
      - uses: actions/cache@<SHA>
        with: { path: '**/testdata/fuzz', key: fuzz-corpus-${{ github.sha }}, restore-keys: fuzz-corpus- }
      - run: ./tools/fuzz-smoke.sh 30s   # discovers all FuzzXxx functions automatically
  tidy-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<SHA>
      - uses: actions/setup-go@<SHA>
      - run: ./gomod.sh tidy && git diff --exit-code
  required:
    needs: [build-test, lint, vuln, fuzz-smoke, tidy-check]
    runs-on: ubuntu-latest
    steps: [{ run: echo ok }]
```

### `.github/workflows/release.yml` (new)
```yaml
name: release
on:
  push: { tags: ['v*', '*/v*'] }
permissions: { contents: write, id-token: write, packages: write }
jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<SHA>
      - uses: actions/setup-go@<SHA>
      - uses: sigstore/cosign-installer@<SHA>
      - uses: anchore/sbom-action@<SHA>
        with: { format: cyclonedx-json, output-file: sbom.cdx.json }
      - uses: goreleaser/goreleaser-action@<SHA>
        with: { args: release --clean }
        env: { GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }} }
```

### `.github/workflows/codeql.yml` (new)
```yaml
name: codeql
on: { push: { branches: [main] }, pull_request: {}, schedule: [{ cron: '17 3 * * 1' }] }
permissions: { security-events: write, contents: read, actions: read }
jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<SHA>
      - uses: github/codeql-action/init@<SHA>
        with: { languages: go }
      - uses: github/codeql-action/analyze@<SHA>
```

### `.github/workflows/actionlint.yml` (new)
```yaml
name: actionlint
on: { pull_request: { paths: ['.github/workflows/**'] } }
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<SHA>
      - uses: rhysd/actionlint@<SHA>
      - uses: woodruffw/zizmor-action@<SHA>
```

---

## 6. Lint Configuration Blueprint

Updated `.golangci.yml` (excerpt):

```yaml
version: "2"
linters:
  default: standard
  enable:
    # existing
    - misspell
    - unconvert
    - unparam
    - nakedret
    - prealloc
    - gocritic
    - gosec
    - bodyclose
    - noctx
    - govet
    - errcheck
    - staticcheck
    - exhaustive
    - unused
    - ineffassign
    - errorlint
    - nilerr
    - copyloopvar
    - wastedassign
    - sqlclosecheck
    - rowserrcheck
    # NEW (F-023)
    - paralleltest
    - tparallel
    - testifylint
    - contextcheck
    - nilnil
    - nestif
    - revive
    - depguard
    - tagliatelle
  exclusions:
    rules:
      # F-022 — narrow per-rule, not per-linter blanket
      - path: "_test\\.go"
        text: "Error return value of .Close|.Shutdown|.Logout. is not checked"
        linters: [errcheck]
      - path: "_test\\.go"
        text: "should not use ALL_CAPS in Go names"
        linters: [revive]
  settings:
    govet: { enable: [shadow], disable: [fieldalignment] }
    revive:
      rules:
        - name: "must-prefix"
          arguments:
            - { calls: ["init", "main"] } # ban Must* outside init/main
    depguard:
      rules:
        # F-014 — tier rules
        foundation:
          files:
            - "errors/**"
            - "logger/**"
            - "config/**"
            - "validation/**"
          deny:
            - { pkg: "github.com/kbukum/gokit/server", desc: "foundation must not import transport" }
            - { pkg: "github.com/kbukum/gokit/sse",    desc: "foundation must not import transport" }
            - { pkg: "github.com/kbukum/gokit/grpc",   desc: "foundation must not import transport" }
        worker-no-transport:
          files: ["worker/**"]
          deny:
            - { pkg: "github.com/kbukum/gokit/sse", desc: "worker is transport-agnostic (F-012)" }
formatters:
  enable: [gofmt, goimports, gofumpt]
  settings:
    goimports: { local-prefixes: [github.com/kbukum/gokit] }
issues:
  max-issues-per-linter: 0
  max-same-issues: 0
```

---

## 7. Remediation Roadmap

### Milestone `v0.x cleanup` (immediate; weeks-of-effort)
- F-001 (Critical · S) — bump Go to 1.26.2 + `toolchain` directive across all 34 manifests + `go.work`.
- F-003, F-004, F-006 (S each) — pin all actions by SHA; pin `govulncheck`; remove `@master`.
- F-005, F-050 (S each) — drop standalone gosec; rely on golangci-lint gosec only.
- F-008/F-009/F-010 (S each) — fix three goroutine leak sites with `select{ … <-ctx.Done() }` or `runtime.Send`.
- F-011 (S) — drop `zerolog.SetGlobalLevel`; per-instance level.
- F-022/F-023/F-024 (S) — tighten lint exclusions; enable missing 10 linters; fix the 15 outstanding lint issues.
- F-040 (S) — fix `TLSConfig.hasSettings` MinVersion bug.
- F-041 (S) — enforce JWT HMAC secret length.
- F-058 (S) — remove invalid `group-by: dependency-name` from dependabot.yml.
- F-031 (S) — drop `errors/response.go init()`; convert to `Renderer`.
- F-025/F-026/F-027 (S→M) — cut `0.2.0` Release; populate CHANGELOG; add minimal `release.yml`.
- F-051 (S) — pin golangci-lint to a version in CI; align with dev.

### Milestone `v0.y redesign`
- F-013 (XL) — typed DI rewrite.
- F-015 (L) — `internal/registry`; collapse 6 implementations.
- F-012 (M) — move worker→sse bridge.
- F-014 (S) — depguard tier rules (locks F-012 in).
- F-002, F-036, F-037, F-038 (S–M) — OIDC verifier hardening: alg binding, clock skew, nonce, single-flight JWKS.
- F-032/33/34/35 (S–M) — auth middleware → ProblemDetail; `Mode` enum; `WWW-Authenticate`; `AppError.Error()` no longer leaks cause.
- F-039 (S) — strip query token after consumption.
- F-018 (M) — fuzz coverage on JWT/JWKS/OIDC/API key/problem JSON/schema/MCP/config.
- F-019 (S) — `//go:build integration` tagging.
- F-046 (M) — broaden table-driven adoption.
- F-020/21/47/48/49 (M–L) — bench harness + benchstat gate + sync.Pool + pprof.
- F-030 (M) — ship `healthhttp.Registry`.
- F-043/44/45 (S–M) — observability idempotency.
- F-053/54/55/56/92/93/94/95/96 (S each) — CI cleanups.
- F-064/65/66/89 (S–M) — ADRs; README quickstart; release-process doc.

### Milestone `v1.0 stabilization`
- F-007 (L) — full release pipeline (GoReleaser library mode + cosign keyless + syft SBOM + SLSA provenance).
- F-028 (M) — SemVer + Deprecation policies (mandatory v1.0 prerequisite).
- F-029 (organisational) — recruit ≥1 reviewer per major area; bus factor ≥2.
- F-067/68 (M each) — independent per-module SemVer; signed tags; clean-tree check in `tag-modules.sh`.
- F-017 (L) — coverage floors per package (≥80%) enforced at codecov.
- F-070/71/72/73 (S each) — pre-commit, actionlint+zizmor, gitleaks, branch protection audit.
- F-060 (M) — document or simplify the 34-module split.

### Open / informational
- F-073 — branch protection (out of file scope, must verify).
- Cross-sibling parity: blocked on Phase B (rskit) and Phase C (pykit).

---

## 8. Open Questions / Assumptions

1. **Branch protection** rules are not file-visible. We assume `CI Status` aggregate is required; signed commits / linear history may not be enabled. Verify before v1.0.
2. **Multi-module rationale**: docs hint at "core lightweight, sub-modules pull heavy deps" but never explain `*/testutil` modules (8 of 34). Open question whether they could collapse into a single `testutil` module per top-level area.
3. **Cross-sibling consistency** (Dimension 14) cannot be evaluated until the rskit and pykit reviews complete; placeholders are in `cross-sibling-matrix.md`.
4. **SBOM tooling choice** (`syft` vs `cyclonedx-gomod`) is left to maintainer preference; both compile to the GoReleaser blueprint above.
5. **Codecov per-package floors**: the prompt requires ≥80% per package; baseline coverage measurement was for the root module only — full per-module run pending under the `cov-all` Make target proposed in dim3.
6. **Windows + race**: Windows test job runs without `-race` (F-055). We assume this is intentional due to MinGW/cgo cost; needs explicit doc.
7. **`tag-modules.sh` consumers**: any downstream automation depending on lockstep tagging (F-067) must be inventoried before independent tagging is rolled out.

---

## 9. Final Verdict — v1.0 Readiness Gate

**NOT READY for v1.0.** Block list:

- [ ] F-001 Go 1.26.2 + `toolchain` directive across all manifests (Critical).
- [ ] F-002 OIDC alg-confusion fix.
- [ ] F-003/4/5/6 supply-chain hygiene (SHA pinning; remove `@master`; pin govulncheck; drop standalone gosec).
- [ ] F-007 Release pipeline with cosign + SBOM + SLSA provenance.
- [ ] F-008/9/10 Goroutine leak fixes.
- [ ] F-011 Logger global state removal.
- [ ] F-013 Typed DI redesign (or explicit deferral with documented breaking-change schedule).
- [ ] F-015 Single registry implementation.
- [ ] F-022/23 Lint config tightening + missing linter set.
- [ ] F-025/26/27 Active GitHub Releases; current CHANGELOG.
- [ ] F-028 SemVer + Deprecation policies.
- [ ] F-029 Bus factor ≥2.
- [ ] F-030 `/healthz` shipped.
- [ ] F-017 ≥80% coverage in `auth/oidc`, `auth/apikey`, `server`, `server/middleware`, `server/endpoint`.
- [ ] F-018 Fuzz on JWT/JWKS/OIDC/API key/problem JSON/schema/MCP/config.

The repository has a coherent design intent, clear documentation, and a working monorepo scaffolding. The v1.0 gap is overwhelmingly in **supply-chain posture**, **concurrency safety guarantees**, and **lifecycle automation** — none of which are conceptually hard, but together they represent multiple weeks of focused work.

---

*End of review. After approval, Phase E will create a per-finding GitHub issue using the prompt's mandated template, plus a meta tracking issue with the milestone roadmap inline. Cross-sibling backlinks will be added once `rskit` and `pykit` reviews complete.*
