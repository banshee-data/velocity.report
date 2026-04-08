#!/usr/bin/env python3

"""Tests for LaTeX fatal-signature detection helpers."""

import tempfile
import unittest
from pathlib import Path

from pdf_generator.core.pdf_generator import _detect_fatal_latex_signature


class TestDetectFatalLatexSignature(unittest.TestCase):
    def test_returns_none_when_log_missing(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            base_path = Path(tmpdir) / "missing_report"
            self.assertIsNone(_detect_fatal_latex_signature(base_path))

    def test_detects_lm_metric_error(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            base_path = Path(tmpdir) / "report"
            log_path = base_path.with_suffix(".log")
            log_path.write_text(
                "Some line\n"
                "! Font \\TU/lmr/m/n/12=[lmroman12-regular]:mapping=tex-text; at 12pt not loadable: Metric (TFM) file or installed font not found\n"
            )

            failure = _detect_fatal_latex_signature(base_path)
            self.assertIsNotNone(failure)
            self.assertIn("metric", failure.lower())

    def test_detects_nullfont_signature(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            base_path = Path(tmpdir) / "report"
            log_path = base_path.with_suffix(".log")
            log_path.write_text("! Font nullfont fallback triggered\n")

            failure = _detect_fatal_latex_signature(base_path)
            self.assertIsNotNone(failure)
            self.assertIn("nullfont", failure.lower())

    def test_detects_prefixed_explicit_fatal_line(self):
        """TeX logs may prefix fatal lines with '!' and extra whitespace."""
        with tempfile.TemporaryDirectory() as tmpdir:
            base_path = Path(tmpdir) / "report"
            log_path = base_path.with_suffix(".log")
            log_path.write_text(
                "Some preamble\n"
                "!  ==> Fatal error occurred, no output PDF file produced!\n"
            )

            failure = _detect_fatal_latex_signature(base_path)
            self.assertIsNotNone(failure)
            self.assertIn("fatal error", failure.lower())


if __name__ == "__main__":
    unittest.main()
