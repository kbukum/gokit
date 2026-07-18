# Pass 01 — Canonical-owner reuse

gokit *is* the canonical toolkit, so the duplication risk is internal: **did the change reimplement something an existing package (or the standard library) already owns?** Vibe-coded code reaches for a fresh local helper instead of the owner — assume duplication until proven otherwise. Treat findings here as a blocker class.

> **Run in a separate, clean-context agent** — never inline in the session that wrote the code. An independent reviewer re-derives every judgment from the code and the principles instead of trusting prior reasoning. A plan/spec may be passed in as a scope checklist only; it never excuses a baseline violation.

**Scope note.** *Changes mode:* for each new type/helper in the diff, name the concern and find its owner. *Project mode:* sweep the tree for the patterns below and check each against the owning package — long-lived internal forks are exactly what this pass exists to surface.

## The rule

Reuse or enhance the canonical owner before writing new code. Never duplicate a shared concern — **errors, config, logging, auth, retries/resilience, observability, HTTP, registries, validation, process, di**. If the owner is inadequate, enhance it *generically* rather than forking a copy in another package. gokit must stay foundational and multi-purpose: a fix belongs in the owner so every consumer benefits.

## How to check — build the owner map, then compare

The canonical owner set is documented in [`docs/concern-owners.md`](../../../../docs/concern-owners.md) — start there, then check each low-level operation against it. Do not eyeball it and do not rely on a fixed concern list — that is how a fork slips through. Work it as a method, in order:

**1. Build the owner map.** Every gokit module is a potential owner. Establish what this module *could* reuse before judging what it *does*:

```bash
ls -d */ | tr -d /                                       # all gokit modules = the candidate owner set
grep -E '^\s+github.com/kbukum/gokit' <module>/go.mod    # owners it already imports (reuse adds no new edge)
```

**2. Scan the module for every low-level operation and check each against that map.** The class most often missed is a **drop to the standard library for a capability a gokit module already wraps** (safe file/path handling, subprocess, HTTP, atomic writes) — not just a reimplemented named concern. Sweep the in-scope code, not the tree:

```bash
grep -rnE 'os\.|filepath\.|ioutil\.|exec\.Command|net/http|http\.Client|sort\.Slice|"sort"|errors\.New\(|fmt\.Errorf\(|log\.Print|fmt\.Print|time\.After|context\.WithTimeout' <module> --include=*.go | grep -v _test.go
```

For each hit and each new local helper, name the concern, find its owner in the map, and decide:

- **Filesystem / paths / file IO** → `fs` (path confinement, symlink-escape rejection, atomic writes, permissions) and `util` (copy, ensure-dir, read/write helpers). Raw `os.Open`+`filepath.EvalSymlinks`+`Rel` escape checks, `os.WriteFile` where atomicity matters, or a hand-rolled dir walk are candidate forks — check against `fs`/`util`.
- **Errors** → `errors` (`AppError`, RFC 9457, typed codes, cause via `%w`, `errors.Is/As/Join`). A fresh sentinel or custom error struct for a shared concern is a fork.
- **Resilience** → `resilience` (retry / timeout / circuit-break), not hand-rolled loops or scattered `context.WithTimeout` + custom backoff.
- **HTTP** → `httpclient` / `server`, not a raw `http.Client{}` with custom retry/timeout.
- **Subprocess** → `process` (argv-only), not a bare `exec.Command`.
- **Config / logging / di / observability** → the owning package; logging is `log/slog` via `logger`, never a fresh `log` or `fmt.Print`.
- **Collections / time / crypto / codec** → a modern stdlib primitive first (`slices`, `maps`, `cmp`, `errors.Join`, `sync.OnceValue`, `slices.SortFunc` over `sort.Slice`) or the gokit owner (`util` clock/slice helpers, `encryption`/`security`, `codec`/`schema`). Reinventing either is a fork.

The list above is illustrative, not exhaustive: the rule is *any* module, so if a hit maps to a gokit owner not named here, it still counts.

**3. Judge each candidate — reuse, enhance, add, or justify:**

- Owner covers it → **reuse**: delete the fork, call the owner. *(blocker)*
- Owner is close but inadequate → **enhance it generically** so every consumer benefits, then reuse — never fork a tweaked copy. "Almost the same" (a near-copy with one changed line, or a copied comment) is still a fork.
- No owner and the capability is genuinely foundational and generally useful → **add it to the owning module (or a new one)**, not locally — a local solution is a **should-fix** with an "upstream to the owner" note.
- Deliberately stricter/narrower policy that must not be shared → **justified local**: state why, and flag it as a candidate to promote into the owner.

An owner that **nothing imports yet** is a strong signal its intended consumers are running local forks — check it explicitly:

```bash
grep -rln 'kbukum/gokit/<owner>' --include=*.go . | grep -v _test.go | grep -v '^./<owner>/'
```

## Output for this pass

Per finding, name the concrete package/symbol that should have been used (e.g. "use `fs.ConfineExistingPath` instead of the local `ensureUnderRoot`", "use `resilience` retry policy instead of a hand-rolled loop", "wrap with `process` rather than `exec.Command`") and its outcome (reuse / enhance / add / justified-local).
