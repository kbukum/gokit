# Tests and proof

Is every behavior proven by a deterministic test, and is the proof the right kind?

Rules: [Testing and proof](../../../engineering.md#testing-and-proof), including [real adapter proof](../../../engineering.md#real-adapter-proof). Placement: [test layout](../../../../docs/TEST-LAYOUT.md).

## Look for

| Check | Severity |
|---|---|
| New or changed behavior with no test in the same change; a fix with no regression test | blocker |
| Failure paths untested | should-fix |
| Unit or fake proof claimed for driver, TLS, process, container, or browser behavior that needs real adapter proof | blocker |
| Auth or session change not proven through `auth/testhost` | blocker |
| Module, package, security-set, or patch coverage below its gated floor | blocker |
| Tests asserting private fields or call order | should-fix |
| `time.Sleep` waits, real network, unseeded randomness, global env mutation | should-fix |
| Shared state between parallel tests; failure under `-shuffle=on` | blocker |
| Fake hand-rolled inline that belongs in a `testutil` helper; padding file names (`_extra`, `_coverage`) | should-fix |
| New parser, validator, codec, schema, or auth path without a fuzz target | should-fix |

## Detection starters

```bash
git diff --name-only <base>...HEAD -- '*.go' | grep -v _test.go   # source changed
git diff --name-only <base>...HEAD -- '*_test.go'                 # tests changed
grep -rn --include=*_test.go 'time\.Sleep\|http\.Get\|net\.Dial\|os\.Setenv' <path>
grep -rL --include=*_test.go 'func Fuzz' <path>
```

## Gates

```bash
make test M=<module> T=<pattern>   # race + shuffle, one module
make test-affected                 # changed modules and dependents
make test-coverage M=<module>      # race tests with coverage
make test-integration M=<module>   # Docker integration
```

A green unit run does not prove integration behavior. Record which kind of proof was run.
