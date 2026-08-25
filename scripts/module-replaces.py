#!/usr/bin/env python3
"""Keep intra-repo `replace` directives complete across the multi-module workspace.

Why this exists
    gokit is a multi-module monorepo. `go.work` unifies the modules for
    `go build`/`go test`, but `go mod tidy` ignores the workspace and resolves
    each module against its own `require` + `replace` graph. Any intra-repo
    `require` (a `github.com/kbukum/gokit[/...]` module) therefore needs a local
    `replace => <relative path>` so tidy resolves it from the working tree
    instead of the module proxy. Without it, tidy succeeds only while the
    required version is already published, and breaks the moment a release bump
    moves the version to an as-yet-unpublished tag.

    These `replace` directives are ignored by downstream consumers (they only
    apply when the module is the main module), so committing local relative
    paths is safe. This tool derives them from the module graph so the set is
    complete by construction instead of hand-maintained.

Usage (from anywhere in the repo):
    scripts/module-replaces.py            # fix: add any missing replaces
    scripts/module-replaces.py --check    # verify only; non-zero if incomplete

Exit codes:
    0  replace graph complete (or successfully fixed)
    1  missing replaces (--check), or a tooling/invocation error
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
MODULE_LINE = re.compile(r"^module\s+(\S+)", re.MULTILINE)


def workspace_module_dirs() -> list[Path]:
    """Module directories listed in the root go.work `use (...)` block."""
    go_work = REPO_ROOT / "go.work"
    if not go_work.exists():
        # Fall back to every go.mod in the tree (vendor/.git excluded).
        return sorted(
            p.parent
            for p in REPO_ROOT.rglob("go.mod")
            if "vendor" not in p.parts and ".git" not in p.parts
        )
    text = go_work.read_text()
    block = re.search(r"use\s*\((.*?)\)", text, re.DOTALL)
    entries: list[str] = []
    if block:
        entries += re.findall(r"[^\s()]+", block.group(1))
    entries += re.findall(r"^use\s+(\S+)", text, re.MULTILINE)
    dirs: list[Path] = []
    seen: set[Path] = set()
    for entry in entries:
        d = (REPO_ROOT / entry).resolve()
        if (d / "go.mod").exists() and d not in seen:
            seen.add(d)
            dirs.append(d)
    return dirs


def module_path(mod_dir: Path) -> str | None:
    m = MODULE_LINE.search((mod_dir / "go.mod").read_text())
    return m.group(1) if m else None


def mod_edit_json(mod_dir: Path) -> dict:
    out = subprocess.check_output(
        ["go", "mod", "edit", "-json"], cwd=mod_dir, text=True
    )
    return json.loads(out)


def find_missing(mod_dir: Path, path_to_dir: dict[str, Path]) -> list[tuple[str, str]]:
    """Return (require_path, relative_replace_target) for each missing replace."""
    data = mod_edit_json(mod_dir)
    self_path = module_path(mod_dir)
    replaced = {r["Old"]["Path"] for r in (data.get("Replace") or [])}
    missing: list[tuple[str, str]] = []
    for req in data.get("Require") or []:
        path = req["Path"]
        if path == self_path or path not in path_to_dir or path in replaced:
            continue
        rel = os.path.relpath(path_to_dir[path], mod_dir)
        missing.append((path, rel))
    return missing


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check",
        action="store_true",
        help="verify completeness without modifying any go.mod (for CI)",
    )
    args = parser.parse_args()

    mod_dirs = workspace_module_dirs()
    path_to_dir = {p: d for d in mod_dirs if (p := module_path(d))}

    total_missing = 0
    for mod_dir in mod_dirs:
        missing = find_missing(mod_dir, path_to_dir)
        if not missing:
            continue
        total_missing += len(missing)
        rel_mod = mod_dir.relative_to(REPO_ROOT)
        for path, rel in missing:
            verb = "MISSING" if args.check else "add"
            print(f"{verb}: {rel_mod}: replace {path} => {rel}")
            if not args.check:
                subprocess.check_call(
                    ["go", "mod", "edit", f"-replace={path}={rel}"], cwd=mod_dir
                )

    if total_missing == 0:
        print("replace graph complete: every intra-repo require has a local replace")
        return 0
    if args.check:
        print(
            f"\n{total_missing} missing replace directive(s). "
            "Run 'make replace-sync' and commit the result.",
            file=sys.stderr,
        )
        return 1
    print(f"\nadded {total_missing} replace directive(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
