# Docs and supply chain

Do the docs match the code, and is every dependency and pipeline safe?

Rules: [Documentation and consumer experience](../../../engineering.md#documentation-and-consumer-experience), [Dependencies and supply chain](../../../engineering.md#dependencies-and-supply-chain). Dependency holds: [dependencies](../../../../docs/dependencies.md).

## Look for

| Check | Severity |
|---|---|
| Exported identifier without a doc comment starting with its name | should-fix |
| Godoc, package README, or example describing old behavior | should-fix |
| Behavior change without a CHANGELOG `[Unreleased]` entry | should-fix |
| Module added, renamed, or removed without its Toven group and `make module-index` | should-fix |
| Guidance restating a rule instead of linking its owner; a claim of enforcement no gate makes | should-fix |
| Hard-wrapped Markdown prose; broken relative link or anchor | nit |
| New dependency that the standard library or an owner covers, or that is unmaintained | should-fix |
| Dependency with an open advisory, or a disallowed license | blocker |
| `govulncheck` finding silenced without a reason in `.github/govulncheck-suppressions.json` | blocker |
| Untidy `go.mod`/`go.sum`; new module missing from workspaces | should-fix |
| Workflow action pinned by tag instead of SHA, or broad job permissions | should-fix |

## Detection starters

```bash
grep -rn 'uses:' .github/workflows .github/actions | grep -v '@[0-9a-f]\{40\}' | grep -v 'uses: \./'
git diff --name-only <base>...HEAD | grep -E 'go\.mod|toven\.toml|MODULE-INDEX|CHANGELOG'
grep -rn --include=*.go '^func [A-Z]\|^type [A-Z]' <path> | grep -v _test.go   # spot-check godoc
```

## Gates

```bash
make tidy-check     # read-only tidiness
make vuln           # govulncheck per module
make licenses       # license allow-list
make module-index   # regenerate, then check git diff
make actionlint     # when workflows change
```
