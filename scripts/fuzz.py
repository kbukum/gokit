#!/usr/bin/env python3
"""Plan and run gokit's Go fuzz targets in balanced groups.

Every `func FuzzXxx` in a module's `_test.go` files is one target. Targets are
split into contiguous groups of about `--size` each, so CI runs the groups as a
parallel matrix and every target runs in exactly one group.

Usage (from anywhere in the repo):
    scripts/fuzz.py plan [--size 8]                 # JSON: targets, groups, modules
    scripts/fuzz.py list [--group K --groups N]     # one target per line
    scripts/fuzz.py run --group K --groups N [--fuzztime 30s] [--failures DIR]

Modules come from `--modules-json` (a JSON array of module roots), else from
Toven: with `--base REF` (or FUZZ_BASE) only the modules whose tests are
affected since REF, otherwise every Go module.

Exit codes:
    0  every run target passed (or the plan/list succeeded)
    1  a target found a failing input or did not build
    2  invalid invocation
"""

from __future__ import annotations

import argparse
import dataclasses
import enum
import json
import math
import os
import pathlib
import re
import shutil
import subprocess
import sys
from collections.abc import Iterable, Sequence

ROOT = pathlib.Path(__file__).resolve().parent.parent
FUZZ_FUNC = re.compile(r"^func (Fuzz[A-Za-z0-9_]+)\(", re.MULTILINE)
FAILING_INPUT = re.compile(r"Failing input written to (\S+)")
# When -fuzztime ends while workers shut down, Go can fail the target with only
# this message, printed indented under its "--- FAIL:" line and followed by the
# package summary. That is a harness race, not a finding. A test's own error
# carries a "file.go:N:" prefix and does not match.
DEADLINE_RACE = re.compile(r"^\s+context deadline exceeded\s*$", re.MULTILINE)
SKIP_DIRS = {"testdata", "vendor", ".git", "node_modules"}
DURATION = re.compile(r"([0-9]+)(ms|s|m|h)")
UNIT_SECONDS = {"ms": 0.001, "s": 1, "m": 60, "h": 3600}
# Fuzzing time one shard may spend, leaving the rest of the CI fuzz job's
# 120-minute limit for setup, builds and corpus upload.
SHARD_BUDGET_SECONDS = 90 * 60


@dataclasses.dataclass(frozen=True, order=True)
class Target:
    module: str
    package: str
    name: str

    def __str__(self) -> str:
        return f"{self.module} {self.package} {self.name}"


class Verdict(enum.Enum):
    PASS = "pass"
    CRASH = "crash"
    ERROR = "error"


def discover(root: pathlib.Path, modules: Iterable[str]) -> list[Target]:
    """Every fuzz target owned by `modules` (roots relative to `root`)."""
    roots = sorted({_norm(m) for m in modules})
    every_module = {_norm(str(p.parent.relative_to(root))) for p in root.rglob("go.mod")}
    targets: list[Target] = []
    for module in roots:
        base = root / module
        for test in _test_files(base, root, every_module - {module}):
            names = sorted(set(FUZZ_FUNC.findall(test.read_text(encoding="utf-8"))))
            package = _package(test.parent.relative_to(base))
            targets.extend(Target(module, package, name) for name in names)
    return sorted(targets)


def fuzz_seconds(text: str) -> float:
    """Seconds in a positive Go duration such as 30s or 1m30s; anything else is refused."""
    if not re.fullmatch(r"(?:[0-9]+(?:ms|s|m|h))+", text):
        raise ValueError(f"fuzztime must be a Go duration such as 30s or 2m, got {text!r}")
    seconds = sum(int(n) * UNIT_SECONDS[unit] for n, unit in DURATION.findall(text))
    if seconds <= 0:
        raise ValueError("fuzztime must be positive; Go fuzzes without end at zero")
    return seconds


def group_count(targets: int, size: int) -> int:
    return math.ceil(targets / size) if targets else 0


def split(targets: Sequence[Target], groups: int) -> list[list[Target]]:
    """Contiguous, balanced groups; targets of one package stay mostly together."""
    if groups < 0 or groups > len(targets) or (groups == 0 and targets):
        raise ValueError(f"cannot split {len(targets)} targets into {groups} groups")
    ordered = sorted(targets)
    base, extra = divmod(len(ordered), groups) if groups else (0, 0)
    result, start = [], 0
    for index in range(groups):
        end = start + base + (1 if index < extra else 0)
        result.append(ordered[start:end])
        start = end
    flat = [t for group in result for t in group]
    if flat != ordered:
        raise AssertionError("fuzz grouping lost or duplicated a target")
    return result


def verdict(code: int, output: str) -> Verdict:
    if code == 0:
        return Verdict.PASS
    if FAILING_INPUT.search(output):
        return Verdict.CRASH
    if code == 1 and output.count("--- FAIL:") == 1 and DEADLINE_RACE.search(output):
        return Verdict.PASS
    return Verdict.ERROR


def failing_inputs(output: str) -> list[str]:
    return FAILING_INPUT.findall(output)


def _norm(module: str) -> str:
    module = module.strip().strip("/")
    return "." if module in ("", ".") else module.removeprefix("./")


def _package(rel: pathlib.Path) -> str:
    text = rel.as_posix()
    return "." if text == "." else f"./{text}"


def _test_files(base: pathlib.Path, root: pathlib.Path, others: set[str]) -> Iterable[pathlib.Path]:
    for directory, dirs, files in os.walk(base):
        here = pathlib.Path(directory)
        keep = []
        for name in sorted(dirs):
            rel = _norm(str((here / name).relative_to(root)))
            if name in SKIP_DIRS or name.startswith(".") or rel in others:
                continue
            keep.append(name)
        dirs[:] = keep
        for name in sorted(files):
            if name.endswith("_test.go"):
                yield here / name


def module_query(toven: str, base: str | None) -> list[str]:
    """Toven command listing every module, or only those affected since base."""
    if base is None:
        return [toven, "--output", "jsonl", "modules"]
    return [toven, "--output", "jsonl", "affected", "test", "--base", base, "--merge-base"]


def go_module_roots(output: str) -> list[str]:
    records = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
    return [r["path"] for r in records if r["module"].startswith("go:")]


def _modules(args: argparse.Namespace) -> list[str]:
    if args.modules_json:
        modules = json.loads(args.modules_json)
        if not isinstance(modules, list) or not all(isinstance(m, str) for m in modules):
            raise SystemExit("--modules-json must be a JSON array of module roots")
        return modules
    output = subprocess.run(
        module_query(args.toven, args.base),
        cwd=ROOT, capture_output=True, text=True, check=True, timeout=120,
    ).stdout
    return go_module_roots(output)


def _selected(args: argparse.Namespace) -> list[Target]:
    targets = discover(ROOT, _modules(args))
    if args.group is None:
        return targets
    if not 1 <= args.group <= args.groups:
        raise SystemExit(f"--group must be within 1..{args.groups}")
    return split(targets, args.groups)[args.group - 1]


def _run(targets: Sequence[Target], fuzztime: str, failures: pathlib.Path | None) -> int:
    grouped = os.environ.get("GITHUB_ACTIONS") == "true"
    failed = 0
    for target in targets:
        print(f"::group::{target}" if grouped else f"==> {target}", flush=True)
        argv = ["go", "-C", target.module, "test", "-run=^$", f"-fuzz=^{target.name}$",
                f"-fuzztime={fuzztime}", target.package]
        result = subprocess.run(argv, cwd=ROOT, capture_output=True, text=True, check=False)
        output = result.stdout + result.stderr
        print(output, end="" if output.endswith("\n") else "\n")
        if grouped:
            print("::endgroup::")
        outcome = verdict(result.returncode, output)
        if outcome is Verdict.PASS:
            continue
        failed += 1
        message = "found a failing input" if outcome is Verdict.CRASH else f"failed (exit {result.returncode})"
        print(f"::error::{target} {message}" if grouped else f"FAIL {target} {message}", flush=True)
        if failures is not None:
            _keep_inputs(target, failing_inputs(output), failures)
    print(f"{len(targets) - failed}/{len(targets)} fuzz targets passed")
    return 1 if failed else 0


def _keep_inputs(target: Target, inputs: Iterable[str], failures: pathlib.Path) -> None:
    package_dir = ROOT / target.module / target.package
    for rel in inputs:
        source = (package_dir / rel).resolve()
        if not source.is_relative_to(ROOT) or not source.is_file():
            continue
        destination = failures / source.relative_to(ROOT)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, destination)


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--toven", default=os.environ.get("TOVEN", "toven"))
    parser.add_argument("--modules-json", default=os.environ.get("FUZZ_MODULES") or None)
    parser.add_argument("--base", default=os.environ.get("FUZZ_BASE") or None,
                        help="fuzz only modules affected since this Git ref")
    sub = parser.add_subparsers(dest="command", required=True)
    plan = sub.add_parser("plan")
    plan.add_argument("--size", type=int, default=8)
    plan.add_argument("--fuzztime", default="30s")
    for name in ("list", "run"):
        command = sub.add_parser(name)
        command.add_argument("--group", type=int)
        command.add_argument("--groups", type=int)
        if name == "run":
            command.add_argument("--fuzztime", default="30s")
            command.add_argument("--failures", type=pathlib.Path)
    args = parser.parse_args(argv)

    if args.command == "plan":
        if args.size < 1:
            parser.error("--size must be positive")
        try:
            seconds = fuzz_seconds(args.fuzztime)
        except ValueError as err:
            parser.error(str(err))
        modules = _modules(args)
        count = len(discover(ROOT, modules))
        groups = group_count(count, args.size)
        # Balanced groups hold at most size targets, run one after another.
        if seconds * min(args.size, count) > SHARD_BUDGET_SECONDS:
            parser.error(f"fuzztime {args.fuzztime} for up to {args.size} targets per shard exceeds the "
                         f"{SHARD_BUDGET_SECONDS // 60}-minute shard budget")
        print(json.dumps({"targets": count, "groups": groups, "group": list(range(1, groups + 1)),
                          "modules": modules, "fuzztime": args.fuzztime}))
        return 0
    if (args.group is None) != (args.groups is None):
        parser.error("--group and --groups go together")
    if args.command == "run":
        try:
            fuzz_seconds(args.fuzztime)
        except ValueError as err:
            parser.error(str(err))
    targets = _selected(args)
    if args.command == "list":
        for target in targets:
            print(target)
        return 0
    return _run(targets, args.fuzztime, args.failures)


if __name__ == "__main__":
    sys.exit(main())
