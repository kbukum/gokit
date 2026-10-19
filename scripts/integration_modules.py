#!/usr/bin/env python3
"""Print the Toven keys of modules that hold `//go:build integration` files.

The integration suite only needs these modules; the rest have nothing the tag
adds, and running them again would repeat their unit tests.

Usage: scripts/integration_modules.py [--toven BIN]
"""

from __future__ import annotations

import argparse
import json
import os
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
BUILD_LINE = re.compile(r"^//go:build\s+(.*)$")
TAG = re.compile(r"(!?)\bintegration\b(?![\w.])")
SKIP_DIRS = {"testdata", "vendor", "node_modules"}


def tagged(path: pathlib.Path) -> bool:
    """Whether the file's build constraint requires (not excludes) `integration`."""
    with path.open(encoding="utf-8") as source:
        for line in source:
            stripped = line.strip()
            if stripped.startswith("package "):
                return False
            match = BUILD_LINE.match(stripped)
            if match:
                return any(not neg for neg in TAG.findall(match.group(1)))
    return False


def owning_modules(root: pathlib.Path, modules: dict[str, str]) -> list[str]:
    """Keys of `modules` (root path -> key) that own an integration-tagged file."""
    found: set[str] = set()
    for directory, dirs, files in os.walk(root):
        here = pathlib.Path(directory)
        dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS and not d.startswith(".")
                         and not _foreign_module(root, here / d, modules))
        for name in files:
            if name.endswith(".go") and tagged(here / name):
                rel = (here / name).relative_to(root).parent.as_posix()
                owner = _owner(rel, modules)
                if owner:
                    found.add(owner)
    return sorted(found)


def _foreign_module(root: pathlib.Path, directory: pathlib.Path, modules: dict[str, str]) -> bool:
    """A Go module Toven does not manage (such as the examples workspace)."""
    return (directory / "go.mod").is_file() and directory.relative_to(root).as_posix() not in modules


def _owner(rel: str, modules: dict[str, str]) -> str | None:
    while True:
        if rel in modules:
            return modules[rel]
        if rel in ("", "."):
            return None
        rel = rel.rpartition("/")[0] or "."


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--toven", default=os.environ.get("TOVEN", "toven"))
    args = parser.parse_args()
    output = subprocess.run(
        [args.toven, "--output", "jsonl", "modules"],
        cwd=ROOT, capture_output=True, text=True, check=True, timeout=120,
    ).stdout
    records = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
    modules = {r["path"]: r["module"] for r in records if r["module"].startswith("go:")}
    for key in owning_modules(ROOT, modules):
        print(key)
    return 0


if __name__ == "__main__":
    sys.exit(main())
