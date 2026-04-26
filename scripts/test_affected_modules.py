"""Tests for scripts/affected_modules.py.

Run with:  python3 -m pytest scripts/test_affected_modules.py -v

Avoids any network/Go dependencies — fixtures fabricate a repo on disk
with hand-crafted go.mod files and exercise the pure logic.
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

import pytest

SCRIPT_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPT_DIR))

import affected_modules as am  # noqa: E402


# ──────────────────────────────────────────────────────────────────────────
# Fixture: a fake gokit-shaped repo
# ──────────────────────────────────────────────────────────────────────────


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def git(repo: Path, *args: str) -> str:
    res = subprocess.run(
        ["git", *args],
        cwd=repo,
        capture_output=True,
        text=True,
        check=True,
        env={
            "GIT_AUTHOR_NAME": "t",
            "GIT_AUTHOR_EMAIL": "t@t",
            "GIT_COMMITTER_NAME": "t",
            "GIT_COMMITTER_EMAIL": "t@t",
            "PATH": "/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin",
        },
    )
    return res.stdout


@pytest.fixture()
def repo(tmp_path: Path) -> Path:
    r = tmp_path / "repo"
    r.mkdir()
    git(r, "init", "-q", "-b", "main")

    # Root module.
    write(r / "go.mod", "module github.com/kbukum/gokit\n\ngo 1.26.0\n")
    write(r / "README.md", "# repo\n")
    write(r / "Makefile", "all:\n\techo ok\n")
    write(r / ".github" / "workflows" / "ci.yml", "name: CI\n")
    write(r / "scripts" / "tool.sh", "#!/bin/sh\n")

    # auth module — depends on root.
    write(
        r / "auth" / "go.mod",
        "module github.com/kbukum/gokit/auth\n\n"
        "go 1.26.0\n\n"
        "require github.com/kbukum/gokit v0.1.0\n\n"
        "replace github.com/kbukum/gokit => ../\n",
    )
    write(r / "auth" / "auth.go", "package auth\n")

    # auth/oidc — depends on auth (and root via auth).
    write(
        r / "auth" / "oidc" / "go.mod",
        "module github.com/kbukum/gokit/auth/oidc\n\n"
        "go 1.26.0\n\n"
        "require (\n"
        "\tgithub.com/kbukum/gokit v0.1.0\n"
        "\tgithub.com/kbukum/gokit/auth v0.1.0\n"
        ")\n\n"
        "replace (\n"
        "\tgithub.com/kbukum/gokit => ../../\n"
        "\tgithub.com/kbukum/gokit/auth => ../\n"
        ")\n",
    )
    write(r / "auth" / "oidc" / "oidc.go", "package oidc\n")

    # storage — independent.
    write(
        r / "storage" / "go.mod",
        "module github.com/kbukum/gokit/storage\n\ngo 1.26.0\n",
    )
    write(r / "storage" / "s.go", "package storage\n")

    git(r, "add", "-A")
    git(r, "commit", "-q", "-m", "init")
    return r


def base_sha(repo: Path) -> str:
    return git(repo, "rev-parse", "HEAD").strip()


def commit_change(repo: Path, files: dict[str, str | None]) -> str:
    """Apply file changes (None = delete) and commit. Returns new HEAD."""
    for rel, content in files.items():
        path = repo / rel
        if content is None:
            git(repo, "rm", "-q", "-f", rel)
        else:
            write(path, content)
            git(repo, "add", rel)
    git(repo, "commit", "-q", "-m", "change")
    return git(repo, "rev-parse", "HEAD").strip()


# ──────────────────────────────────────────────────────────────────────────
# discover_modules
# ──────────────────────────────────────────────────────────────────────────


def test_discover_modules_includes_root_and_subs(repo: Path) -> None:
    mods = am.discover_modules(repo)
    assert mods == sorted({".", "auth", "auth/oidc", "storage"})


# ──────────────────────────────────────────────────────────────────────────
# parse_gomod
# ──────────────────────────────────────────────────────────────────────────


def test_parse_gomod_block_require(repo: Path) -> None:
    mp, reqs, reps = am.parse_gomod(repo / "auth" / "oidc" / "go.mod")
    assert mp == "github.com/kbukum/gokit/auth/oidc"
    assert "github.com/kbukum/gokit" in reqs
    assert "github.com/kbukum/gokit/auth" in reqs
    assert reps["github.com/kbukum/gokit/auth"] == "../"


def test_parse_gomod_single_line_require(repo: Path) -> None:
    mp, reqs, reps = am.parse_gomod(repo / "auth" / "go.mod")
    assert mp == "github.com/kbukum/gokit/auth"
    assert reqs == ["github.com/kbukum/gokit"]
    assert reps == {"github.com/kbukum/gokit": "../"}


# ──────────────────────────────────────────────────────────────────────────
# build_graph
# ──────────────────────────────────────────────────────────────────────────


def test_build_graph_resolves_local_replace(repo: Path) -> None:
    mods = am.discover_modules(repo)
    fwd, _p, warnings = am.build_graph(repo, mods)
    assert warnings == []
    assert fwd["auth"] == {"."}
    assert fwd["auth/oidc"] == {".", "auth"}
    assert "storage" not in fwd  # no in-repo deps


def test_build_graph_warns_on_unresolvable_in_repo_require(repo: Path) -> None:
    # Require a non-existent in-repo module (no matching local module path,
    # no replace pointing into the tree). Should warn.
    write(
        repo / "storage" / "go.mod",
        "module github.com/kbukum/gokit/storage\n\n"
        "go 1.26.0\n\n"
        "require github.com/kbukum/gokit/nonexistent v0.1.0\n",
    )
    git(repo, "add", "storage/go.mod")
    git(repo, "commit", "-q", "-m", "broken require")
    mods = am.discover_modules(repo)
    _f, _p, warnings = am.build_graph(repo, mods)
    assert warnings, "expected a warning for unresolvable in-repo require"
    assert "storage" in warnings[0]
    assert "nonexistent" in warnings[0]


def test_build_graph_resolves_require_even_without_replace(repo: Path) -> None:
    # If the required path matches a discovered local module's `module`
    # directive, that's a valid resolution even without an explicit
    # `replace` in this go.mod. (Production go.mod files normally have
    # both, but the graph for "what to retest" only cares about discoverable
    # in-repo edges.)
    write(
        repo / "storage" / "go.mod",
        "module github.com/kbukum/gokit/storage\n\n"
        "go 1.26.0\n\n"
        "require github.com/kbukum/gokit/auth v0.1.0\n",
    )
    git(repo, "add", "storage/go.mod")
    git(repo, "commit", "-q", "-m", "require-only")
    mods = am.discover_modules(repo)
    fwd, _p, warnings = am.build_graph(repo, mods)
    assert warnings == []
    assert fwd["storage"] == {"auth"}


# ──────────────────────────────────────────────────────────────────────────
# reverse_closure
# ──────────────────────────────────────────────────────────────────────────


def test_reverse_closure_propagates_to_dependents(repo: Path) -> None:
    mods = am.discover_modules(repo)
    fwd, _p, _w = am.build_graph(repo, mods)
    # Touching root must affect everything that requires it.
    assert am.reverse_closure({"."}, fwd) == {".", "auth", "auth/oidc"}
    # Touching auth must affect auth/oidc but not storage or root.
    assert am.reverse_closure({"auth"}, fwd) == {"auth", "auth/oidc"}
    # Touching storage stays local.
    assert am.reverse_closure({"storage"}, fwd) == {"storage"}


# ──────────────────────────────────────────────────────────────────────────
# owner_module
# ──────────────────────────────────────────────────────────────────────────


def test_owner_module_longest_prefix(repo: Path) -> None:
    mods = sorted(am.discover_modules(repo), key=len, reverse=True)
    assert am.owner_module("auth/oidc/oidc.go", mods) == "auth/oidc"
    assert am.owner_module("auth/auth.go", mods) == "auth"
    assert am.owner_module("storage/s.go", mods) == "storage"
    assert am.owner_module("README.md", mods) == "."


# ──────────────────────────────────────────────────────────────────────────
# compute (end-to-end)
# ──────────────────────────────────────────────────────────────────────────


def test_compute_push_event_returns_full(repo: Path) -> None:
    out = am.compute(repo, "push", base_sha(repo))
    assert out["is_full"] == "true"
    assert set(out["modules"]) == {".", "auth", "auth/oidc", "storage"}


def test_compute_change_in_oidc_only(repo: Path) -> None:
    base = base_sha(repo)
    commit_change(repo, {"auth/oidc/oidc.go": "package oidc // change\n"})
    out = am.compute(repo, "pull_request", base)
    assert out["is_full"] == "false"
    assert out["modules"] == ["auth/oidc"]


def test_compute_change_in_auth_propagates_to_oidc(repo: Path) -> None:
    base = base_sha(repo)
    commit_change(repo, {"auth/auth.go": "package auth // c\n"})
    out = am.compute(repo, "pull_request", base)
    assert set(out["modules"]) == {"auth", "auth/oidc"}


def test_compute_change_in_root_propagates_to_all_dependents(repo: Path) -> None:
    base = base_sha(repo)
    commit_change(repo, {"main.go": "package gokit // c\n"})
    out = am.compute(repo, "pull_request", base)
    assert set(out["modules"]) == {".", "auth", "auth/oidc"}
    assert "storage" not in out["modules"]


def test_compute_workflow_change_triggers_full(repo: Path) -> None:
    base = base_sha(repo)
    commit_change(repo, {".github/workflows/ci.yml": "name: CI # changed\n"})
    out = am.compute(repo, "pull_request", base)
    assert out["is_full"] == "true"


def test_compute_makefile_change_triggers_full(repo: Path) -> None:
    base = base_sha(repo)
    commit_change(repo, {"Makefile": "all:\n\techo changed\n"})
    out = am.compute(repo, "pull_request", base)
    assert out["is_full"] == "true"


def test_compute_added_module_triggers_full(repo: Path) -> None:
    base = base_sha(repo)
    commit_change(
        repo,
        {
            "newmod/go.mod": "module github.com/kbukum/gokit/newmod\n\ngo 1.26.0\n",
            "newmod/x.go": "package newmod\n",
        },
    )
    out = am.compute(repo, "pull_request", base)
    assert out["is_full"] == "true"


def test_compute_deleted_module_triggers_full(repo: Path) -> None:
    base = base_sha(repo)
    commit_change(
        repo,
        {"storage/go.mod": None, "storage/s.go": None},
    )
    out = am.compute(repo, "pull_request", base)
    assert out["is_full"] == "true"


def test_compute_docs_only_returns_noop(repo: Path) -> None:
    base = base_sha(repo)
    commit_change(repo, {"README.md": "# changed\n"})
    out = am.compute(repo, "pull_request", base)
    # README.md lives under root module (.) so it owns the change.
    # The repo has root go.mod, so it's not noop here. Test a path with
    # no module owner instead.


def test_compute_docs_only_outside_modules(tmp_path: Path) -> None:
    # Repo with NO root go.mod — only sub-modules.
    r = tmp_path / "repo"
    r.mkdir()
    git(r, "init", "-q", "-b", "main")
    write(r / "auth" / "go.mod", "module github.com/kbukum/gokit/auth\n\ngo 1.26.0\n")
    write(r / "auth" / "a.go", "package auth\n")
    write(r / "docs" / "x.md", "x\n")
    git(r, "add", "-A")
    git(r, "commit", "-q", "-m", "init")
    base = git(r, "rev-parse", "HEAD").strip()
    write(r / "docs" / "x.md", "y\n")
    git(r, "add", "-A")
    git(r, "commit", "-q", "-m", "docs")

    out = am.compute(r, "pull_request", base)
    assert out["modules"] == [am.NOOP]
    assert out["is_full"] == "false"


# ──────────────────────────────────────────────────────────────────────────
# build_check_includes
# ──────────────────────────────────────────────────────────────────────────


def test_check_includes_only_filters_extras_for_affected_modules() -> None:
    rows = am.build_check_includes(["auth", "auth/oidc"])
    # Default ubuntu rows for each affected module.
    assert {"module": "auth", "os": "ubuntu-latest"} in rows
    assert {"module": "auth/oidc", "os": "ubuntu-latest"} in rows
    # auth has a macos extra in CROSS_OS_EXTRAS.
    assert {"module": "auth", "os": "macos-latest"} in rows
    # storage NOT in affected: no storage/macos row.
    assert all(r["module"] != "storage" for r in rows)
    # Root NOT in affected: no root windows row.
    assert all(r["module"] != "." for r in rows)


def test_check_includes_includes_root_extras_when_root_affected() -> None:
    rows = am.build_check_includes(["."])
    assert {"module": ".", "os": "ubuntu-latest"} in rows
    assert {"module": ".", "os": "macos-latest"} in rows
    assert {"module": ".", "os": "windows-latest"} in rows
    assert {"module": ".", "os": "ubuntu-24.04-arm"} in rows


# ──────────────────────────────────────────────────────────────────────────
# emit / payload shapes
# ──────────────────────────────────────────────────────────────────────────


def test_noop_payload_shape() -> None:
    p = am.noop_payload("docs only")
    assert p["modules"] == [am.NOOP]
    assert p["check_includes"] == [
        {"module": am.NOOP, "os": "ubuntu-latest"}
    ]
    assert p["is_full"] == "false"


def test_full_payload_shape() -> None:
    p = am.full_payload(["auth", "storage"], "push")
    assert p["is_full"] == "true"
    assert p["modules"] == ["auth", "storage"]
    assert any(
        r["module"] == "storage" and r["os"] == "ubuntu-24.04-arm"
        for r in p["check_includes"]
    )


def test_emit_writes_github_output(tmp_path: Path) -> None:
    out_file = tmp_path / "out"
    am.emit(
        str(out_file),
        {"modules": ["auth"], "is_full": "false", "reason": "x"},
    )
    text = out_file.read_text()
    assert 'modules=["auth"]' in text
    assert "is_full=false" in text
    assert "reason=x" in text


# ──────────────────────────────────────────────────────────────────────────
# CLI smoke
# ──────────────────────────────────────────────────────────────────────────


def test_cli_runs_end_to_end(repo: Path, tmp_path: Path) -> None:
    out_file = tmp_path / "gh_out"
    base = base_sha(repo)
    commit_change(repo, {"auth/oidc/oidc.go": "package oidc // c\n"})
    res = subprocess.run(
        [
            sys.executable,
            str(SCRIPT_DIR / "affected_modules.py"),
            "--repo",
            str(repo),
            "--event",
            "pull_request",
            "--base",
            base,
            "--output",
            str(out_file),
        ],
        capture_output=True,
        text=True,
        check=True,
    )
    text = out_file.read_text()
    assert 'modules=["auth/oidc"]' in text
    # Stderr should contain the human-readable summary.
    assert "is_full:  false" in res.stderr
    # Parse the modules JSON via stdout for good measure.
    line = next(
        ln for ln in res.stdout.splitlines() if ln.startswith("modules=")
    )
    assert json.loads(line.split("=", 1)[1]) == ["auth/oidc"]
