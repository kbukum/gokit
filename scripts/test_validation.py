"""Contracts for the affected-validation entry point and real Toven selection."""

from __future__ import annotations

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent.parent


class ValidationTests(unittest.TestCase):
    def test_make_delegates_dirty_input_and_dependency_selection_to_toven(self):
        with tempfile.TemporaryDirectory(prefix="gokit-validation-") as directory:
            root = Path(directory)
            tool = root / "toven"
            tool.write_text('#!/bin/sh\nprintf "%s\\n" "$@"\n')
            tool.chmod(0o700)
            result = subprocess.run(
                ["make", "--no-print-directory", "-f", str(ROOT / "Makefile"),
                 "test-affected", f"TOVEN={tool}", "BASE=HEAD"],
                cwd=root, capture_output=True, text=True, timeout=15, check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.splitlines(), [
                "test", "--base", "HEAD", "--no-cache", "--",
                "-race", "-shuffle=on", "-count=1",
            ])

    def test_toven_selects_dirty_untracked_deleted_and_reverse_dependents(self):
        toven = os.environ.get("TOVEN") or shutil.which("toven")
        self.assertIsNotNone(toven, "Toven is required; missing tooling is not a skip")
        with tempfile.TemporaryDirectory(prefix="gokit-impact-") as directory:
            root = Path(directory)
            env = {**os.environ, "GIT_CONFIG_NOSYSTEM": "1",
                   "GIT_CONFIG_GLOBAL": os.devnull, "GIT_TERMINAL_PROMPT": "0"}

            def run(*args):
                result = subprocess.run(
                    args, cwd=root, env=env, capture_output=True, text=True,
                    timeout=30, check=False,
                )
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                return result.stdout

            (root / "toven.toml").write_text(
                '[project]\nname="probe"\nroot="."\nbase_ref="HEAD"\n'
                '[ecosystems.go]\nmodules="auto"\n'
                '[ecosystems.go.tasks.test]\n'
                'argv=["go","-C","{module.root}","test","{args}","{module.selector}"]\n'
                'fan_out="per-module"\nselector=["./..."]\n'
            )
            for name in ("leaf", "consumer", "unrelated"):
                module = root / name
                module.mkdir()
                manifest = f"module example.com/{name}\n\ngo 1.22.0\n"
                if name == "consumer":
                    manifest += "\nrequire example.com/leaf v0.0.0\nreplace example.com/leaf => ../leaf\n"
                (module / "go.mod").write_text(manifest)
                (module / "value.go").write_text(f"package {name}\n")
            run("git", "init", "-q")
            run("git", "add", ".")
            run("git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
            source = root / "leaf/value.go"
            for state in ("unstaged", "staged", "untracked", "deleted"):
                with self.subTest(state=state):
                    source.write_text("package leaf\nconst Changed = true\n")
                    if state == "staged":
                        run("git", "add", "leaf/value.go")
                    elif state == "untracked":
                        # Restore just this owned fixture's baseline before adding a new input.
                        run("git", "restore", "--staged", "--worktree", "leaf/value.go")
                        (root / "leaf/new_test.go").write_text("package leaf\n")
                    elif state == "deleted":
                        (root / "leaf/new_test.go").unlink()
                        source.unlink()
                    output = run(toven, "affected", "test", "--base", "HEAD")
                    self.assertIn("go:leaf", output)
                    self.assertIn("go:consumer", output)
                    self.assertNotIn("go:unrelated", output)


if __name__ == "__main__":
    unittest.main()
