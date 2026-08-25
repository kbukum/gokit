"""Tests for scripts/module-replaces.py.

Covers missing-replace detection and relative-path computation against real
throwaway modules driven through `go mod edit`.
"""

from __future__ import annotations

import importlib.util
import pathlib
import subprocess
import sys

import pytest

HERE = pathlib.Path(__file__).resolve().parent

# Hyphenated filename → load via importlib rather than a plain import.
_spec = importlib.util.spec_from_file_location(
    "module_replaces", HERE / "module-replaces.py"
)
assert _spec and _spec.loader
mr = importlib.util.module_from_spec(_spec)
sys.modules["module_replaces"] = mr
_spec.loader.exec_module(mr)


def _init_module(path: pathlib.Path, module_path: str) -> None:
    path.mkdir(parents=True, exist_ok=True)
    (path / "go.mod").write_text(f"module {module_path}\n\ngo 1.26.0\n")


def _require(mod_dir: pathlib.Path, path: str, version: str = "v0.2.0") -> None:
    subprocess.check_call(
        ["go", "mod", "edit", f"-require={path}@{version}"], cwd=mod_dir
    )


def _replace(mod_dir: pathlib.Path, path: str, target: str) -> None:
    subprocess.check_call(
        ["go", "mod", "edit", f"-replace={path}={target}"], cwd=mod_dir
    )


@pytest.fixture(autouse=True)
def _require_go() -> None:
    if subprocess.run(["go", "version"], capture_output=True).returncode != 0:
        pytest.skip("go toolchain not available")


def test_detects_missing_intra_repo_replace(tmp_path: pathlib.Path) -> None:
    testutil = tmp_path / "testutil"
    consumer = tmp_path / "sub" / "consumer"
    _init_module(testutil, "example.com/kit/testutil")
    _init_module(consumer, "example.com/kit/consumer")
    _require(consumer, "example.com/kit/testutil")

    path_to_dir = {
        "example.com/kit/testutil": testutil,
        "example.com/kit/consumer": consumer,
    }

    missing = mr.find_missing(consumer, path_to_dir)
    assert missing == [("example.com/kit/testutil", "../../testutil")]


def test_no_missing_when_replace_present(tmp_path: pathlib.Path) -> None:
    testutil = tmp_path / "testutil"
    consumer = tmp_path / "consumer"
    _init_module(testutil, "example.com/kit/testutil")
    _init_module(consumer, "example.com/kit/consumer")
    _require(consumer, "example.com/kit/testutil")
    _replace(consumer, "example.com/kit/testutil", "../testutil")

    path_to_dir = {
        "example.com/kit/testutil": testutil,
        "example.com/kit/consumer": consumer,
    }
    assert mr.find_missing(consumer, path_to_dir) == []


def test_ignores_external_requires(tmp_path: pathlib.Path) -> None:
    consumer = tmp_path / "consumer"
    _init_module(consumer, "example.com/kit/consumer")
    _require(consumer, "github.com/google/uuid", "v1.6.0")

    # External module is absent from the intra-repo map → never flagged.
    assert mr.find_missing(consumer, {"example.com/kit/consumer": consumer}) == []
