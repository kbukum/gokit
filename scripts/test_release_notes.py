#!/usr/bin/env python3

import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent / "release-notes.sh"


class ReleaseNotesTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tempdir.cleanup)
        self.repo = Path(self.tempdir.name)
        (self.repo / "scripts").mkdir()
        self.script = self.repo / "scripts" / "release-notes.sh"
        self.script.symlink_to(SCRIPT)
        (self.repo / "CHANGELOG.md").write_text(
            "# Changelog\n\n"
            "## [Unreleased]\n\n"
            "### Fixed\n\n"
            "- Future fix.\n\n"
            "## [0.3.0-alpha.1] - 2026-07-19\n\n"
            "### Added\n\n"
            "- First alpha.\n",
            encoding="utf-8",
        )
        subprocess.run(
            ["git", "init", "-b", "main"],
            cwd=self.repo,
            check=True,
            capture_output=True,
            text=True,
        )

    def run_script(self, version: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [str(self.script), version],
            cwd=self.repo,
            check=False,
            capture_output=True,
            text=True,
        )

    def test_extracts_exact_prerelease_section(self) -> None:
        result = self.run_script("v0.3.0-alpha.1")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("## v0.3.0-alpha.1", result.stdout)
        self.assertIn("- First alpha.", result.stdout)
        self.assertNotIn("- Future fix.", result.stdout)

    def test_rejects_missing_prerelease_section(self) -> None:
        result = self.run_script("v0.3.0-alpha.2")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn(
            "CHANGELOG.md has no '## [0.3.0-alpha.2]' section",
            result.stderr,
        )

    def test_rejects_invalid_version(self) -> None:
        result = self.run_script("not-a-version")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("invalid semantic version", result.stderr)


if __name__ == "__main__":
    unittest.main()
