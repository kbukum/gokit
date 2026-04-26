"""Tests for scripts/govulncheck.py.

Covers suppression-file parsing, expiry handling, reachable-vs-imported
classification, and end-to-end behavior with a mocked govulncheck.
"""

from __future__ import annotations

import datetime as dt
import json
import pathlib
import sys

import pytest

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import govulncheck as gv  # type: ignore  # noqa: E402


# ---------- helpers ----------------------------------------------------------


def write_suppressions(tmp_path: pathlib.Path, entries: list[dict]) -> pathlib.Path:
    p = tmp_path / "sup.json"
    p.write_text(json.dumps({"suppressions": entries}))
    return p


def make_finding(osv: str, *, called: bool, module: str = "github.com/x/y") -> dict:
    trace = [{"module": module}]
    if called:
        trace[0]["function"] = "Foo"
    return {"finding": {"osv": osv, "trace": trace}}


def stream(messages: list[dict]) -> str:
    # govulncheck emits pretty-printed JSON objects in a stream, not NDJSON.
    return "\n".join(json.dumps(m, indent=2) for m in messages) + "\n"


# ---------- load_suppressions ------------------------------------------------


def test_load_suppressions_missing_file_returns_empty(tmp_path):
    assert gv.load_suppressions(tmp_path / "nope.json") == []


def test_load_suppressions_invalid_json(tmp_path):
    p = tmp_path / "bad.json"
    p.write_text("{ not json")
    with pytest.raises(SystemExit, match="not valid JSON"):
        gv.load_suppressions(p)


@pytest.mark.parametrize("missing", ["id", "modules", "reason", "expires"])
def test_load_suppressions_required_keys(tmp_path, missing):
    base = {"id": "GO-1", "modules": ["m"], "reason": "r", "expires": "2099-01-01"}
    base.pop(missing)
    p = write_suppressions(tmp_path, [base])
    with pytest.raises(SystemExit, match=f"missing required key '{missing}'"):
        gv.load_suppressions(p)


def test_load_suppressions_invalid_expiry(tmp_path):
    p = write_suppressions(tmp_path, [{"id": "GO-1", "modules": ["m"], "reason": "r", "expires": "tomorrow"}])
    with pytest.raises(SystemExit, match="invalid expires"):
        gv.load_suppressions(p)


def test_load_suppressions_empty_modules(tmp_path):
    p = write_suppressions(tmp_path, [{"id": "GO-1", "modules": [], "reason": "r", "expires": "2099-01-01"}])
    with pytest.raises(SystemExit, match="must be non-empty list"):
        gv.load_suppressions(p)


def test_load_suppressions_invalid_accept_reachable_type(tmp_path):
    p = write_suppressions(
        tmp_path,
        [{"id": "GO-1", "modules": ["m"], "reason": "r", "expires": "2099-01-01", "accept_reachable": "yes"}],
    )
    with pytest.raises(SystemExit, match="must be bool"):
        gv.load_suppressions(p)


def test_load_suppressions_happy_path(tmp_path):
    p = write_suppressions(
        tmp_path,
        [{"id": "GO-1", "modules": ["m1", "m2"], "reason": "r", "expires": "2099-12-31"}],
    )
    out = gv.load_suppressions(p)
    assert len(out) == 1
    assert out[0]["expires_date"] == dt.date(2099, 12, 31)
    assert out[0]["accept_reachable"] is False  # default


# ---------- applicable_suppression -------------------------------------------


def _sup(id_: str, *, modules=("workload",)) -> dict:
    return {"id": id_, "modules": list(modules), "reason": "", "expires": "x", "expires_date": dt.date(2099, 1, 1), "accept_reachable": False}


def test_applicable_suppression_matches():
    s = _sup("GO-1")
    assert gv.applicable_suppression([s], "workload", "GO-1") is s


def test_applicable_suppression_module_mismatch():
    assert gv.applicable_suppression([_sup("GO-1")], "tool", "GO-1") is None


def test_applicable_suppression_id_mismatch():
    assert gv.applicable_suppression([_sup("GO-1")], "workload", "GO-2") is None


# ---------- collect_findings & is_called -------------------------------------


def test_collect_findings_groups_by_osv():
    msgs = [make_finding("GO-1", called=False), make_finding("GO-1", called=True), make_finding("GO-2", called=False)]
    grouped = gv.collect_findings(msgs)
    assert set(grouped.keys()) == {"GO-1", "GO-2"}
    assert len(grouped["GO-1"]) == 2


def test_collect_findings_ignores_non_finding_messages():
    msgs = [{"config": {"protocol_version": "v1.0.0"}}, {"progress": {"message": "Scanning"}}, make_finding("GO-1", called=True)]
    assert set(gv.collect_findings(msgs).keys()) == {"GO-1"}


def test_is_called_true_when_any_trace_has_function():
    findings = [make_finding("GO-1", called=False)["finding"], make_finding("GO-1", called=True)["finding"]]
    assert gv.is_called(findings) is True


def test_is_called_false_when_no_trace_has_function():
    findings = [make_finding("GO-1", called=False)["finding"]]
    assert gv.is_called(findings) is False


# ---------- parse_ndjson -----------------------------------------------------


def test_parse_ndjson_skips_blanks():
    raw = '{"a":1}\n\n  \n{"b":2}\n'
    assert list(gv.parse_ndjson(raw)) == [{"a": 1}, {"b": 2}]


def test_parse_ndjson_handles_pretty_printed_stream():
    raw = '{\n  "a": 1\n}\n{\n  "b": 2\n}\n'
    assert list(gv.parse_ndjson(raw)) == [{"a": 1}, {"b": 2}]


# ---------- end-to-end main() ------------------------------------------------


@pytest.fixture
def fake_repo(tmp_path, monkeypatch):
    """Create a minimal repo layout the script can locate as root."""
    (tmp_path / ".github").mkdir()
    (tmp_path / "go.mod").write_text("module example.com/x\n\ngo 1.22\n")
    monkeypatch.chdir(tmp_path)
    return tmp_path


def patch_govulncheck(monkeypatch, *, rc: int, stdout: str, stderr: str = ""):
    def fake_run(extra_args):
        return rc, stdout, stderr
    monkeypatch.setattr(gv, "run_govulncheck", fake_run)


def _write_sup_at(repo: pathlib.Path, entries: list[dict]) -> None:
    (repo / ".github" / "govulncheck-suppressions.json").write_text(
        json.dumps({"suppressions": entries})
    )


def test_main_no_findings_returns_0(fake_repo, monkeypatch, capsys):
    patch_govulncheck(monkeypatch, rc=0, stdout="")
    rc = gv.main(["--module", "workload", "--", "./..."])
    assert rc == 0
    assert "unsuppressed:     0" in capsys.readouterr().out


def test_main_unsuppressed_finding_returns_1(fake_repo, monkeypatch, capsys):
    patch_govulncheck(monkeypatch, rc=3, stdout=stream([make_finding("GO-9999", called=True)]))
    rc = gv.main(["--module", "workload"])
    assert rc == 1
    out = capsys.readouterr().out
    assert "GO-9999" in out
    assert "REACHABLE" in out


def test_main_suppressed_unreachable_returns_0(fake_repo, monkeypatch, capsys):
    _write_sup_at(
        fake_repo,
        [{"id": "GO-1", "modules": ["workload"], "reason": "not reachable", "expires": "2099-01-01"}],
    )
    patch_govulncheck(monkeypatch, rc=3, stdout=stream([make_finding("GO-1", called=False)]))
    rc = gv.main(["--module", "workload"])
    assert rc == 0
    out = capsys.readouterr().out
    assert "Suppressed" in out
    assert "GO-1" in out


def test_main_suppressed_reachable_without_accept_reachable_fails(fake_repo, monkeypatch, capsys):
    _write_sup_at(
        fake_repo,
        [{"id": "GO-1", "modules": ["workload"], "reason": "x", "expires": "2099-01-01"}],
    )
    patch_govulncheck(monkeypatch, rc=3, stdout=stream([make_finding("GO-1", called=True)]))
    rc = gv.main(["--module", "workload"])
    assert rc == 1
    err = capsys.readouterr().err
    assert "REACHABLE" in err


def test_main_accept_reachable_with_references_suppresses(fake_repo, monkeypatch, capsys):
    _write_sup_at(
        fake_repo,
        [{
            "id": "GO-1", "modules": ["workload"], "reason": "x", "expires": "2099-01-01",
            "accept_reachable": True, "references": ["https://example.com/ticket"],
        }],
    )
    patch_govulncheck(monkeypatch, rc=3, stdout=stream([make_finding("GO-1", called=True)]))
    rc = gv.main(["--module", "workload"])
    assert rc == 0
    err = capsys.readouterr().err
    assert "accepting REACHABLE advisory GO-1" in err


def test_main_accept_reachable_without_references_fails(fake_repo, monkeypatch, capsys):
    _write_sup_at(
        fake_repo,
        [{
            "id": "GO-1", "modules": ["workload"], "reason": "x", "expires": "2099-01-01",
            "accept_reachable": True,
        }],
    )
    patch_govulncheck(monkeypatch, rc=3, stdout=stream([make_finding("GO-1", called=True)]))
    rc = gv.main(["--module", "workload"])
    assert rc == 1
    err = capsys.readouterr().err
    assert "no references" in err


def test_main_expired_suppression_fails_even_with_no_findings(fake_repo, monkeypatch, capsys):
    _write_sup_at(
        fake_repo,
        [{"id": "GO-1", "modules": ["workload"], "reason": "x", "expires": "2020-01-01"}],
    )
    patch_govulncheck(monkeypatch, rc=0, stdout="")
    rc = gv.main(["--module", "workload", "--today", "2025-01-01"])
    assert rc == 1
    err = capsys.readouterr().err
    assert "expired" in err


def test_main_suppression_for_other_module_does_not_apply(fake_repo, monkeypatch, capsys):
    _write_sup_at(
        fake_repo,
        [{"id": "GO-1", "modules": ["tool"], "reason": "x", "expires": "2099-01-01"}],
    )
    patch_govulncheck(monkeypatch, rc=3, stdout=stream([make_finding("GO-1", called=False)]))
    rc = gv.main(["--module", "workload"])
    assert rc == 1


def test_main_govulncheck_invocation_failure_returns_2(fake_repo, monkeypatch):
    patch_govulncheck(monkeypatch, rc=1, stdout="", stderr="boom")
    rc = gv.main(["--module", "workload"])
    assert rc == 2
