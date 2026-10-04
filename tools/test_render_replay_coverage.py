"""Coverage rendering must match the Go gate across multiple test binaries."""

import tempfile
import unittest
from pathlib import Path

from render_replay_coverage import PRODUCTION_PACKAGES, coverage_profile


class CoverageProfileTests(unittest.TestCase):
    def profile(self, extra):
        rows = [f"{package}/source.go:1.1,2.1 2 1" for package in PRODUCTION_PACKAGES]
        return "mode: set\n" + "\n".join(rows + extra) + "\n"

    def measure(self, text):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "source.out"
            filtered = Path(directory) / "filtered.out"
            source.write_text(text, encoding="utf-8")
            totals, generated = coverage_profile(source, filtered)
            return totals, generated, filtered.read_text(encoding="utf-8")

    def test_cross_binary_union_counts_each_block_once(self):
        package = PRODUCTION_PACKAGES[0]
        totals, generated, filtered = self.measure(self.profile([
            f"{package}/source.go:1.1,2.1 2 0",
            f"{package}/source.go:3.1,4.1 3 0",
            f"{package}/source.go:3.1,4.1 3 1",
            f"{package}/source.go:3.1,4.1 3 0",
            f"{package}/models.gen.go:1.1,2.1 7 0",
            f"{package}/models.gen.go:1.1,2.1 7 1",
        ]))
        self.assertEqual(totals[package], (5, 5))
        self.assertEqual(generated, {f"{package}/models.gen.go"})
        self.assertEqual(filtered.count("source.go:3.1,4.1"), 1)
        self.assertNotIn("models.gen.go", filtered)

    def test_conflicting_duplicate_statement_counts_fail(self):
        package = PRODUCTION_PACKAGES[0]
        with self.assertRaisesRegex(RuntimeError, "conflicting duplicate"):
            self.measure(self.profile([f"{package}/source.go:1.1,2.1 3 1"]))

    def test_negative_counts_fail(self):
        package = PRODUCTION_PACKAGES[0]
        with self.assertRaisesRegex(RuntimeError, "negative Go coverage"):
            self.measure(self.profile([f"{package}/source.go:3.1,4.1 3 -1"]))


if __name__ == "__main__":
    unittest.main()
