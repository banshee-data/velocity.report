#!/usr/bin/env python3
"""Tests for LaTeX failure diagnostics in the PDF generator."""

import os
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import MagicMock, patch

from pdf_generator.core.pdf_generator import (
    _read_latex_log_excerpt,
    _suggest_latex_fixes,
    _explain_latex_failure,
    generate_pdf_report,
)


class TestLatexLogHelpers(unittest.TestCase):
    """Unit tests for LaTeX log parsing and hint generation helpers."""

    def test_read_latex_log_excerpt_collects_key_lines(self):
        """Ensure the log reader captures error, location, and guidance lines."""

        with TemporaryDirectory() as tmp_dir:
            base_path = Path(tmp_dir) / "faulty_build"
            log_path = base_path.with_suffix(".log")
            log_path.write_text(
                "Random info line\n"
                "! LaTeX Error: File `fontspec.sty' not found.\n"
                "l.17 \\usepackage{fontspec}\n"
                "See the LaTeX manual or LaTeX Companion for explanation.\n"
                "Trailing noise\n",
                encoding="utf-8",
            )

            excerpt = _read_latex_log_excerpt(base_path)

            self.assertEqual(
                excerpt,
                [
                    "! LaTeX Error: File `fontspec.sty' not found.",
                    "l.17 \\usepackage{fontspec}",
                    "See the LaTeX manual or LaTeX Companion for explanation.",
                ],
            )

    def test_suggest_latex_fixes_provides_actionable_hints(self):
        """Verify helper produces engine and font guidance without duplicates."""

        excerpt = ["! LaTeX Error: File `fontspec.sty' not found."]
        hints = _suggest_latex_fixes("xelatex", "xelatex not found", excerpt)

        # Engine installation guidance
        self.assertTrue(any("tex" in hint.lower() for hint in hints))
        # fontspec hint should be present
        self.assertTrue(any("fontspec" in hint.lower() for hint in hints))
        # ensure hints remain deduplicated
        self.assertEqual(len(hints), len(set(hints)))

    def test_explain_latex_failure_compiles_full_message(self):
        """Confirm explanatory string includes excerpt, hints, and paths."""

        with TemporaryDirectory() as tmp_dir:
            base_path = Path(tmp_dir) / "faulty_build"
            log_path = base_path.with_suffix(".log")
            log_path.write_text(
                "! LaTeX Error: File `fontspec.sty' not found.\n",
                encoding="utf-8",
            )

            message = _explain_latex_failure(
                "xelatex", base_path, RuntimeError("xelatex not found")
            )

            self.assertIn("LaTeX compilation with xelatex failed.", message)
            self.assertIn("fontspec", message)
            self.assertIn(str(log_path), message)
            self.assertIn(str(base_path.with_suffix(".tex")), message)
            self.assertIn("Underlying error: xelatex not found", message)


class TestLatexFailureIntegration(unittest.TestCase):
    """Integration-style tests for LaTeX failure reporting in generate_pdf_report."""

    def test_generate_pdf_report_surfaces_latex_diagnostics(self):
        """Simulate repeated LaTeX failures and ensure diagnostics bubble up."""

        with TemporaryDirectory() as tmp_dir, patch(
            "pdf_generator.core.pdf_generator.DocumentBuilder"
        ) as mock_builder, patch(
            "pdf_generator.core.pdf_generator.chart_exists", return_value=False
        ), patch(
            "pdf_generator.core.pdf_generator.MapProcessor"
        ):
            mock_doc = MagicMock()
            builder_inst = mock_builder.return_value
            builder_inst.build.return_value = mock_doc

            mock_doc.generate_pdf.side_effect = [
                RuntimeError("xelatex not found on PATH"),
                RuntimeError("lualatex missing binaries"),
                RuntimeError("pdflatex missing package fontspec"),
            ]
            mock_doc.generate_tex.return_value = None

            base_path = Path(tmp_dir) / "faulty_report"
            log_path = base_path.with_suffix(".log")
            log_path.write_text(
                "! LaTeX Error: File `fontspec.sty' not found.\n"
                "l.17 \\usepackage{fontspec}\n"
                "See the LaTeX manual or LaTeX Companion for explanation.\n",
                encoding="utf-8",
            )

            output_pdf = str(base_path.with_suffix(".pdf"))

            with self.assertRaises(RuntimeError) as ctx:
                generate_pdf_report(
                    output_path=output_pdf,
                    start_iso="2025-06-01T00:00:00-07:00",
                    end_iso="2025-06-02T00:00:00-07:00",
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Diagnostic Test Site",
                    overall_metrics=[
                        {
                            "count": 120,
                            "p50": 20.0,
                            "p85": 28.0,
                            "p98": 35.0,
                            "max_speed": 42.0,
                        }
                    ],
                    daily_metrics=None,
                    granular_metrics=[],
                    histogram=None,
                    tz_name="UTC",
                    charts_prefix=os.path.join(tmp_dir, "missing_charts"),
                    include_map=False,
                )

            message = str(ctx.exception)

            self.assertIn("LaTeX compilation with pdflatex failed.", message)
            self.assertIn("fontspec", message)
            self.assertIn("Suggested fixes:", message)
            self.assertIn(str(log_path), message)
            self.assertIn(
                "Underlying error: pdflatex missing package fontspec", message
            )

            self.assertEqual(mock_doc.generate_pdf.call_count, 3)
            self.assertTrue(mock_doc.generate_tex.called)


if __name__ == "__main__":
    unittest.main()
