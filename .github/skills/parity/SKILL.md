---
name: parity
description: >-
    Align gokit with its sibling kit rskit by capability, not blindly — mirror the
    strongest existing implementation for a given scope, keep gokit idiomatic Go, and track parity
    through rskit tracking issues. Use when porting or aligning a module with a sibling
    counterpart, deciding whether something should be shared or stay kit-only, or when touching
    anything that has a cross-kit parity row.
user-invocable: true
---

# Cross-kit parity for gokit

gokit is a sibling kit to rskit (Rust): the same capabilities and the same engineering baseline, each expressed idiomatically per language. The goal of parity is **intuition transfer** — a user fluent in one kit should adapt to the other quickly because the concepts, shapes, and behavior line up. rskit lives at https://github.com/kbukum/rskit.

**Neither kit is the canonical reference.** Parity is judged per capability, and for each scope the kit with the *better, more complete, more correct* implementation is the one the other mirrors. Parity levels the kits **up, never down** — never weaken or simplify a stronger gokit implementation just to match a weaker sibling, and when rskit is stronger in some scope, bring gokit up to it rather than settle. If gokit is ahead in a scope, the parity direction flows outward from gokit; flag it for rskit.

## Parity is scoped and capability-first, not symbol-for-symbol

Parity is judged **by capability**, weighing where each language is strongest — not by copying every sibling symbol. Decide per capability:

- **Fully mirror** when Go is equally capable and the concept is shared infrastructure (errors, config, di, provider shapes, resilience, transport, data adapters). Whichever kit currently has the best version of that capability sets the shape the others match.
- **Light version** when Go can cover the common case but the heavy work belongs in another language. **Media is the canonical example:** gokit `media` is a light standalone module — detection, metadata, cheap image ops, time/spatial types, subtitles (SRT/VTT). Heavy audio/video/matrix transcoding and codec/filter/pipeline vocabulary are Rust-strongest and stay **rskit-only** by design.
- **Intentionally kit-only** when a concept is framework- or language-specific (e.g. rskit `http` is Axum-specific and folded into gokit `server`; gokit `connect` is ConnectRPC-specific with no rskit peer). Record these as deliberate `➖` rows with a note, not gaps to close.

When in doubt about a heavy capability, prefer the light-gokit / strong-elsewhere split.

## Idiom beats structural mimicry

Parity is about **capability and behavior**, not identical names or directory layouts. Where a language's current idioms, conventions, or best practices differ, follow Go's convention rather than force structural sameness — naming conventions, file/folder organization, error/option ergonomics, and module layout should read as native Go. Match the *concept and behavior* so users get intuition transfer; do not transliterate Rust. Call out any deliberate rename or reshaping in the rskit tracking issue so the mapping stays discoverable.

## Workflow

1. **Find the strongest existing implementation.** Locate the counterpart in the sibling kit (rskit crate in the [rskit repo](https://github.com/kbukum/rskit)) and study the public API, invariants, and error model — not just the surface — then decide which side currently has the better implementation for this scope.
2. **Decide the mirroring level** (full / light / kit-only) and the direction (bring gokit up to a stronger sibling, or lift gokit's stronger version outward) using the rules above.
3. **Implement idiomatically in Go** — generics-first, typed errors (`AppError`/RFC 9457), options constructors, no `any` in public APIs. Do not transliterate Rust; match behavior and invariants, express them the Go way. Enforce documented value invariants in code (saturate/clamp, NaN guards, half-open ranges) as gokit `media` does.
4. **Track parity in rskit's tracking issues.** When a public abstraction changes, open or annotate a tracking issue in the rskit repo (https://github.com/kbukum/rskit) recording the capability, the mirroring level, and any intentional divergence. Reference it by full URL.
5. **Propagate to whichever kit is behind.** If aligning exposes a gap, weakness, or redesign opportunity in a sibling — including gokit itself — note it for that kit so the weaker side is brought up. Keep each kit generic and multi-purpose; never make one kit specific to another.

## Naming and cross-references

- Module/package names align across kits **where the idiom allows** (gokit `logging` matches rskit `logging`, etc.). Preserve shared naming when it is natural in Go; call out any deliberate, idiom-driven rename in the rskit tracking issue.
- In PR/issue text, reference items in rskit (or any other repo) with **full URLs**, never bare `#123` — a bare number resolves to the current repo.
- Do not name branches/commits/PRs after internal plan or batch numbers; name by the actual capability change. Each PR must read standalone.

## Validate

Run the affected gokit modules through toven (see `validate`) and,
for a real audit of the parity claim, the `review` skill:

```bash
toven test --module go:<module> -- -race -count=1 -shuffle=on
```

Per repo workflow, **create the branch and make edits only** — the maintainer commits and pushes.
