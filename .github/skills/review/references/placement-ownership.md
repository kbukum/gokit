# Placement and ownership

Is the code in the right module, package, and layer, and does it reuse the canonical owner? Misplaced or forked code makes every later lens moot.

Rules: [Placement and reuse](../../../engineering.md#placement-and-reuse). Model: [Architecture](../../../../docs/ARCHITECTURE.md). Owners: [concern owners](../../../../docs/concern-owners.md). Naming and layout: [naming and structure](../../../../docs/naming-and-structure.md).

## Look for

| Check | Severity |
|---|---|
| Upward import or cycle against the [layer table](../../../../docs/ARCHITECTURE.md#package-layers) | blocker |
| Heavy dependency in the root module, or a foundation concern buried in a sub-module | blocker |
| A local helper, error type, retry loop, HTTP client, subprocess call, or path check that an owner already provides | blocker |
| Owner is close but inadequate and the change forks a near-copy instead of enhancing it | blocker |
| Foundational capability with no owner solved locally instead of in the right module | should-fix |
| An owner nothing imports yet (its consumers are probably running forks) | should-fix |
| New sub-module missing a `replace`, `go.work`/`core.go.work`/`contrib.go.work` entry, or Toven group | should-fix |
| Backend that registers in `init()` instead of an explicit `Register(registry)` | blocker |
| Stuttering or misleading name, catch-all file mixing concerns | should-fix |

## How to judge reuse

1. Name the concern of each new type or helper.
2. Find its owner in [concern owners](../../../../docs/concern-owners.md), or among existing modules (`toven modules`).
3. Decide: **reuse** (delete the fork), **enhance** the owner generically, **add** to the right owner, or **justified local** (state why; flag it to promote later).

Name the exact symbol that should be used, for example "use `fs.ConfineExistingPath` instead of local `ensureUnderRoot`".

## Detection starters

```bash
# gokit edges per module
for m in $(find . -name go.mod -not -path './tmp/*'); do echo "== $m"; grep -E '^\s+github.com/kbukum/gokit' "$m"; done
# stdlib calls that an owner may already wrap
grep -rnE 'exec\.Command|http\.Client\{|filepath\.EvalSymlinks|os\.WriteFile|sort\.Slice|log\.Print|fmt\.Print' --include=*.go <path> | grep -v _test.go
# import-time registration
grep -rn --include=*.go '^func init()' . | grep -v _test.go
# who imports an owner
grep -rln 'kbukum/gokit/<owner>' --include=*.go . | grep -v _test.go | grep -v '^./<owner>/'
```

Then `make lint M=<module>` (depguard's selected bans) and any Toven run (domain `allow_groups`). Layers not covered by depguard are checked here by reading imports.
