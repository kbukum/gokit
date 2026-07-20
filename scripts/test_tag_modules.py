#!/usr/bin/env python3

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parents[1] / "tag-modules.sh"


class TagModulesTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tempdir.cleanup)
        root = Path(self.tempdir.name)
        self.repo = root / "repo"
        self.origin = root / "origin.git"

        self.run_git("init", "--bare", str(self.origin), cwd=root)
        self.run_git("init", "-b", "main", str(self.repo), cwd=root)
        self.run_git("config", "user.name", "Release Test", cwd=self.repo)
        self.run_git("config", "user.email", "release-test@example.com", cwd=self.repo)

        (self.repo / "sub").mkdir()
        (self.repo / "go.mod").write_text(
            "module github.com/kbukum/gokit\n\ngo 1.26.0\n",
            encoding="utf-8",
        )
        (self.repo / "sub" / "go.mod").write_text(
            "module github.com/kbukum/gokit/sub\n\ngo 1.26.0\n",
            encoding="utf-8",
        )
        self.write_changelog()

        self.run_git("add", ".", cwd=self.repo)
        self.run_git("commit", "-m", "initial", cwd=self.repo)
        self.run_git("remote", "add", "origin", str(self.origin), cwd=self.repo)
        self.run_git("push", "-u", "origin", "main", cwd=self.repo)

    def write_changelog(self, version: str = "0.3.0-alpha.1") -> None:
        (self.repo / "CHANGELOG.md").write_text(
            "# Changelog\n\n"
            "## [Unreleased]\n\n"
            "_No unreleased changes._\n\n"
            f"## [{version}] - 2026-07-19\n\n"
            "### Changed\n\n"
            "- Prepared the release.\n",
            encoding="utf-8",
        )

    def run_script(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [str(SCRIPT), *args],
            cwd=self.repo,
            check=False,
            capture_output=True,
            text=True,
        )

    @staticmethod
    def run_git(*args: str, cwd: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", *args],
            cwd=cwd,
            check=True,
            capture_output=True,
            text=True,
        )

    def test_dry_run_is_safe_from_feature_branch_with_dirty_tree(self) -> None:
        self.run_git("switch", "-c", "feature", cwd=self.repo)
        (self.repo / "scratch").write_text("dirty\n", encoding="utf-8")

        result = self.run_script("v0.3.0-alpha.1", "--dry-run")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            result.stdout.splitlines()[-2:],
            ["v0.3.0-alpha.1", "sub/v0.3.0-alpha.1"],
        )

    def test_rejects_invalid_semver(self) -> None:
        for version in (
            "0.3.0",
            "v0.3",
            "v0.3.0-alpha..1",
            "v0.3.0-alpha.01",
            "v0.3.0+build",
        ):
            with self.subTest(version=version):
                result = self.run_script(version, "--dry-run")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("invalid semantic version", result.stderr)

    def test_requires_exact_populated_changelog_section(self) -> None:
        result = self.run_script("v0.3.0-alpha.2", "--dry-run")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("CHANGELOG.md is missing [0.3.0-alpha.2]", result.stderr)

    def test_rejects_unreleased_content(self) -> None:
        changelog = (self.repo / "CHANGELOG.md").read_text(encoding="utf-8")
        (self.repo / "CHANGELOG.md").write_text(
            changelog.replace(
                "_No unreleased changes._",
                "### Fixed\n\n- Not yet included in the release.",
            ),
            encoding="utf-8",
        )

        result = self.run_script("v0.3.0-alpha.1", "--dry-run")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("[Unreleased] must be empty", result.stderr)

    def test_rejects_module_path_that_does_not_match_directory(self) -> None:
        (self.repo / "sub" / "go.mod").write_text(
            "module github.com/kbukum/gokit/wrong\n\ngo 1.26.0\n",
            encoding="utf-8",
        )

        result = self.run_script("v0.3.0-alpha.1", "--dry-run")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("module path mismatch", result.stderr)

    def test_rejects_force_option(self) -> None:
        result = self.run_script("v0.3.0-alpha.1", "--force")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unknown argument: --force", result.stderr)

    def test_pushes_complete_signed_tag_family(self) -> None:
        if shutil.which("ssh-keygen") is None:
            self.skipTest("ssh-keygen is required for signed-tag integration test")

        key = Path(self.tempdir.name) / "release-signing-key"
        subprocess.run(
            ["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(key)],
            check=True,
        )
        self.run_git("config", "gpg.format", "ssh", cwd=self.repo)
        self.run_git("config", "user.signingkey", str(key), cwd=self.repo)

        result = self.run_script("v0.3.0-alpha.1", "--push")

        self.assertEqual(result.returncode, 0, result.stderr)
        remote_refs = self.run_git(
            "ls-remote",
            "--tags",
            "origin",
            cwd=self.repo,
        ).stdout
        self.assertIn("refs/tags/v0.3.0-alpha.1\n", remote_refs)
        self.assertIn("refs/tags/sub/v0.3.0-alpha.1\n", remote_refs)
        self.assertEqual(
            self.run_git(
                "cat-file",
                "-t",
                "v0.3.0-alpha.1",
                cwd=self.repo,
            ).stdout.strip(),
            "tag",
        )


if __name__ == "__main__":
    unittest.main()
