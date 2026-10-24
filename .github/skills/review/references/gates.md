# Gates

Which rules does a gate actually check, did the gates run, and does any doc claim more than they do?

Rules: [Enforcement](../../../engineering.md#enforcement), [Known gaps](../../../engineering.md#known-gaps). Sources of truth: `make help`, [`toven.toml`](../../../../toven.toml), [`.golangci.yml`](../../../../.golangci.yml), [`codecov.yml`](../../../../codecov.yml), [CI](../../../workflows/ci.yml).

## Look for

| Check | Severity |
|---|---|
| Doc, skill, or comment claims a rule is enforced when no gate checks it | should-fix |
| A gate weakened: floor lowered, linter disabled, job made optional, `continue-on-error` added, suppression without a reason | blocker |
| A coverage ratchet in `toven.toml` lowered instead of raised | blocker |
| Gate config changed without updating [Enforcement](../../../engineering.md#enforcement) | should-fix |
| Acceptance claimed without the required gate output, or with unit proof standing in for integration proof | blocker |
| A known gap closed by a new gate but still listed in [Known gaps](../../../engineering.md#known-gaps) | nit |

## Detection starters

```bash
git diff <base>...HEAD -- .golangci.yml codecov.yml toven.toml Makefile .github/workflows
grep -n 'continue-on-error' .github/workflows/*.yml
grep -rn 'enforced\|enforces\|gated by\|CI fails' --include=*.md . | grep -v '^./tmp/'
```

For each enforcement claim, find the line in a gate source that checks it. If none exists, the claim moves to Known gaps or is removed.
