#!/usr/bin/env python3
"""Compute the set of Go modules affected by a PR's diff.

Outputs (to GITHUB_OUTPUT or stdout):

    modules         JSON array of module dirs to run check/lint/vuln on
    check_includes  JSON array of {module, os} rows for the check matrix
    is_full         "true"|"false"
    reason          short human-readable explanation

A module is "affected" if its directory contains a changed file, OR it
transitively requires (via in-repo `require` edges) a module that is.

A "full rebuild" is forced when:
  * event_name == "push"
  * any changed path matches a FULL_REBUILD_GLOB (workflows, scripts, lint
    config, security config, build orchestration files)
  * any go.mod file is added, deleted, or renamed
  * graph parsing fails or the merge-base cannot be computed
  * a module's `require` references an in-repo path that does not resolve
    to a discovered local module (suggests a missing replace)

When no module is affected (e.g. docs-only PR), a sentinel "__noop__" row
is emitted so the GitHub Actions matrix is never empty.
"""

from __future__ import annotations

import argparse
import fnmatch
import json
import os
import re
import subprocess
import sys
from collections import defaultdict, deque
from pathlib import Path
from typing import Iterable

REPO_MODULE_PREFIX = "github.com/kbukum/gokit"

# Paths whose change forces full rebuild. Glob patterns matched against
# repo-relative posix paths.
FULL_REBUILD_GLOBS: tuple[str, ...] = (
    ".github/workflows/*",
    ".github/workflows/**/*",
    ".golangci.yml",
    ".golangci.yaml",
    "gosec.toml",
    "Makefile",
    "gomod.sh",
    "scripts/*",
    "scripts/**/*",
    "go.work",
    "go.work.sum",
)

# Cross-OS coverage extras. Single source of truth for per-module OS
# expansions previously hard-coded in ci.yml. Filtered by affected set.
CROSS_OS_EXTRAS: tuple[tuple[str, str], ...] = (
    (".", "macos-latest"),
    (".", "windows-latest"),
    ("storage", "macos-latest"),
    ("server", "macos-latest"),
    ("workload", "macos-latest"),
    ("httpclient", "macos-latest"),
    ("grpc", "macos-latest"),
    ("auth", "macos-latest"),
    (".", "ubuntu-24.04-arm"),
    ("database", "ubuntu-24.04-arm"),
    ("storage", "ubuntu-24.04-arm"),
)

NOOP = "__noop__"

REQUIRE_LINE = re.compile(
    r"^\s*(github\.com/kbukum/gokit(?:/[\w./-]+)?)\s+v"
)
REPLACE_LINE = re.compile(
    r"^\s*(github\.com/kbukum/gokit(?:/[\w./-]+)?)\s+=>\s+(\S+)"
)
MODULE_LINE = re.compile(r"^\s*module\s+(\S+)")


def run(cmd: list[str], cwd: Path | None = None) -> str:
    """Run a command, return stdout. Raise on failure."""
    res = subprocess.run(
        cmd,
        cwd=cwd,
        capture_output=True,
        text=True,
        check=False,
    )
    if res.returncode != 0:
        raise RuntimeError(
            f"command failed: {' '.join(cmd)}\nstderr: {res.stderr.strip()}"
        )
    return res.stdout


def discover_modules(repo: Path) -> list[str]:
    """Return repo-relative dirs (posix style) of every go.mod in HEAD."""
    out = run(
        [
            "git",
            "ls-files",
            "--cached",
            "--others",
            "--exclude-standard",
            "*go.mod",
        ],
        cwd=repo,
    )
    mods: list[str] = []
    for line in out.splitlines():
        line = line.strip()
        if not line or "/vendor/" in line or line.startswith(".git/"):
            continue
        if not line.endswith("go.mod"):
            continue
        d = str(Path(line).parent.as_posix())
        if d == "":
            d = "."
        mods.append(d)
    return sorted(set(mods))


def parse_gomod(path: Path) -> tuple[str, list[str], dict[str, str]]:
    """Return (module_path, in_repo_requires, replace_map).

    in_repo_requires: list of `github.com/kbukum/gokit[/X]` paths required.
    replace_map: maps `github.com/kbukum/gokit[/X]` -> replacement target
                 (relative path or external).
    """
    module_path = ""
    requires: list[str] = []
    replaces: dict[str, str] = {}
    text = path.read_text(encoding="utf-8")
    in_require = False
    in_replace = False
    for raw in text.splitlines():
        line = raw.split("//", 1)[0]
        m = MODULE_LINE.match(line)
        if m:
            module_path = m.group(1)
            continue
        stripped = line.strip()
        if stripped.startswith("require ("):
            in_require = True
            continue
        if stripped.startswith("replace ("):
            in_replace = True
            continue
        if stripped == ")":
            in_require = False
            in_replace = False
            continue

        if in_require or stripped.startswith("require "):
            mr = REQUIRE_LINE.match(line.replace("require ", "", 1))
            if mr:
                requires.append(mr.group(1))
        if in_replace or stripped.startswith("replace "):
            rl = line.replace("replace ", "", 1)
            mr = REPLACE_LINE.match(rl)
            if mr:
                replaces[mr.group(1)] = mr.group(2).strip()
    return module_path, requires, replaces


def build_graph(
    repo: Path, mod_dirs: list[str]
) -> tuple[dict[str, set[str]], dict[str, str], list[str]]:
    """Build forward graph: module_dir -> set of module_dirs it depends on.

    Returns (forward_graph, module_path_to_dir, warnings).

    A warning string is emitted (non-fatal here; caller may upgrade to
    full-rebuild trigger) for any in-repo `require` that does not resolve
    to a known local module via `replace`.
    """
    path_to_dir: dict[str, str] = {}
    parsed: dict[str, tuple[list[str], dict[str, str]]] = {}
    for d in mod_dirs:
        gomod = repo / d / "go.mod" if d != "." else repo / "go.mod"
        mp, reqs, reps = parse_gomod(gomod)
        if mp:
            path_to_dir[mp] = d
        parsed[d] = (reqs, reps)

    warnings: list[str] = []
    fwd: dict[str, set[str]] = defaultdict(set)
    for d, (reqs, reps) in parsed.items():
        for req in reqs:
            if req == REPO_MODULE_PREFIX or req.startswith(
                REPO_MODULE_PREFIX + "/"
            ):
                target_dir: str | None = path_to_dir.get(req)
                if target_dir is None:
                    rep = reps.get(req)
                    if rep and (rep.startswith("./") or rep.startswith("../")):
                        resolved = (repo / d / rep).resolve()
                        try:
                            rel = resolved.relative_to(repo)
                            cand = str(rel.as_posix()) or "."
                            if cand in mod_dirs:
                                target_dir = cand
                        except ValueError:
                            pass
                if target_dir is None:
                    warnings.append(
                        f"{d}/go.mod requires {req} but no local module "
                        f"resolves it (missing replace?)"
                    )
                    continue
                if target_dir != d:
                    fwd[d].add(target_dir)
    return fwd, path_to_dir, warnings


def reverse_closure(
    seeds: set[str], fwd: dict[str, set[str]]
) -> set[str]:
    """BFS over reverse edges to expand seed set with all dependents."""
    rev: dict[str, set[str]] = defaultdict(set)
    for src, dsts in fwd.items():
        for dst in dsts:
            rev[dst].add(src)

    visited: set[str] = set()
    q: deque[str] = deque(seeds)
    while q:
        cur = q.popleft()
        if cur in visited:
            continue
        visited.add(cur)
        for dependent in rev.get(cur, ()):
            if dependent not in visited:
                q.append(dependent)
    return visited


def owner_module(rel_path: str, mod_dirs_sorted_desc: list[str]) -> str | None:
    """Return the module dir owning `rel_path` via longest-prefix match."""
    p = rel_path.replace("\\", "/")
    for d in mod_dirs_sorted_desc:
        if d == ".":
            continue
        if p == d or p.startswith(d + "/"):
            return d
    if "." in mod_dirs_sorted_desc:
        return "."
    return None


def changed_files(
    repo: Path, base_sha: str, head_sha: str
) -> tuple[list[str], list[tuple[str, str]]]:
    """Return (changed_paths, status_pairs) where status_pairs is [(status, path)].

    Status codes from `git diff --name-status`: A/M/D/R<num>/C<num>.
    Renames are reported as 'R<num>' with old path then new path; we
    surface both as separate entries (status 'D' for old, 'A' for new).
    """
    out = run(
        ["git", "diff", "--name-status", "--no-renames", f"{base_sha}...{head_sha}"],
        cwd=repo,
    )
    paths: list[str] = []
    status_pairs: list[tuple[str, str]] = []
    for line in out.splitlines():
        if not line.strip():
            continue
        parts = line.split("\t")
        status = parts[0]
        for p in parts[1:]:
            paths.append(p)
            status_pairs.append((status[0], p))
    return paths, status_pairs


def matches_full_rebuild(paths: Iterable[str]) -> str | None:
    for p in paths:
        for pat in FULL_REBUILD_GLOBS:
            if fnmatch.fnmatch(p, pat):
                return f"path matches full-rebuild glob: {p} ~ {pat}"
    return None


def emit(out_path: str | None, payload: dict[str, object]) -> None:
    """Write outputs as KEY=VALUE lines to GITHUB_OUTPUT (or stdout)."""
    lines: list[str] = []
    for k, v in payload.items():
        if isinstance(v, (list, dict)):
            value = json.dumps(v, separators=(",", ":"))
        else:
            value = str(v)
        lines.append(f"{k}={value}")
    text = "\n".join(lines) + "\n"
    if out_path:
        with open(out_path, "a", encoding="utf-8") as f:
            f.write(text)
    sys.stdout.write(text)


def build_check_includes(
    modules: list[str],
) -> list[dict[str, str]]:
    """Build the `check` matrix include list.

    Every affected module gets a default ubuntu-latest row. Then the
    cross-OS extras are appended only when the module is in the affected
    set. Yields a single matrix dimension `include:` (no implicit
    expansion).
    """
    rows: list[dict[str, str]] = []
    for m in modules:
        rows.append({"module": m, "os": "ubuntu-latest"})
    affected = set(modules)
    for mod, os_name in CROSS_OS_EXTRAS:
        if mod in affected:
            rows.append({"module": mod, "os": os_name})
    return rows


def noop_payload(reason: str) -> dict[str, object]:
    return {
        "modules": [NOOP],
        "check_includes": [{"module": NOOP, "os": "ubuntu-latest"}],
        "is_full": "false",
        "reason": reason,
    }


def full_payload(modules: list[str], reason: str) -> dict[str, object]:
    return {
        "modules": modules,
        "check_includes": build_check_includes(modules),
        "is_full": "true",
        "reason": reason,
    }


def affected_payload(
    modules: list[str], reason: str
) -> dict[str, object]:
    return {
        "modules": modules,
        "check_includes": build_check_includes(modules),
        "is_full": "false",
        "reason": reason,
    }


def compute(
    repo: Path,
    event_name: str,
    base_sha: str | None,
    head_sha: str = "HEAD",
) -> dict[str, object]:
    all_modules = discover_modules(repo)
    if not all_modules:
        return noop_payload("no go.mod files discovered")

    if event_name == "push":
        return full_payload(all_modules, "push event: full rebuild")

    if not base_sha:
        return full_payload(all_modules, "no base SHA: conservative full rebuild")

    try:
        paths, status_pairs = changed_files(repo, base_sha, head_sha)
    except RuntimeError as e:
        return full_payload(
            all_modules, f"diff failed ({e}): conservative full rebuild"
        )

    if not paths:
        return noop_payload("no changed paths")

    # Any go.mod add/delete forces full rebuild (graph topology change).
    for status, p in status_pairs:
        if p.endswith("go.mod") and status in {"A", "D"}:
            return full_payload(
                all_modules,
                f"go.mod {('added' if status == 'A' else 'deleted')}: {p}",
            )

    full_reason = matches_full_rebuild(paths)
    if full_reason:
        return full_payload(all_modules, full_reason)

    fwd, _path_to_dir, warnings = build_graph(repo, all_modules)
    if warnings:
        return full_payload(
            all_modules,
            "dependency graph warnings (treating as full): "
            + "; ".join(warnings[:3]),
        )

    mod_dirs_sorted_desc = sorted(all_modules, key=len, reverse=True)
    seeds: set[str] = set()
    unowned: list[str] = []
    for p in paths:
        owner = owner_module(p, mod_dirs_sorted_desc)
        if owner is None:
            unowned.append(p)
        else:
            seeds.add(owner)

    if not seeds:
        return noop_payload(
            f"changed paths own no module (e.g. docs): {len(unowned)} files"
        )

    affected = reverse_closure(seeds, fwd)
    affected_sorted = sorted(affected)
    return affected_payload(
        affected_sorted,
        f"affected from {sorted(seeds)} via reverse-dep closure",
    )


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--repo", default=".", help="repository root")
    ap.add_argument(
        "--event",
        default=os.environ.get("GITHUB_EVENT_NAME", "pull_request"),
        help="GitHub event name (push|pull_request|...)",
    )
    ap.add_argument(
        "--base",
        default=os.environ.get("BASE_SHA"),
        help="base SHA to diff against (e.g. PR base)",
    )
    ap.add_argument("--head", default="HEAD", help="head ref/SHA")
    ap.add_argument(
        "--output",
        default=os.environ.get("GITHUB_OUTPUT"),
        help="path to GITHUB_OUTPUT (defaults to env)",
    )
    args = ap.parse_args()
    repo = Path(args.repo).resolve()

    payload = compute(repo, args.event, args.base, args.head)

    sys.stderr.write("─── affected_modules.py ───\n")
    sys.stderr.write(f"event:    {args.event}\n")
    sys.stderr.write(f"base:     {args.base}\n")
    sys.stderr.write(f"head:     {args.head}\n")
    sys.stderr.write(f"is_full:  {payload['is_full']}\n")
    sys.stderr.write(f"reason:   {payload['reason']}\n")
    sys.stderr.write(
        f"modules:  {json.dumps(payload['modules'])}\n"
    )
    sys.stderr.write(
        f"includes: {json.dumps(payload['check_includes'])}\n"
    )
    sys.stderr.write("───────────────────────────\n")

    emit(args.output, payload)
    return 0


if __name__ == "__main__":
    sys.exit(main())
