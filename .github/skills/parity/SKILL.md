---
name: parity
description: "gokit: Align a capability with sibling kits while preserving idiomatic APIs and tracking deliberate gaps."
user-invocable: true
---

# Align sibling capabilities

Compare gokit with [rskit](https://github.com/kbukum/rskit) by behavior and contracts, not symbol or folder identity. Neither kit is the default reference: align toward the strongest implementation without weakening either.

## Choose the scope

| Level | Use when |
|---|---|
| Full | The infrastructure capability is equally suitable in both languages |
| Light | Go covers common use; heavy work belongs elsewhere, such as media transcoding in Rust |
| Kit-only | The capability is language- or framework-specific, such as ConnectRPC or Axum |

Project consistency comes first, then current Go idioms, then structural parity. Preserve required wire/protocol compatibility, but do not force foreign internal representations or public `any` APIs.

## Apply

1. Inspect both implementations' APIs, invariants, failure behavior, and ownership.
2. Choose the level and direction; retain deliberate kit-only distinctions.
3. Implement idiomatically using typed contracts, canonical owners, and the [baseline](../../copilot-instructions.md).
4. Record public abstraction changes and intentional divergence in rskit's tracking issues, using full URLs. Do not claim parity without evidence.
5. Identify improvements for whichever kit remains behind; keep both generic and runtime-independent.
6. Run affected [validation](../validate/SKILL.md) and review the actual parity claim.

Use shared names where natural, and document intentional mappings. Branches, commits, and PRs describe the capability, not internal plan numbers. Publication and cross-repository mutations require authorization.
