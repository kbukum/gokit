"""Tests for scripts/govulncheck.py.

Covers suppression-file parsing, expiry handling, reachable-vs-imported
classification, and end-to-end behavior with a mocked govulncheck.
"""

from __future__ import annotations

import datetime as dt
import json
import pathlib
import re
import subprocess
import sys
import tomllib

import pytest
import yaml

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


def make_config() -> dict:
    return {"config": {"protocol_version": "v1.0.0", "scan_level": "symbol", "scan_mode": "source"}}


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
    (tmp_path / "workload").mkdir()
    (tmp_path / "workload" / "go.mod").write_text("module example.com/x/workload\n\ngo 1.22\n")
    monkeypatch.chdir(tmp_path)
    return tmp_path


def patch_govulncheck(monkeypatch, *, rc: int, stdout: str, stderr: str = ""):
    def fake_run(extra_args):
        return rc, stream([make_config()]) + stdout, stderr
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


@pytest.mark.parametrize("location", [".", "workload"])
@pytest.mark.parametrize("module", ["workload", "./workload"])
def test_main_scans_selected_module(fake_repo, monkeypatch, location, module):
    monkeypatch.chdir(fake_repo / location)
    calls = []

    def fake_run(extra_args):
        calls.append(extra_args)
        return 0, stream([make_config()]), ""

    monkeypatch.setattr(gv, "run_govulncheck", fake_run)
    assert gv.main(["--module", module]) == 0
    assert calls == [["-C", str(fake_repo / "workload"), "./..."]]


def test_main_absolute_module_uses_canonical_suppression_name(fake_repo, monkeypatch):
    _write_sup_at(fake_repo, [{"id": "GO-1", "modules": ["workload"], "reason": "accepted", "expires": "2099-01-01"}])
    patch_govulncheck(monkeypatch, rc=0, stdout=stream([make_finding("GO-1", called=False)]))
    assert gv.main(["--module", str(fake_repo / "workload")]) == 0


@pytest.mark.parametrize("module", ["../outside", "missing", ".github"])
def test_main_rejects_invalid_module(fake_repo, monkeypatch, module):
    def unexpected_run(extra_args):
        pytest.fail("invalid module must not run the scanner")

    monkeypatch.setattr(gv, "run_govulncheck", unexpected_run)
    assert gv.main(["--module", module]) == 1


def test_main_imported_advisory_fails_even_when_scanner_returns_success(fake_repo, monkeypatch):
    patch_govulncheck(monkeypatch, rc=0, stdout=stream([make_finding("GO-9999", called=False)]))
    assert gv.main(["--module", "workload"]) == 1


def test_local_and_ci_security_gates_share_policy():
    root = HERE.parent
    config = tomllib.loads((root / "toven.toml").read_text())
    task = config["ecosystems"]["go"]["tasks"]["vuln"]
    assert task["argv"] == [
        "python3", "{workspace.root}/scripts/govulncheck.py", "--module",
        "{module.root}", "--", "{args}", "{module.selector}",
    ]
    assert task["cacheable"] is False
    workflow = yaml.safe_load((root / ".github/workflows/ci.yml").read_text())
    security = workflow["jobs"]["security"]
    for job in ["security", "licenses", "fuzz", "integration"]:
        assert workflow["jobs"][job]["if"] == "needs.changes.outputs.go-code == 'true'"
    assert any("scripts/govulncheck.py" in step.get("run", "") for step in security["steps"])
    assert "security" in workflow["jobs"]["ci-status"]["needs"]
    integration = workflow["jobs"]["integration"]
    assert any("run test --" in step.get("run", "") and "-tags=integration" in step["run"] for step in integration["steps"])


@pytest.mark.parametrize("rc,output", [
    (3, "Vulnerability detected"),
    (0, '{"finding":'),
    (0, ""),
    (0, '[]'),
    (3, '{"config":{"protocol_version":"v1.0.0"}}'),
])
def test_main_rejects_invalid_scanner_output(fake_repo, monkeypatch, rc, output):
    monkeypatch.setattr(gv, "run_govulncheck", lambda extra: (rc, output, ""))
    assert gv.main(["--module", "workload"]) == 2


@pytest.mark.parametrize("rc", [0, 3])
@pytest.mark.parametrize("finding", [
    None, {}, [], "invalid",
    {"trace": [{"module": "example.com/x"}]},
    {"osv": ""},
    {"osv": ["GO-1"]},
    {"osv": "GO-1"},
    {"osv": "GO-1", "trace": []},
    {"osv": "GO-1", "trace": "invalid"},
    {"osv": "GO-1", "trace": [None]},
    {"osv": "GO-1", "trace": [{}]},
    {"osv": "GO-1", "trace": [{"module": 42}]},
    {"osv": "GO-1", "trace": [{"module": "example.com/x", "function": False}]},
])
def test_main_rejects_malformed_findings(fake_repo, monkeypatch, capsys, rc, finding):
    patch_govulncheck(monkeypatch, rc=rc, stdout=stream([{"finding": finding}]))
    assert gv.main(["--module", "workload"]) == 2
    output = capsys.readouterr()
    assert "invalid govulncheck output" in output.err
    assert "total advisories" not in output.out


@pytest.mark.parametrize("flag,value", [
    ("format", "text"), ("C", ".."), ("scan", "module"), ("scan", "package"),
    ("mode", "binary"), ("mode", "extract"),
])
@pytest.mark.parametrize("prefix", ["-", "--"])
@pytest.mark.parametrize("joined", [False, True])
def test_main_rejects_scanner_policy_overrides(fake_repo, monkeypatch, flag, value, prefix, joined):
    def unexpected_run(extra_args):
        pytest.fail("overrides must not invoke the scanner")

    monkeypatch.setattr(gv, "run_govulncheck", unexpected_run)
    args = [f"{prefix}{flag}={value}"] if joined else [f"{prefix}{flag}", value]
    assert gv.main(["--module", "workload", "--", *args, "./..."]) == 1


def test_run_govulncheck_owns_scan_policy(monkeypatch):
    calls = []

    def fake_run(command, **kwargs):
        calls.append(command)
        assert kwargs["env"]["GOWORK"] == "off"
        return subprocess.CompletedProcess(command, 0, "output", "")

    monkeypatch.setattr(gv.subprocess, "run", fake_run)
    assert gv.run_govulncheck(["-C", "/repo/module", "./..."]) == (0, "output", "")
    assert calls == [[
        "govulncheck", "-format", "json", "-mode", "source", "-scan", "symbol",
        "-C", "/repo/module", "./...",
    ]]


@pytest.mark.parametrize("rc", [0, 3])
@pytest.mark.parametrize("level,mode", [
    ("module", "source"), ("package", "source"), ("symbol", "binary"),
    ("symbol", "extract"), (None, "source"), ("symbol", None),
])
def test_main_rejects_non_symbol_source_results(fake_repo, monkeypatch, capsys, rc, level, mode):
    _write_sup_at(fake_repo, [
        {"id": "GO-1", "modules": ["workload"], "reason": "not reachable", "expires": "2099-01-01"},
    ])
    config = make_config()
    config["config"]["scan_level"] = level
    config["config"]["scan_mode"] = mode
    output = stream([config, make_finding("GO-1", called=False)])
    monkeypatch.setattr(gv, "run_govulncheck", lambda extra: (rc, output, ""))
    assert gv.main(["--module", "workload"]) == 2
    captured = capsys.readouterr()
    assert "invalid govulncheck output" in captured.err
    assert "total advisories" not in captured.out


def test_main_rejects_multiple_scanner_configurations(fake_repo, monkeypatch):
    output = stream([make_config(), make_config()])
    monkeypatch.setattr(gv, "run_govulncheck", lambda extra: (0, output, ""))
    assert gv.main(["--module", "workload"]) == 2


@pytest.mark.parametrize("job", ["preflight", "check", "check-cross", "lint", "security", "licenses", "fuzz", "integration"])
def test_aggregate_rejects_skipped_required_jobs(job):
    workflow = yaml.safe_load((HERE.parent / ".github/workflows/ci.yml").read_text())
    script = workflow["jobs"]["ci-status"]["steps"][0]["run"]
    script = script.replace("${{ needs.changes.outputs.go-code }}", "true")
    script = script.replace("${{ needs." + job + ".result }}", "skipped")
    script = re.sub(r"\$\{\{ needs\.[\w-]+\.result \}\}", "success", script)
    result = subprocess.run(["bash", "-c", script], capture_output=True, text=True)
    assert result.returncode == 1
    assert "CI failed" in result.stdout
