"""Tests for scripts/fuzz.py: target discovery, balanced grouping and the run verdict."""

from __future__ import annotations

import importlib.util
import json
import pathlib
import sys

import pytest

HERE = pathlib.Path(__file__).resolve().parent

_spec = importlib.util.spec_from_file_location("fuzz", HERE / "fuzz.py")
assert _spec and _spec.loader
fuzz = importlib.util.module_from_spec(_spec)
sys.modules["fuzz"] = fuzz
_spec.loader.exec_module(fuzz)


def _write(root: pathlib.Path, rel: str, text: str) -> None:
    path = root / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


@pytest.fixture
def repo(tmp_path: pathlib.Path) -> pathlib.Path:
    _write(tmp_path, "go.mod", "module example.com/kit\n")
    _write(tmp_path, "codec/codec_test.go", "package codec\n\nfunc FuzzDecode(f *testing.F) {}\nfunc FuzzEncode(f *testing.F) {}\n")
    _write(tmp_path, "codec/helper.go", "package codec\n\nfunc FuzzNotATest() {}\n")
    _write(tmp_path, "root_test.go", "package kit\n\nfunc FuzzRoot(f *testing.F) {}\n")
    _write(tmp_path, "ai/go.mod", "module example.com/kit/ai\n")
    _write(tmp_path, "ai/llm/llm_test.go", "package llm\n\nfunc FuzzPrompt(f *testing.F) {}\n")
    _write(tmp_path, "ai/llm/testdata/fuzz/FuzzPrompt/seed", "go test fuzz v1\n")
    return tmp_path


def test_discovery_assigns_each_target_to_its_own_module(repo: pathlib.Path) -> None:
    targets = fuzz.discover(repo, [".", "ai"])

    assert [(t.module, t.package, t.name) for t in targets] == [
        (".", ".", "FuzzRoot"),
        (".", "./codec", "FuzzDecode"),
        (".", "./codec", "FuzzEncode"),
        ("ai", "./llm", "FuzzPrompt"),
    ]


def test_discovery_is_scoped_to_the_given_modules(repo: pathlib.Path) -> None:
    targets = fuzz.discover(repo, ["ai"])

    assert [t.name for t in targets] == ["FuzzPrompt"]


@pytest.mark.parametrize("count,size,groups", [(0, 8, 0), (1, 8, 1), (8, 8, 1), (9, 8, 2), (63, 8, 8)])
def test_group_count_targets_the_group_size(count: int, size: int, groups: int) -> None:
    assert fuzz.group_count(count, size) == groups


def test_groups_cover_every_target_exactly_once_and_stay_balanced() -> None:
    targets = [fuzz.Target(".", f"./p{i // 3}", f"Fuzz{i}") for i in range(63)]

    groups = fuzz.split(targets, 8)

    flat = [t for group in groups for t in group]
    assert sorted(flat) == sorted(targets)
    assert len(flat) == len(set(flat))
    sizes = [len(group) for group in groups]
    assert max(sizes) - min(sizes) <= 1


def test_split_rejects_more_groups_than_targets() -> None:
    with pytest.raises(ValueError):
        fuzz.split([fuzz.Target(".", ".", "FuzzA")], 2)


@pytest.mark.parametrize(
    "code,output,verdict",
    [
        (0, "ok", fuzz.Verdict.PASS),
        (1, "--- FAIL\n    Failing input written to testdata/fuzz/FuzzA/abc\n", fuzz.Verdict.CRASH),
        (1, "fuzz: elapsed: 30s\n--- FAIL: FuzzA (30.01s)\n    context deadline exceeded\nFAIL\nexit status 1\n"
            "FAIL\tgithub.com/kbukum/gokit/x\t30.2s\n", fuzz.Verdict.PASS),
        (1, "--- FAIL: FuzzA (0.01s)\n    a_test.go:12: context deadline exceeded\nFAIL\n"
            "FAIL\tgithub.com/kbukum/gokit/x\t0.2s\n", fuzz.Verdict.ERROR),
        (1, "--- FAIL: FuzzA (30.01s)\n    context deadline exceeded\n--- FAIL: TestB (0.00s)\nFAIL\n"
            "FAIL\tgithub.com/kbukum/gokit/x\t30.2s\n", fuzz.Verdict.ERROR),
        (2, "build failed", fuzz.Verdict.ERROR),
    ],
)
def test_verdict(code: int, output: str, verdict: object) -> None:
    assert fuzz.verdict(code, output) is verdict


def test_failing_input_paths_are_read_from_go_output() -> None:
    output = "    Failing input written to testdata/fuzz/FuzzA/abc\n    To re-run:\n"

    assert fuzz.failing_inputs(output) == ["testdata/fuzz/FuzzA/abc"]


def test_module_query_selects_all_modules_or_the_affected_ones() -> None:
    assert fuzz.module_query("toven", None) == ["toven", "--output", "jsonl", "modules"]
    assert fuzz.module_query("toven", "origin/main") == [
        "toven", "--output", "jsonl", "affected", "test", "--base", "origin/main", "--merge-base",
    ]


def test_go_module_roots_keep_only_go_records() -> None:
    output = "\n".join([
        "note: full activation",
        '{"module":"go:gokit","path":"."}',
        '{"module":"command:repo","path":"."}',
        '{"module":"go:auth","path":"auth"}',
    ])
    assert fuzz.go_module_roots(output) == [".", "auth"]


def test_plan_reports_the_modules_it_planned(repo: pathlib.Path, monkeypatch: pytest.MonkeyPatch,
                                             capsys: pytest.CaptureFixture[str]) -> None:
    monkeypatch.setattr(fuzz, "ROOT", repo)
    assert fuzz.main(["--modules-json", '[".", "ai"]', "plan", "--size", "1"]) == 0
    plan = json.loads(capsys.readouterr().out)
    assert plan["modules"] == [".", "ai"]
    assert plan["groups"] == plan["targets"] == len(plan["group"])


@pytest.mark.parametrize("text,seconds", [("30s", 30), ("1m30s", 90), ("500ms", 0.5), ("2h", 7200)])
def test_fuzz_seconds_reads_go_durations(text: str, seconds: float) -> None:
    assert fuzz.fuzz_seconds(text) == seconds


@pytest.mark.parametrize("text", ["0s", "0m0s", "", "30", "1.5s", "100x", "30s; rm -rf /", "-1s"])
def test_fuzz_seconds_refuses_zero_and_non_durations(text: str) -> None:
    with pytest.raises(ValueError):
        fuzz.fuzz_seconds(text)


def test_plan_carries_the_checked_fuzz_time(repo: pathlib.Path, monkeypatch: pytest.MonkeyPatch,
                                            capsys: pytest.CaptureFixture[str]) -> None:
    monkeypatch.setattr(fuzz, "ROOT", repo)
    assert fuzz.main(["--modules-json", '[".", "ai"]', "plan", "--fuzztime", "2m"]) == 0
    assert json.loads(capsys.readouterr().out)["fuzztime"] == "2m"


@pytest.mark.parametrize("fuzztime", ["0s", "abc", "2h"])
def test_plan_refuses_a_fuzz_time_a_shard_cannot_finish(repo: pathlib.Path, monkeypatch: pytest.MonkeyPatch,
                                                        fuzztime: str) -> None:
    monkeypatch.setattr(fuzz, "ROOT", repo)
    with pytest.raises(SystemExit) as exit_info:
        fuzz.main(["--modules-json", '[".", "ai"]', "plan", "--fuzztime", fuzztime])
    assert exit_info.value.code == 2


def test_run_refuses_a_zero_fuzz_time() -> None:
    with pytest.raises(SystemExit) as exit_info:
        fuzz.main(["--modules-json", "[]", "run", "--fuzztime", "0s"])
    assert exit_info.value.code == 2
