#!/usr/bin/env python3
"""Wrapper around `govulncheck` that supports a documented suppression list.

Why not just use `govulncheck` directly?
    Some advisories have no upstream fix and are not reachable in our usage
    (e.g. server-side daemon CVEs imported transitively via a client library).
    govulncheck has no native suppression mechanism in v1.1.x. This wrapper
    fills that gap *narrowly*: every suppression must be justified, scoped to
    a module, given an expiry date, and — for advisories that govulncheck
    still flags as reachable — explicitly opted-in via `accept_reachable`
    with a tracking issue in `references`.

Usage (from a module directory):
    scripts/govulncheck.py --module workload -- ./...

Suppression file:
    .github/govulncheck-suppressions.json (relative to repo root)

Exit codes:
    0  no unsuppressed vulns
    1  unsuppressed vulns found, OR an expired suppression, OR config error
    2  govulncheck itself failed (binary missing, invocation error, etc.)
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import pathlib
import subprocess
import sys
from typing import Iterable


REPO_ROOT_MARKER = ".github"


def find_repo_root(start: pathlib.Path) -> pathlib.Path:
    cur = start.resolve()
    for p in (cur, *cur.parents):
        if (p / REPO_ROOT_MARKER).is_dir() and (p / "go.mod").exists():
            return p
    raise SystemExit(f"could not locate repo root from {start}")


def load_suppressions(path: pathlib.Path) -> list[dict]:
    if not path.exists():
        return []
    try:
        data = json.loads(path.read_text())
    except json.JSONDecodeError as e:
        raise SystemExit(f"suppression file {path} is not valid JSON: {e}")
    raw = data.get("suppressions", [])
    if not isinstance(raw, list):
        raise SystemExit(f"suppression file {path}: 'suppressions' must be a list")
    out: list[dict] = []
    for i, entry in enumerate(raw):
        for required in ("id", "modules", "reason", "expires"):
            if required not in entry:
                raise SystemExit(
                    f"suppression file {path}: entry #{i} missing required key {required!r}"
                )
        try:
            entry["expires_date"] = dt.date.fromisoformat(entry["expires"])
        except ValueError as e:
            raise SystemExit(
                f"suppression file {path}: entry {entry['id']!r} has invalid expires {entry['expires']!r}: {e}"
            )
        if not isinstance(entry["modules"], list) or not entry["modules"]:
            raise SystemExit(
                f"suppression file {path}: entry {entry['id']!r} 'modules' must be non-empty list"
            )
        entry.setdefault("accept_reachable", False)
        if not isinstance(entry["accept_reachable"], bool):
            raise SystemExit(
                f"suppression file {path}: entry {entry['id']!r} 'accept_reachable' must be bool"
            )
        out.append(entry)
    return out


def applicable_suppression(suppressions: list[dict], module: str, osv_id: str) -> dict | None:
    for s in suppressions:
        if s["id"] == osv_id and module in s["modules"]:
            return s
    return None


def run_govulncheck(extra_args: list[str]) -> tuple[int, str, str]:
    cmd = ["govulncheck", "-format", "json", *extra_args]
    proc = subprocess.run(cmd, capture_output=True, text=True)
    return proc.returncode, proc.stdout, proc.stderr


def parse_ndjson(stream: str) -> Iterable[dict]:
    """Parse govulncheck's JSON output.

    govulncheck emits a *stream* of pretty-printed JSON objects (not NDJSON):
    each top-level object spans multiple lines. We use raw_decode to peel them
    off one at a time. Defensive: skip past any non-JSON noise on a line so
    the wrapper never crashes the CI on a malformed message.
    """
    decoder = json.JSONDecoder()
    idx = 0
    n = len(stream)
    while idx < n:
        while idx < n and stream[idx].isspace():
            idx += 1
        if idx >= n:
            break
        try:
            obj, end = decoder.raw_decode(stream, idx)
        except json.JSONDecodeError:
            nl = stream.find("\n", idx)
            if nl == -1:
                break
            idx = nl + 1
            continue
        yield obj
        idx = end


def collect_findings(messages: Iterable[dict]) -> dict[str, list[dict]]:
    by_id: dict[str, list[dict]] = {}
    for msg in messages:
        finding = msg.get("finding")
        if not finding:
            continue
        osv = finding.get("osv")
        if not osv:
            continue
        by_id.setdefault(osv, []).append(finding)
    return by_id


def is_called(findings: list[dict]) -> bool:
    """A finding is 'called' (reachable) if any trace has a non-empty function."""
    for f in findings:
        for t in f.get("trace") or []:
            if t.get("function"):
                return True
    return False


def main(argv: list[str]) -> int:
    p = argparse.ArgumentParser(description="govulncheck wrapper with suppression support")
    p.add_argument("--module", required=True, help="module name as referenced in suppressions[].modules")
    p.add_argument(
        "--suppressions",
        default=None,
        help="path to suppression JSON (default: <repo-root>/.github/govulncheck-suppressions.json)",
    )
    p.add_argument(
        "--today",
        default=None,
        help="override today's date (YYYY-MM-DD) for testing expiry handling",
    )
    p.add_argument("govulncheck_args", nargs=argparse.REMAINDER, help="args passed to govulncheck (use -- to separate)")
    args = p.parse_args(argv)

    repo_root = find_repo_root(pathlib.Path.cwd())
    sup_path = pathlib.Path(args.suppressions) if args.suppressions else repo_root / ".github" / "govulncheck-suppressions.json"
    suppressions = load_suppressions(sup_path)

    today = dt.date.fromisoformat(args.today) if args.today else dt.date.today()
    expired = [s for s in suppressions if s["expires_date"] < today and args.module in s["modules"]]
    for s in expired:
        print(
            f"::error::govulncheck suppression {s['id']} for module {args.module} expired on {s['expires']}; "
            "re-evaluate the advisory and either remove the entry or extend the expiry with fresh justification.",
            file=sys.stderr,
        )

    extra = list(args.govulncheck_args)
    if extra and extra[0] == "--":
        extra = extra[1:]
    if not extra:
        extra = ["./..."]

    rc, stdout, stderr = run_govulncheck(extra)
    if rc not in (0, 3):
        sys.stderr.write(stderr)
        sys.stdout.write(stdout)
        return 2

    by_id = collect_findings(parse_ndjson(stdout))

    suppressed: list[tuple[str, dict]] = []
    unsuppressed: list[tuple[str, list[dict]]] = []
    for osv_id, findings in sorted(by_id.items()):
        sup = applicable_suppression(suppressions, args.module, osv_id)
        if sup is None:
            unsuppressed.append((osv_id, findings))
            continue
        reachable = is_called(findings)
        if not reachable:
            suppressed.append((osv_id, sup))
            continue
        if not sup.get("accept_reachable"):
            unsuppressed.append((osv_id, findings))
            print(
                f"::warning::suppression {osv_id} for module {args.module} matched but the advisory is REACHABLE in this codebase; "
                "treating as unsuppressed. Set accept_reachable: true with explicit justification, or fix the call.",
                file=sys.stderr,
            )
            continue
        if not sup.get("references"):
            print(
                f"::error::suppression {osv_id} for module {args.module} sets accept_reachable but has no references; "
                "add a tracking issue or upstream advisory link before accepting a reachable finding.",
                file=sys.stderr,
            )
            unsuppressed.append((osv_id, findings))
            continue
        suppressed.append((osv_id, sup))
        print(
            f"::warning::accepting REACHABLE advisory {osv_id} for module {args.module} per documented "
            f"suppression (expires {sup['expires']}). Verify the rationale still holds at expiry.",
            file=sys.stderr,
        )

    print(f"\n=== govulncheck report (module: {args.module}) ===")
    print(f"  total advisories: {len(by_id)}")
    print(f"  suppressed:       {len(suppressed)}")
    print(f"  unsuppressed:     {len(unsuppressed)}")

    if suppressed:
        print("\nSuppressed (documented):")
        for osv_id, s in suppressed:
            print(f"  - {osv_id}  expires={s['expires']}  reason={s['reason'][:120].replace(chr(10), ' ')}...")

    if unsuppressed:
        print("\nUnsuppressed findings:")
        for osv_id, findings in unsuppressed:
            reachable = "REACHABLE" if is_called(findings) else "imported"
            mods = sorted({(t.get("module") or "?") for f in findings for t in (f.get("trace") or [])})
            print(f"  - {osv_id}  [{reachable}]  modules={mods}")
        return 1

    if expired:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
