# gokit engineering standard

The rules every change follows. Read the section your change needs, not the whole file. Agents start at [copilot-instructions.md](copilot-instructions.md).

## Read when

| You are changing | Read |
|---|---|
| Where a package or module goes, imports, shared helpers | [Placement and reuse](#placement-and-reuse) |
| Exported identifiers, signatures, options, module versions | [Public API and stability](#public-api-and-stability) |
| Error types, retries, timeouts, remote calls | [Errors and resilience](#errors-and-resilience) |
| Goroutines, channels, components, shutdown | [Concurrency and lifecycle](#concurrency-and-lifecycle) |
| Registries, adapters, config selection, globals | [Composition](#composition) |
| Input handling, auth, crypto, paths, SQL, subprocesses | [Security and privacy](#security-and-privacy) |
| LLM, agent, tool, or retrieval code | [AI and model features](#ai-and-model-features) |
| File layout, naming, signatures, `doc.go` | [Code style and structure](#code-style-and-structure) |
| Tests, fixtures, coverage, integration proof | [Testing and proof](#testing-and-proof) |
| Which command to run | [Validation](#validation) |
| READMEs, godoc, comments, changelog, skills | [Documentation and consumer experience](#documentation-and-consumer-experience) |
| Dependencies, CI, release | [Dependencies and supply chain](#dependencies-and-supply-chain) |
| Whether a rule is checked by a gate | [Enforcement](#enforcement) and [Known gaps](#known-gaps) |

## Where guidance lives

Each topic has one owner. Other files link to it.

| Topic | Owner |
|---|---|
| What gokit is, packages, quickstart, status | [README](../README.md) |
| Where code goes: layers, module kinds, domains | [Architecture](../docs/ARCHITECTURE.md) |
| Which package owns a shared concern | [Concern owners](../docs/concern-owners.md) |
| Naming and file layout | [Naming and structure](../docs/naming-and-structure.md) |
| Test placement | [Test layout](../docs/TEST-LAYOUT.md) |
| Why a decision was made | [ADRs](../docs/adr/README.md) |
| Rules every change follows | This file |
| Contributor workflow | [CONTRIBUTING](../CONTRIBUTING.md) |
| Vulnerability reporting and secure use | [SECURITY](../SECURITY.md), [security model](../docs/security-model.md) |
| Versioning, release, deprecation | [Versioning](../docs/VERSIONING.md), [Releasing](../docs/RELEASING.md), [SemVer policy](../docs/policy/SEMVER.md), [Deprecation](../docs/policy/DEPRECATION.md) |
| Agent startup rules | [copilot-instructions.md](copilot-instructions.md) |
| Procedures | [Skills](skills/README.md) |
| Gate list | `make help`, [`toven.toml`](../toven.toml), [CI](workflows/ci.yml) |

Versions of Go and tools live in `go.mod`, the Makefile, and CI. Docs do not copy them.

## Priorities

1. **Consistency across the project.** Make the same choice everywhere so the code reads as one whole.
2. **Current, idiomatic Go.** Use today's standard library and practices, not folklore.
3. **Parity with rskit.** Match capabilities and wire contracts where it does not cost ergonomics or idiom. When one kit has the stronger design, the other levels up. Internal types do not need to match.

## Change workflow

- Work in phases: discover, decide, implement completely, validate.
- For each real defect choose **Redesign**, **Align**, **Enhance**, or **Drop**, with a one-line reason. Leave sound code alone.
- gokit is pre-1.0. Fix root causes and update callers. Do not add compatibility shims, wrappers, or "keep the old path" hedges.
- Build the simplest design that fully solves the problem. Complexity must earn its place.

## Placement and reuse

- Put code where [Architecture](../docs/ARCHITECTURE.md) says it goes. Imports point down; a cycle or upward import is a defect.
- Before writing a helper, find the owner in [concern owners](../docs/concern-owners.md). Reuse it, or enhance it generically so every consumer benefits. Never fork a near-copy.
- Shared concerns always have one owner: errors, config, logging, auth, retries, observability, HTTP, registries, validation, subprocesses, DI, filesystem paths.
- Prefer the standard library when it already covers the need.
- Heavy external dependencies live in their own sub-module, never the root module.
- gokit never imports another kit at runtime.

## Public API and stability

- Keep the public surface small and typed. Unexport what callers do not need.
- Use generics where they remove real duplication or unsafe casts, not by default.
- No `any` or `interface{}` in public APIs, except for genuinely opaque values (JSON bodies, `ctx.Value`, DB scans, third-party contracts). Document each one.
- Interfaces are small. Components opt in to extra capabilities through separate interfaces.
- Export interfaces and constructors; keep concrete implementations unexported unless callers need the type.
- Constructors take `...Option`, or a cohesive request/config struct. Follow the owning package's existing pattern. A module's `Config` struct has `ApplyDefaults()` and `Validate()`.
- `context.Context` is the first parameter and is never stored in a struct.
- Names read well at the call site and do not stutter (`cache.New`, not `cache.NewCache`). See [naming and structure](../docs/naming-and-structure.md).
- **Stability.** gokit is pre-1.0. A minor release may break APIs; patch releases do not. Breaking changes are listed in the [CHANGELOG](../CHANGELOG.md). Follow the [SemVer policy](../docs/policy/SEMVER.md) and [deprecation policy](../docs/policy/DEPRECATION.md).
- A change to a core package's public surface affects every sub-module, adapter, and downstream consumer. List that impact before changing it.

## Errors and resilience

- Return typed `AppError` values (RFC 9457) with typed codes. Preserve the cause with `%w`, `errors.Is/As/Join`.
- `errors.New(code, message)` derives the REST status. Do not add status overrides.
- `errors/rpc` owns the shared protobuf details for Connect and gRPC. Transport-specific mapping stays in its transport package.
- Validation errors expose semantic violation reasons, not validator rule IDs.
- Decoded remote failures are not trusted public `AppError`s.
- No `panic`, `log.Fatal`, ignored errors (`_ =`), or unchecked type assertions on runtime paths. `Must*` is only for compile-time-safe construction or an explicit twin of an error-returning function.
- No success-shaped fallbacks: never return a zero value with a `nil` error on a real failure.
- Every remote call has a timeout through `context.Context`.
- Retry only idempotent operations, with bounded, jittered backoff, and one retry owner. Server-provided delays are minimums. Use [`resilience`](../resilience/) rather than hand-rolled loops.
- Circuit-break and degrade gracefully instead of hanging or cascading.

## Concurrency and lifecycle

- Every goroutine has an owner, cancellation through context, a timeout, and a shutdown path. A goroutine with no way to stop is a defect.
- Channels, buffers, queues, and concurrency are bounded, with documented backpressure.
- Components drain in-flight work on shutdown and tear down within a bound.
- Guard shared state with a mutex or confine it to one goroutine. Do not hold a lock across blocking work.
- Component lifecycle is `Start`/`Stop`/`Health` with deterministic ordering through the registry.

## Composition

- Registries and policies are created and injected explicitly. Config selects a registered adapter.
- No `init()` side effects, no import-time I/O, and no mutable package-level registries.
- Inject loggers, tracers, policies, and clients. Do not reach for globals.
- A backend adapter registers through an explicit `Register(registry)` call. The core package keeps a lean in-memory or local default.
- Provider shapes are `RequestResponse`, `Stream`, `Sink`, and `Duplex`, with registry, manager, and selector. Cross-cutting behavior composes as `Middleware[I, O]` chains.

## Security and privacy

- Validate untrusted input at every trust boundary. Bound reads (`io.LimitReader`) and sizes.
- Secure by default and least privilege.
- Parameterized SQL only. Subprocesses are argv-only through [`process`](../process/), with an isolated environment and bounded output.
- Credentials go in headers or the secure browser-session contract, never in URLs or query strings. Never log them.
- Use current cryptography from [`encryption`](../encryption/) and [`security`](../security/). Reject `alg: none` and check JWT `exp`, `iss`, and `aud`.
- Confine paths under a root and reject symlink escapes through [`fs`](../fs/).
- Minimize, redact, and retention-bound sensitive data.
- No secrets in source, logs, fixtures, or retained artifacts.

Threats and secure use are described in the [security model](../docs/security-model.md).

## AI and model features

- Model output and retrieved context are untrusted input.
- Enforce structured, validated outputs.
- Tool calls are least privilege. Destructive actions need a human gate.
- Version prompts and models, and gate changes on evals.

## Code style and structure

- Format with gofmt, goimports, and gofumpt; lint with golangci-lint. Config is in [`.golangci.yml`](../.golangci.yml).
- Package names are lowercase, single word, and singular.
- Every package has a `doc.go` that holds only the package clause and package docs. No declarations or imports.
- Split code into concern-named files. Look again at a mixed-concern file around 300–400 code lines, or a package with more than about 10 non-test files in 2–3 separable groups. These are signals, not limits. A cohesive file may stay large. Split only when ownership becomes clearer, without cycles or over-fragmentation.
- **Signatures, not columns.** A long signature is a design signal. Reduce parameters with a request/options struct or functional options instead of hand-wrapping. When a multi-line signature is warranted, use the gofmt-stable one-parameter-per-line form with a trailing comma.
- No dead code, commented-out code, or speculative scaffolding. TODOs link a tracked issue.

Details and examples: [naming and structure](../docs/naming-and-structure.md).

## Testing and proof

- Test first: failing test, minimal code, then refactor while green. Every fix has a regression test.
- Tests assert behavior and public contracts, including failure paths. They do not assert private fields or call order.
- Tests are deterministic: inject clocks (no `time.Sleep` waits), seed randomness, use `t.TempDir` and `t.Setenv`, and no real network in unit tests.
- Tests pass under `-race -shuffle=on -count=1`. Parallel tests are independent. Use table-driven tests when cases share setup, inputs, or assertions.
- Reuse [`testutil`](../testutil/README.md) and the owning area's test helpers. Do not hand-roll fakes inline.
- Parsers, validators, codecs, schema, and auth/JWT code have fuzz targets.
- Place tests by the [test layout](../docs/TEST-LAYOUT.md).
- **Coverage.** The gated floors are 80% per module (Toven) and, through Codecov, 80% per package, 85% for `errors`, `auth`, `authz`, `security`, `resilience`, and `encryption`, and 85% on the patch. A few modules carry a lower ratchet in [`toven.toml`](../toven.toml) that may only rise. The target is 85% overall; see [Known gaps](#known-gaps).

### Real adapter proof

Unit tests and builds do not prove driver, TLS, process, container, or browser behavior.

- Runtime test-environment changes need real adapter proof. Reuse `testutil` for fresh, bounded cleanup and rollback, `component/testutil` for deterministic lifecycle failures, and adapter test helpers for real loopback, file, and container checks. Reset fixture data with requests quiesced; do not erase handlers or migration metadata. Required PostgreSQL checks fail when Docker is missing. Fake migration drivers and in-memory AutoMigrate prove orchestration only. Treat forced shutdown as a failure, and record final commands, limits, and cleanup outcomes.
- Authentication changes prove the real HTTP, Connect, and SSE session lifecycle through [`auth/testhost`](../auth/testhost/README.md). Use trusted HTTPS with no verification bypass or weaker cookies. Reset before sign-in; never restore saved browser credentials against reset state. Cover duplicate, mixed, and invalid credentials, unsafe-cookie CSRF, unavailable storage, rotation and logout races, and local and cross-instance stream teardown. Scenario controls belong only to the test module. Frontend acceptance belongs to the browser consumer.

## Validation

Scope commands to what changed. Full gates are for acceptance and CI. Run `make help` for every target.

```bash
make test M=<module> T=<pattern>   # one module, race + shuffle
make lint M=<module>               # golangci-lint, one module
make test-affected                 # changed modules and their dependents
make check-<domain>                # one domain; `toven groups` lists domains
make check                         # build + vet + test, full acceptance
```

The [validate skill](skills/validate/SKILL.md) lists Toven selectors and the extra gates (coverage, integration, vuln, licenses, tidy, structure). Docs-only changes need link and anchor checks, not Go builds.

## Documentation and consumer experience

- gokit publishes to pkg.go.dev, so godoc is the public documentation. Every exported identifier has a doc comment that starts with its name.
- Docs lead with the reader's task or the capability. Keep detail only where it is needed for correct use, contracts, safety, or verification.
- A behavior change updates its godoc, package README, examples, and CHANGELOG in the same change. Adding, renaming, or removing a module also updates its `toven.toml` group and regenerates [MODULE-INDEX](../docs/MODULE-INDEX.md) with `make module-index`.
- Examples compile and use the current API.
- Errors tell the caller what failed and what to do.
- Comments explain why, not what. They describe the code as it is now: no history, plans, or fix narration.
- Markdown and Go comment prose is not hard-wrapped to a column. Keep real structure: paragraphs, lists, tables, code blocks.
- Changelogs summarize outcomes by release and concern, and keep breaking changes visible.
- Skills state triggers, actions, and acceptance, and link the rules here instead of restating them.

## Dependencies and supply chain

- Each new dependency is necessary, maintained, license-compatible, and free of open advisories. Check primary sources.
- `go.mod`/`go.sum` stay tidy. New modules join the right `go.work` files and Toven group.
- Triage `govulncheck` findings in [`.github/govulncheck-suppressions.json`](govulncheck-suppressions.json) with a reason; never silence them.
- Pin GitHub Actions by commit SHA and give jobs minimal permissions.
- Release artifacts are signed (cosign) and carry an SBOM and provenance. See [Releasing](../docs/RELEASING.md).

## Enforcement

Rules checked by a gate today. CI's `CI Status` job requires the gate, check, cross-platform check, lint, security, fuzz, licenses, and integration jobs.

| Rule | Gate |
|---|---|
| Formatting, lint, gosec, selected import bans | `make lint` (golangci-lint, `.golangci.yml`) |
| Module-to-domain dependency direction | Toven `[groups.*].guardrails.allow_groups`, checked on every Toven run from the direct module graph |
| Build, vet, race + shuffle tests | `make check`, `make test-coverage` |
| Coverage floors per module, package, security set, and patch | Toven coverage and `codecov.yml` |
| Known vulnerabilities | `make vuln` |
| Dependency licenses | `make licenses` |
| Tidy modules, replace graph, uniform Go version | `make tidy-check`, `make replace-check`, CI gate job |
| Workflow syntax | `make actionlint` |
| Unreleased CHANGELOG section | `scripts/check_changelog_unreleased.sh` in CI |
| Fuzz targets run | `scripts/fuzz.py` in CI (`make fuzz` locally) |
| Real Docker integration | `make test-integration` in CI |
| Release signing, SBOM, provenance | [`release-artifacts.yml`](workflows/release-artifacts.yml) |

`make structure` (declare-only `doc.go`, large files, test-file suffixes) runs in CI but is advisory only.

## Known gaps

These rules are written down but no gate checks them. They are intentionally not built yet and open to revisit.

- **Package layering inside the root module.** depguard bans only selected edges (foundation packages importing higher layers, observability importing AI, transport importing auth, core errors importing wire types, auth core importing database). The full layer order in [Architecture](../docs/ARCHITECTURE.md) is checked in review.
- **ADR 0001 vs the current layer model.** ADR 0001 describes three tiers; the current model is in [Architecture](../docs/ARCHITECTURE.md). No superseding ADR exists yet.
- **85% overall coverage.** A target, not a gate. Codecov's project check is relative (`auto`).
- **No `init()`, no global registries, no runtime panics.** Checked in review, not by a linter.
- **`doc.go` presence and godoc on exports.** Not gated; `make structure` only reports `doc.go` content.
- **Action SHA pinning.** Followed in workflows, not checked by a gate.
- **MODULE-INDEX freshness.** Regenerated by hand; CI does not diff it.
- **Markdown links and anchors.** No docs lint or link check in CI.

## Definition of Done

- [ ] Code sits in the right layer and reuses the canonical owner.
- [ ] Public API is minimal and typed; breaking changes are in the CHANGELOG.
- [ ] Errors are typed with causes; no runtime panics, ignored errors, or success-shaped fallbacks.
- [ ] Goroutines, queues, retries, and remote calls are owned, bounded, and cancellable.
- [ ] Inputs are validated at trust boundaries; no secrets in source, logs, or URLs.
- [ ] Behavior has deterministic tests, including failures; real adapter proof where required.
- [ ] Scoped gates pass, and `make check` passes for acceptance.
- [ ] Godoc, READMEs, examples, MODULE-INDEX, and CHANGELOG match the change.
- [ ] New dependencies are justified and clean under `make vuln` and `make licenses`.
