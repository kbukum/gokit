"""Tests for scripts/integration_modules.py."""

from __future__ import annotations

import importlib.util
import pathlib
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location(
    "integration_modules", pathlib.Path(__file__).with_name("integration_modules.py"))
integration_modules = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(integration_modules)


class OwningModulesTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temp.name)
        self.modules = {".": "go:root", "a": "go:a", "a/nested": "go:a-nested", "b": "go:b"}

    def tearDown(self):
        self.temp.cleanup()

    def write(self, rel: str, text: str) -> None:
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def test_selects_the_nearest_module_of_each_tagged_file(self):
        self.write("x_integration_test.go", "//go:build integration\n\npackage x\n")
        self.write("a/nested/pkg/y_test.go", "//go:build integration && !race\n\npackage pkg\n")
        self.write("a/plain_test.go", "package a\n")
        self.write("b/z.go", "//go:build !integration\n\npackage b\n")
        self.assertEqual(
            integration_modules.owning_modules(self.root, self.modules),
            ["go:a-nested", "go:root"])

    def test_ignores_testdata_and_tags_that_merely_contain_the_word(self):
        self.write("a/testdata/t_test.go", "//go:build integration\n")
        self.write("b/w_test.go", "//go:build integrationx\n\npackage b\n")
        self.write("b/later_test.go", "package b\n\n//go:build integration\n")
        self.assertEqual(integration_modules.owning_modules(self.root, self.modules), [])

    def test_skips_modules_toven_does_not_manage(self):
        self.write("examples/go.mod", "module example.com/examples\n")
        self.write("examples/demo/d_test.go", "//go:build integration\n\npackage demo\n")
        self.assertEqual(integration_modules.owning_modules(self.root, self.modules), [])


if __name__ == "__main__":
    unittest.main()
