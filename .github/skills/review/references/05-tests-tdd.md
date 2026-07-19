# Pass 05 — Tests & TDD

Behavior is only real if a test proves it. Vibe-coded changes routinely ship without tests, or with tests that assert implementation detail instead of behavior. This pass verifies the change is covered, deterministic, and race-clean.

> **Run in a separate, clean-context agent** — never inline in the session that wrote the code.
> An independent reviewer re-derives every judgment from the code
> and the principles instead of trusting prior reasoning.
> A plan/spec may be passed in as a scope checklist only; it never excuses a baseline violation.

**Scope note.** *Changes mode:* every behavioral change in the diff has a test in the same change; every bug fix has a regression test. *Project mode:* assess coverage against the gates and hunt for flaky/implementation-coupled tests across the suite.

## Checks

- **Tests ship with the change.** New/changed behavior has tests in the same change set;
  a bug fix has a regression test that fails without the fix.
  Behavior added with no test is a **blocker**.
- **Behavioral, not implementation-coupled.** Tests assert observable behavior and public contracts,
  not private field values or call sequences that would break on a harmless refactor.
- **Deterministic.** Clocks are **injected** (never `time.Sleep` to "wait" for async work), RNG is **seeded**, no real network / filesystem in unit tests (use fakes / `testutil` / `t.TempDir`). A test that sleeps or hits the network is a should-fix.
- **Race / shuffle / parallel green.** Suite passes under `go test -race -shuffle=on`, and `t.Parallel()` tests are actually independent. Table-driven tests capture the range var correctly (trivially safe on 1.22+, but still check shared mutable state).
- **Coverage gates.** ≥80% per package, ≥85% overall; ≥85% for the security-load-bearing packages (`errors`, `auth`, `authz`, `security`, `resilience`, `encryption`). A change that drops a gated package below its floor is a blocker. (CI enforces project 80% / patch 85% via `codecov.yml`; the per-package floors are checked each step via `make test-coverage`.)
- **Fuzz where it matters.** Parsers, validators, auth/JWT, codecs, and schema have `Fuzz` targets.
  A new parser/validator with no fuzz target is a should-fix.
- **Environment-independent.** Tests use `t.Setenv` (auto-restored) rather than mutating global env;
  no ordering dependency between tests.

## Detection starters

```bash
# behavior touched vs tests touched (changes mode)
git diff --name-only | grep '\.go$' | grep -v _test.go   # source changed
git diff --name-only | grep '_test\.go$'                 # tests changed — should be non-empty
# non-deterministic / external-dependency smells in tests
grep -rn --include=*_test.go 'time.Sleep\|time.Now()\|http.Get\|net.Dial\|rand.Int' .
# missing fuzz on parser/validator/codec/auth packages
grep -rLn --include=*_test.go 'func Fuzz' codec schema auth validation 2>/dev/null
```

## Validation gate

Run the scoped suite for the touched domain, with race + shuffle:

```bash
make test-affected                 # race-enabled, only affected modules (fast inner loop)
./gomod.sh cmd "go test -race -shuffle=on -count=1 ./..." -m <module>   # one module, thorough
make check-<domain>                # domain gate (fmt + vet + lint + test) for the touched area
```

A behavioral change with a green race+shuffle run and coverage above the gate passes; anything untested, sleeping, or below the coverage floor does not.
