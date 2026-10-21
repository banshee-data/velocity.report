"""Exercise the bibliography budget report against small Git checkouts."""

import importlib.util
import io
import json
import subprocess
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path

SCRIPT = Path(__file__).with_name("check-bibliography-budget.py")
spec = importlib.util.spec_from_file_location("check_bibliography_budget", SCRIPT)
budget = importlib.util.module_from_spec(spec)
spec.loader.exec_module(budget)

RULE = "% " + "=" * 20
BIB = "\n".join(
    [
        RULE,
        "% data/maths/references.bib",
        RULE,
        "",
        RULE,
        "% Tracking",
        RULE,
        "",
        "@article{Kalman1960,",
        "  title = {A new approach},",
        "  doi   = {10.1115/1.3662552}",
        "}",
        "@book{Thrun2005,",
        "  title = {Probabilistic robotics}",
        "}",
        "",
        RULE,
        "% Safety",
        RULE,
        "",
        "@article{Tefft2013,",
        "  doi = {10.1016/j.aap.2012.07.022}",
        "}",
        "",
    ]
)


class ParseTest(unittest.TestCase):
    def test_sections_keys_and_dois(self):
        self.assertEqual(
            budget.parse_bib(BIB),
            [
                ("Kalman1960", "Tracking", "10.1115/1.3662552"),
                ("Thrun2005", "Tracking", None),
                ("Tefft2013", "Safety", "10.1016/j.aap.2012.07.022"),
            ],
        )

    def test_report_counts_caps_and_citations(self):
        entries = budget.parse_bib(BIB)
        corpus = "See Kalman1960 and https://doi.org/10.1016/J.AAP.2012.07.022."
        counts, over, uncapped, stale, uncited = budget.report(
            entries, {"Tracking": 1, "Old": 3}, corpus
        )
        self.assertEqual(counts, {"Tracking": 2, "Safety": 1})
        self.assertEqual(over, [("Tracking", 2, 1)])
        self.assertEqual(uncapped, ["Safety"])
        self.assertEqual(stale, ["Old"])
        self.assertEqual(uncited, ["Thrun2005"])


class CheckoutTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        subprocess.run(["git", "-C", str(self.repo), "init", "-q"], check=True)
        self.write(budget.BIB, BIB)
        self.write("docs/plan.md", "Kalman1960 and Thrun2005 and Tefft2013.\n")
        subprocess.run(["git", "-C", str(self.repo), "add", "-A"], check=True)

    def write(self, name, text):
        path = self.repo / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def run_main(self, caps, *flags):
        self.write(budget.BUDGET, json.dumps({"caps": caps}))
        out = io.StringIO()
        with redirect_stdout(out):
            code = budget.main(["--repo", str(self.repo), *flags])
        return code, out.getvalue()

    def test_within_caps_passes_strict(self):
        code, out = self.run_main({"Tracking": 2, "Safety": 1}, "--strict")
        self.assertEqual(code, 0)
        self.assertIn("3 entries in 2 sections", out)
        self.assertNotIn("OVER", out)

    def test_over_cap_is_advisory_unless_strict(self):
        caps = {"Tracking": 1, "Safety": 1}
        self.assertEqual(self.run_main(caps)[0], 0)
        code, out = self.run_main(caps, "--strict")
        self.assertEqual(code, 1)
        self.assertIn("OVER", out)

    def test_uncited_entry_is_listed_but_never_fails(self):
        self.write("docs/plan.md", "Kalman1960 only.\n")
        code, out = self.run_main({"Tracking": 2, "Safety": 1}, "--strict")
        self.assertEqual(code, 0)
        self.assertIn("Thrun2005", out)
        self.assertIn("Tefft2013", out)


if __name__ == "__main__":
    unittest.main()
