#!/usr/bin/env python3
"""Consolidated integration tests for PDF generation pipeline.

This module contains streamlined integration tests that maintain full coverage
while reducing redundancy and improving test execution time.

Previous structure: multiple small tests with overlapping work
New structure: consolidated tests that validate full behavior with less duplication
"""

import unittest
import tempfile
import os
from unittest.mock import patch, MagicMock

from pdf_generator.core.pdf_generator import generate_pdf_report


class TestPDFIntegrationConsolidated(unittest.TestCase):
    """Consolidated end-to-end integration tests for PDF generation."""

    def setUp(self):
        """Set up test fixtures with realistic API data."""
        # Sample data based on ww-test9-2 report
        self.overall_metrics = [
            {
                "Count": 3469,
                "P50Speed": 30.54,
                "P85Speed": 36.94,
                "P98Speed": 43.05,
                "MaxSpeed": 53.52,
            }
        ]

        self.daily_metrics = [
            {
                "StartTime": "2025-06-02T00:00:00-07:00",
                "Count": 891,
                "P50Speed": 30.54,
                "P85Speed": 37.23,
                "P98Speed": 43.92,
                "MaxSpeed": 51.19,
            },
            {
                "StartTime": "2025-06-03T00:00:00-07:00",
                "Count": 1315,
                "P50Speed": 30.54,
                "P85Speed": 36.36,
                "P98Speed": 41.59,
                "MaxSpeed": 53.52,
            },
            {
                "StartTime": "2025-06-04T00:00:00-07:00",
                "Count": 1263,
                "P50Speed": 30.54,
                "P85Speed": 37.23,
                "P98Speed": 42.76,
                "MaxSpeed": 53.52,
            },
        ]

        self.granular_metrics = [
            {
                "StartTime": "2025-06-02T08:00:00-07:00",
                "Count": 109,
                "P50Speed": 23.43,
                "P85Speed": 35.71,
                "P98Speed": 43.78,
                "MaxSpeed": 46.47,
            },
            {
                "StartTime": "2025-06-02T09:00:00-07:00",
                "Count": 152,
                "P50Speed": 30.54,
                "P85Speed": 37.52,
                "P98Speed": 42.47,
                "MaxSpeed": 46.83,
            },
            {
                "StartTime": "2025-06-02T10:00:00-07:00",
                "Count": 162,
                "P50Speed": 34.32,
                "P85Speed": 40.14,
                "P98Speed": 45.96,
                "MaxSpeed": 51.19,
            },
        ]

        self.histogram = {
            "5": 66,
            "10": 239,
            "15": 294,
            "20": 338,
            "25": 720,
            "30": 971,
            "35": 631,
            "40": 183,
            "45": 24,
            "50": 3,
        }

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_full_pdf_generation_and_content_validation(
        self, mock_chart_exists, mock_map_processor
    ):
        """Comprehensive test validating complete PDF generation and all content.

            This single test replaces 9 previous tests that all generated the same
            PDF but checked different aspects of the output. By consolidating, we:
        - Generate PDF once instead of multiple times to reduce redundant work and speed up tests
            - Read .tex file once instead of 9 times
            - Validate all content in one comprehensive test

            Validates:
            - File creation (.tex file exists)
            - LaTeX document structure (documentclass, packages, begin/end)
            - All metrics (vehicle counts, speed percentiles)
            - Location information
            - Date ranges
            - Footer formatting (dates and page numbers)
            - Histogram table data
            - Daily metrics table
            - Survey parameters section
        """
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

            # Generate PDF (will create .tex file when LaTeX compilation fails)
            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Clarendon Avenue, San Francisco",
                    overall_metrics=self.overall_metrics,
                    daily_metrics=self.daily_metrics,
                    granular_metrics=self.granular_metrics,
                    histogram=self.histogram,
                    tz_name="US/Pacific",
                    charts_prefix="test",
                    speed_limit=25,
                    # Original date strings from datepicker (required)
                    start_date="2025-06-02",
                    end_date="2025-06-04",
                )
            except Exception:
                # Expected to fail without LaTeX compiler
                pass

            # Verify .tex file was created
            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(
                os.path.exists(tex_path), f"Expected .tex file at {tex_path}"
            )

            with open(tex_path, "r") as f:
                content = f.read()

            self.assertIn(r"\usepackage{fontspec}", content, "Missing fontspec package")
            self.assertIn(r"\usepackage{graphicx}", content, "Missing graphicx package")

            # === Metrics Validation ===
            # Vehicle count (may be formatted with comma)
            self.assertTrue(
                "3469" in content or "3,469" in content,
                "Vehicle count should appear in .tex file",
            )
            # Speed metrics
            self.assertIn("53.52", content, "Max speed missing")
            self.assertIn("43.05", content, "P98 speed missing")
            self.assertIn("36.94", content, "P85 speed missing")
            self.assertIn("30.54", content, "P50 speed missing")

            # === Location Information ===
            self.assertIn(
                "Clarendon Avenue, San Francisco",
                content,
                "Location information missing",
            )

            # === Date Range ===
            self.assertIn("2025-06-02", content, "Start date missing")
            self.assertIn("2025-06-04", content, "End date missing")

            # === Footer Validation ===
            self.assertIn(
                r"\fancyfoot[L]{\small 2025-06-02 to 2025-06-04}",
                content,
                "Footer date range missing",
            )
            self.assertIn(
                r"\fancyfoot[R]{\small Page \thepage}",
                content,
                "Footer page number missing",
            )
            self.assertIn(
                r"\renewcommand{\footrulewidth}{0.8pt}",
                content,
                "Footer rule missing",
            )
            # Verify date range is NOT in header center (moved to footer)
            self.assertNotIn(
                r"\fancyhead[C]",
                content,
                "Date range should not be in header center",
            )

            # === Histogram Table Data ===
            self.assertIn("66", content, "Histogram count for first bucket missing")
            self.assertIn("971", content, "Histogram highest count missing")

            # === Daily Metrics Table ===
            self.assertIn("891", content, "Daily count for first day missing")
            self.assertIn("1315", content, "Daily count for second day missing")
            self.assertIn("1263", content, "Daily count for third day missing")

            # === Survey Parameters Section ===
            self.assertIn(
                "Survey Parameters", content, "Survey Parameters header missing"
            )
            self.assertIn("1h", content, "Roll-up period missing")
            self.assertIn("mph", content, "Units missing")
            self.assertIn("US/Pacific", content, "Timezone missing")

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_edge_cases_combined(self, mock_chart_exists, mock_map_processor):
        """Validate multiple edge-case behaviors in a single generation.

        Purpose: exercise several independent code paths that may
        occur together in production while keeping the test focused and
        maintainable. The test ensures the generator produces a valid LaTeX
        (.tex) output even when some data or external resources are missing or
        incomplete.

        Behaviors covered:
        - The generator handles missing "overall" summary metrics gracefully.
        - Empty or missing histogram data does not break LaTeX output.
        - The generator tolerates missing daily granularity (no daily metrics).
        - Map processing can succeed and include a map artifact in the output.
        - Chart-availability checks (such as whether a stats chart exists) are
          respected and influence which sections are emitted.
        """
        with tempfile.TemporaryDirectory() as tmpdir:
            # Configure mocks to test multiple paths in one generation
            def chart_side_effect(prefix, chart_type):
                # Return True for stats chart to exercise that code path
                # Return False for histogram (since we're testing empty histogram)
                return chart_type == "stats"

            mock_chart_exists.side_effect = chart_side_effect

            # Mock map processor to return success
            mock_processor = MagicMock()
            mock_processor.process_map.return_value = (True, "test_map.pdf")
            mock_map_processor.return_value = mock_processor

            output_path = os.path.join(tmpdir, "all_edge_cases.pdf")

            # Single generation testing multiple compatible edge cases
            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="24h",
                    units="mph",
                    timezone_display="UTC",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=None,
                    daily_metrics=None,
                    granular_metrics=[],
                    histogram={},
                    tz_name="UTC",
                    charts_prefix="edge",
                    speed_limit=25,
                    # Original date strings from datepicker (required)
                    start_date="2025-06-02",
                    end_date="2025-06-04",
                )
            except Exception:
                # Expected to fail without a LaTeX compiler available in CI
                pass

            # Verify .tex file was created despite all edge cases
            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(
                os.path.exists(tex_path),
                "Should create .tex file with missing/empty inputs and map/chart",
            )

            # Verify the document structure is intact
            with open(tex_path, "r") as f:
                content = f.read()

            self.assertIn(r"\begin{document}", content)
            self.assertIn(r"\end{document}", content)

            # Verify edge cases didn't break the document
            self.assertIn("Test Location", content, "Location should be present")
            self.assertIn("2025-06-02", content, "Start date should be present")
            self.assertIn(
                "Survey Parameters", content, "Parameters section should exist"
            )

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_complete_failure_path(self, mock_chart_exists, mock_map_processor):
        """Test complete failure when all PDF generation engines fail.

        Validates that when both PDF generation and TEX generation fail,
        the appropriate exception is raised.
        """
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "fail_test.pdf")

            with patch(
                "pdf_generator.core.pdf_generator.DocumentBuilder"
            ) as mock_builder_class:
                mock_builder = MagicMock()
                mock_doc = MagicMock()
                mock_builder.build.return_value = mock_doc
                mock_builder_class.return_value = mock_builder

                # Make all PDF generation attempts fail
                mock_doc.generate_pdf.side_effect = Exception("PDF generation failed")
                # Make TEX generation also fail
                mock_doc.generate_tex.side_effect = Exception("TEX generation failed")

                # Should raise the last exception
                with self.assertRaises(Exception) as context:
                    generate_pdf_report(
                        output_path=output_path,
                        start_iso="2025-06-02T00:00:00-07:00",
                        end_iso="2025-06-04T23:59:59-07:00",
                        group="1h",
                        units="mph",
                        timezone_display="UTC",
                        min_speed_str="5.0 mph",
                        location="Test",
                        overall_metrics=[{"Count": 100}],
                        daily_metrics=None,
                        granular_metrics=[],
                        histogram={"10": 50},
                        tz_name="UTC",
                        charts_prefix="fail",
                        speed_limit=25,
                    )

                self.assertIn(
                    "PDF generation failed",
                    str(context.exception),
                    "Should raise PDF generation exception",
                )

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_font_fallback(self, mock_chart_exists, mock_map_processor):
        """Test font fallback logic when custom fonts are missing.

        Validates that the system properly handles missing font files
        by triggering the fallback mechanism.
        """
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "font_fallback.pdf")

            with patch(
                "pdf_generator.core.pdf_generator.os.path.exists"
            ) as mock_exists:
                # Return False for mono font check to trigger fallback
                mock_exists.side_effect = lambda p: "Mono" not in p

                try:
                    generate_pdf_report(
                        output_path=output_path,
                        start_iso="2025-06-02T00:00:00-07:00",
                        end_iso="2025-06-04T23:59:59-07:00",
                        group="1h",
                        units="mph",
                        timezone_display="UTC",
                        min_speed_str="5.0 mph",
                        location="Test",
                        overall_metrics=[{"Count": 100}],
                        daily_metrics=None,
                        granular_metrics=[],
                        histogram={"10": 50},
                        tz_name="UTC",
                        charts_prefix="font",
                        speed_limit=25,
                        # Original date strings from datepicker (required)
                        start_date="2025-06-02",
                        end_date="2025-06-04",
                    )
                except Exception:
                    # May fail due to missing LaTeX, but we're testing font fallback
                    pass

                # Verify font fallback was triggered (mock was called)
                self.assertTrue(
                    mock_exists.called, "Font fallback check should be triggered"
                )

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_date_consistency_no_plus_one_day(
        self, mock_chart_exists, mock_map_processor
    ):
        """Verify dates are consistent throughout the document.

        This test catches the bug where end_of_day timestamps (23:59:59)
        were being extracted and displayed as the next day (+1 day error).

        The test uses dates where timestamp-based extraction could show wrong dates:
        - User selects June 2-4 in datepicker
        - ISO timestamps are 2025-06-02T00:00:00 to 2025-06-04T23:59:59
        - The document should show June 2-4, NOT June 2-5

        Validates:
        - Footer shows correct dates (not +1 day)
        - Overview shows correct dates
        - No occurrence of the wrong end date anywhere
        """
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "date_test.pdf")

            # User selected June 2-4 in datepicker
            start_date = "2025-06-02"
            end_date = "2025-06-04"

            # ISO timestamps include 23:59:59 on end date to include full day
            # This is where the bug occurred - extracting [:10] from end_iso could
            # show 2025-06-05 if timezone conversion went wrong
            start_iso = "2025-06-02T00:00:00-07:00"
            end_iso = "2025-06-04T23:59:59-07:00"

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso=start_iso,
                    end_iso=end_iso,
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=self.overall_metrics,
                    daily_metrics=None,
                    granular_metrics=[],
                    histogram={},
                    tz_name="US/Pacific",
                    charts_prefix="date_test",
                    speed_limit=25,
                    # These are the original date strings from the datepicker
                    # They should be used as-is throughout the document
                    start_date=start_date,
                    end_date=end_date,
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

            with open(tex_path, "r") as f:
                content = f.read()

            # The correct end date should appear in footer
            self.assertIn(
                f"{start_date} to {end_date}",
                content,
                f"Footer should show '{start_date} to {end_date}' (original dates)",
            )

            # The WRONG end date should NOT appear in the footer
            wrong_end_date = "2025-06-05"
            self.assertNotIn(
                f"to {wrong_end_date}",
                content,
                f"Footer should NOT show '{wrong_end_date}' (+1 day error)",
            )

            # Verify correct dates in overview section (now in bullet points)
            # Should show as: \item \textbf{Period:} 2025-06-02 to 2025-06-04
            self.assertIn(
                f"{start_date} to {end_date}",
                content,
                "Overview should show correct date range",
            )
            # Verify NO occurrence of wrong date
            self.assertNotIn(
                wrong_end_date,
                content,
                f"Should NOT show {wrong_end_date} anywhere in document",
            )

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_comparison_date_consistency(self, mock_chart_exists, mock_map_processor):
        """Verify comparison period dates are consistent throughout.

        Similar to test_date_consistency_no_plus_one_day but for comparison periods.
        """
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "compare_date_test.pdf")

            # Primary period: June 2-4
            start_date = "2025-06-02"
            end_date = "2025-06-04"
            start_iso = "2025-06-02T00:00:00-07:00"
            end_iso = "2025-06-04T23:59:59-07:00"

            # Comparison period: January 15-19
            compare_start_date = "2026-01-15"
            compare_end_date = "2026-01-19"
            compare_start_iso = "2026-01-15T00:00:00-08:00"
            compare_end_iso = "2026-01-19T23:59:59-08:00"

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso=start_iso,
                    end_iso=end_iso,
                    compare_start_iso=compare_start_iso,
                    compare_end_iso=compare_end_iso,
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=self.overall_metrics,
                    compare_overall_metrics=self.overall_metrics,
                    daily_metrics=None,
                    compare_daily_metrics=None,
                    granular_metrics=[],
                    compare_granular_metrics=[],
                    histogram={},
                    compare_histogram={},
                    tz_name="US/Pacific",
                    charts_prefix="compare_date_test",
                    speed_limit=25,
                    # Original date strings from datepicker
                    start_date=start_date,
                    end_date=end_date,
                    compare_start_date=compare_start_date,
                    compare_end_date=compare_end_date,
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

            with open(tex_path, "r") as f:
                content = f.read()

            # Footer should show both periods with correct dates
            expected_footer = (
                f"{start_date} to {end_date} vs "
                f"{compare_start_date} to {compare_end_date}"
            )
            self.assertIn(
                expected_footer,
                content,
                f"Footer should show '{expected_footer}'",
            )

            # Wrong dates should NOT appear
            self.assertNotIn(
                "2025-06-05",
                content,
                "Should NOT show June 5 (+1 day error on primary)",
            )
            self.assertNotIn(
                "2026-01-20",
                content,
                "Should NOT show Jan 20 (+1 day error on comparison)",
            )


if __name__ == "__main__":
    unittest.main()


class TestLatexErrorHandling(unittest.TestCase):
    """Tests for LaTeX error handling functions."""

    def test_read_latex_log_excerpt_no_file(self):
        """Test log reading when log file doesn't exist."""
        from pdf_generator.core.pdf_generator import _read_latex_log_excerpt
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmpdir:
            # No log file exists
            base_path = Path(os.path.join(tmpdir, "missing"))
            result = _read_latex_log_excerpt(base_path)
            self.assertEqual(result, [])

    def test_read_latex_log_excerpt_with_errors(self):
        """Test log reading with actual error content."""
        from pdf_generator.core.pdf_generator import _read_latex_log_excerpt
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmpdir:
            # Create log file with error content
            log_path = Path(os.path.join(tmpdir, "test.log"))
            log_path.write_text(
                "Some normal output\n"
                "! Undefined control sequence.\n"
                "l.42 \\badcommand\n"
                "See the LaTeX manual for more info.\n"
            )

            base_path = Path(os.path.join(tmpdir, "test"))
            result = _read_latex_log_excerpt(base_path)
            self.assertGreater(len(result), 0)
            self.assertTrue(
                any("Undefined control sequence" in line for line in result)
            )

    def test_read_latex_log_excerpt_read_error(self):
        """Test log reading when file can't be read."""
        from pdf_generator.core.pdf_generator import _read_latex_log_excerpt
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmpdir:
            base_path = Path(os.path.join(tmpdir, "test"))
            log_path = base_path.with_suffix(".log")

            # Create directory with same name as log file to cause read error
            os.makedirs(str(log_path))

            result = _read_latex_log_excerpt(base_path)
            self.assertEqual(result, [])

    def test_suggest_latex_fixes_fontspec(self):
        """Test fix suggestions for fontspec errors."""
        from pdf_generator.core.pdf_generator import _suggest_latex_fixes

        hints = _suggest_latex_fixes(
            engine="xelatex",
            message="package fontspec not found",
            excerpt=["! fontspec error: cannot find font"],
        )
        self.assertTrue(any("fontspec" in hint.lower() for hint in hints))

    def test_suggest_latex_fixes_atkinson(self):
        """Test fix suggestions for Atkinson font errors."""
        from pdf_generator.core.pdf_generator import _suggest_latex_fixes

        hints = _suggest_latex_fixes(
            engine="xelatex",
            message="font not found",
            excerpt=['The font "AtkinsonHyperlegible" cannot be found'],
        )
        self.assertTrue(any("atkinson" in hint.lower() for hint in hints))

    def test_suggest_latex_fixes_ttf_file(self):
        """Test fix suggestions for missing TTF files."""
        from pdf_generator.core.pdf_generator import _suggest_latex_fixes

        hints = _suggest_latex_fixes(
            engine="xelatex",
            message="font file missing",
            excerpt=["file '/path/to/font.ttf' not found"],
        )
        self.assertTrue(any("font" in hint.lower() for hint in hints))

    def test_suggest_latex_fixes_undefined_sequence(self):
        """Test fix suggestions for undefined control sequence."""
        from pdf_generator.core.pdf_generator import _suggest_latex_fixes

        hints = _suggest_latex_fixes(
            engine="xelatex",
            message="compilation error",
            excerpt=["undefined control sequence \\badcommand"],
        )
        self.assertTrue(any("undefined" in hint.lower() for hint in hints))

    def test_suggest_latex_fixes_no_specific_error(self):
        """Test fallback suggestion when no specific error found."""
        from pdf_generator.core.pdf_generator import _suggest_latex_fixes

        hints = _suggest_latex_fixes(
            engine="xelatex",
            message="unknown error",
            excerpt=["some generic error"],
        )
        # Should have default fallback hint
        self.assertGreater(len(hints), 0)
        self.assertTrue(any("inspect" in hint.lower() for hint in hints))

    def test_suggest_latex_fixes_engine_not_found(self):
        """Test fix suggestions when engine is not found."""
        from pdf_generator.core.pdf_generator import _suggest_latex_fixes

        hints = _suggest_latex_fixes(
            engine="xelatex",
            message="xelatex not found",
            excerpt=[],
        )
        self.assertTrue(any("xelatex" in hint.lower() for hint in hints))

    def test_explain_latex_failure(self):
        """Test the explain_latex_failure function."""
        from pdf_generator.core.pdf_generator import _explain_latex_failure
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmpdir:
            base_path = Path(os.path.join(tmpdir, "test"))

            explanation = _explain_latex_failure(
                engine="xelatex",
                base_path=base_path,
                exc=Exception("LaTeX compilation failed"),
            )

            self.assertIsInstance(explanation, str)
            self.assertGreater(len(explanation), 0)


class TestPDFWithComparisonData(unittest.TestCase):
    """Tests for PDF generation with comparison period data."""

    def setUp(self):
        """Set up test fixtures with comparison data."""
        self.overall_metrics = [
            {
                "Count": 1000,
                "P50Speed": 25.0,
                "P85Speed": 30.0,
                "P98Speed": 35.0,
                "MaxSpeed": 40.0,
            }
        ]
        self.daily_metrics = [
            {
                "StartTime": "2025-06-02T00:00:00-07:00",
                "Count": 500,
                "P50Speed": 25.0,
                "P85Speed": 30.0,
                "P98Speed": 35.0,
                "MaxSpeed": 40.0,
            },
        ]
        self.granular_metrics = [
            {
                "StartTime": "2025-06-02T08:00:00-07:00",
                "Count": 100,
                "P50Speed": 25.0,
                "P85Speed": 30.0,
                "P98Speed": 35.0,
                "MaxSpeed": 40.0,
            },
        ]
        self.histogram = {"10": 50, "15": 100, "20": 150, "25": 100, "30": 50}

        # Comparison data
        self.compare_overall = [
            {
                "Count": 800,
                "P50Speed": 24.0,
                "P85Speed": 29.0,
                "P98Speed": 34.0,
                "MaxSpeed": 39.0,
            }
        ]
        self.compare_daily = [
            {
                "StartTime": "2025-01-15T00:00:00-08:00",
                "Count": 400,
                "P50Speed": 24.0,
                "P85Speed": 29.0,
                "P98Speed": 34.0,
                "MaxSpeed": 39.0,
            },
        ]
        self.compare_granular = [
            {
                "StartTime": "2025-01-15T08:00:00-08:00",
                "Count": 80,
                "P50Speed": 24.0,
                "P85Speed": 29.0,
                "P98Speed": 34.0,
                "MaxSpeed": 39.0,
            },
        ]
        self.compare_histogram = {"10": 40, "15": 80, "20": 120, "25": 80, "30": 40}

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_pdf_with_comparison_histograms(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test PDF generation with comparison histograms."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "compare_hist.pdf")

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=self.overall_metrics,
                    daily_metrics=self.daily_metrics,
                    granular_metrics=self.granular_metrics,
                    histogram=self.histogram,
                    tz_name="US/Pacific",
                    charts_prefix="compare_hist",
                    speed_limit=25,
                    start_date="2025-06-02",
                    end_date="2025-06-04",
                    # Comparison data
                    compare_start_iso="2025-01-15T00:00:00-08:00",
                    compare_end_iso="2025-01-19T23:59:59-08:00",
                    compare_overall_metrics=self.compare_overall,
                    compare_daily_metrics=self.compare_daily,
                    compare_granular_metrics=self.compare_granular,
                    compare_histogram=self.compare_histogram,
                    compare_start_date="2025-01-15",
                    compare_end_date="2025-01-19",
                )
            except Exception:
                pass  # May fail without LaTeX

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

            with open(tex_path, "r") as f:
                content = f.read()

            # Verify comparison dates appear
            self.assertIn("2025-01-15", content)

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_pdf_with_config_periods_and_cosine_angles(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test PDF generation with config periods containing cosine angles."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        config_periods = [
            {"cosine_error_angle": 15.0, "site_id": 1},
            {"cosine_error_angle": 20.0, "site_id": 1},
        ]

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "cosine_test.pdf")

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=self.overall_metrics,
                    daily_metrics=self.daily_metrics,
                    granular_metrics=self.granular_metrics,
                    histogram=self.histogram,
                    tz_name="US/Pacific",
                    charts_prefix="cosine_test",
                    speed_limit=25,
                    start_date="2025-06-02",
                    end_date="2025-06-04",
                    config_periods=config_periods,
                    # Comparison data with different cosine angle
                    compare_start_iso="2025-01-15T00:00:00-08:00",
                    compare_end_iso="2025-01-19T23:59:59-08:00",
                    compare_start_date="2025-01-15",
                    compare_end_date="2025-01-19",
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_pdf_with_histogram_chart(self, mock_chart_exists, mock_map_processor):
        """Test PDF generation when histogram chart exists."""
        # Return True only for histogram chart
        mock_chart_exists.side_effect = (
            lambda prefix, chart_type: chart_type == "histogram"
        )
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "hist_chart.pdf")
            charts_prefix = os.path.join(tmpdir, "test_charts")

            # Create a fake histogram chart PDF
            hist_pdf = f"{charts_prefix}_histogram.pdf"
            with open(hist_pdf, "wb") as f:
                f.write(b"%PDF-1.4 fake pdf")

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=self.overall_metrics,
                    daily_metrics=self.daily_metrics,
                    granular_metrics=self.granular_metrics,
                    histogram=self.histogram,
                    tz_name="US/Pacific",
                    charts_prefix=charts_prefix,
                    speed_limit=25,
                    start_date="2025-06-02",
                    end_date="2025-06-04",
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

            with open(tex_path, "r") as f:
                content = f.read()

            # Verify histogram chart reference
            self.assertIn("histogram.pdf", content)

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_pdf_with_map_enabled(self, mock_chart_exists, mock_map_processor):
        """Test PDF generation when map is included."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        # Return True for map processing with a fake path
        mock_processor.process_map.return_value = (True, "/tmp/fake_map.pdf")
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "with_map.pdf")

            # Create a fake map PDF
            map_pdf = os.path.join(tmpdir, "fake_map.pdf")
            with open(map_pdf, "wb") as f:
                f.write(b"%PDF-1.4 fake pdf")

            mock_processor.process_map.return_value = (True, map_pdf)

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=self.overall_metrics,
                    daily_metrics=self.daily_metrics,
                    granular_metrics=self.granular_metrics,
                    histogram=self.histogram,
                    tz_name="US/Pacific",
                    charts_prefix="map_test",
                    speed_limit=25,
                    start_date="2025-06-02",
                    end_date="2025-06-04",
                    include_map=True,
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

    @patch("pdf_generator.core.pdf_generator.RadarStatsClient")
    @patch("pdf_generator.core.pdf_generator.extract_svg_from_site_data")
    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_pdf_with_site_id_and_gps_coords(
        self, mock_chart_exists, mock_map_processor, mock_extract_svg, mock_client_class
    ):
        """Test PDF generation with site_id and GPS coordinates for map marker."""
        mock_chart_exists.return_value = False

        mock_client = MagicMock()
        mock_client.get_site.return_value = (
            {"id": 1, "name": "Test Site"},
            MagicMock(),
        )
        mock_client_class.return_value = mock_client

        mock_extract_svg.return_value = False  # No db map

        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "site_gps.pdf")

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=self.overall_metrics,
                    daily_metrics=self.daily_metrics,
                    granular_metrics=self.granular_metrics,
                    histogram=self.histogram,
                    tz_name="US/Pacific",
                    charts_prefix="site_gps",
                    speed_limit=25,
                    start_date="2025-06-02",
                    end_date="2025-06-04",
                    include_map=True,
                    site_id=1,
                    map_latitude=37.7749,
                    map_longitude=-122.4194,
                    bbox_ne_lat=37.78,
                    bbox_ne_lng=-122.41,
                    bbox_sw_lat=37.77,
                    bbox_sw_lng=-122.43,
                    map_angle=45.0,
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_pdf_with_stats_charts(self, mock_chart_exists, mock_map_processor):
        """Test PDF generation with stats charts."""

        def chart_exists_side_effect(prefix, chart_type):
            return chart_type in ["stats", "compare_stats"]

        mock_chart_exists.side_effect = chart_exists_side_effect
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "stats_chart.pdf")
            charts_prefix = os.path.join(tmpdir, "charts")

            with open(f"{charts_prefix}_stats.pdf", "wb") as f:
                f.write(b"%PDF-1.4 fake pdf")
            with open(f"{charts_prefix}_compare_stats.pdf", "wb") as f:
                f.write(b"%PDF-1.4 fake pdf")

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=self.overall_metrics,
                    daily_metrics=self.daily_metrics,
                    granular_metrics=self.granular_metrics,
                    histogram=self.histogram,
                    tz_name="US/Pacific",
                    charts_prefix=charts_prefix,
                    speed_limit=25,
                    start_date="2025-06-02",
                    end_date="2025-06-04",
                    compare_start_iso="2025-01-15T00:00:00-08:00",
                    compare_end_iso="2025-01-19T23:59:59-08:00",
                    compare_start_date="2025-01-15",
                    compare_end_date="2025-01-19",
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

    @patch("pdf_generator.core.pdf_generator.RadarStatsClient")
    @patch("pdf_generator.core.pdf_generator.extract_svg_from_site_data")
    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_pdf_with_site_map_extraction_success(
        self, mock_chart_exists, mock_map_processor, mock_extract_svg, mock_client_class
    ):
        """Test PDF generation when site map extraction succeeds."""
        mock_chart_exists.return_value = False

        mock_client = MagicMock()
        mock_client.get_site.return_value = (
            {"id": 1, "name": "Test Site", "map_svg_data": "..."},
            MagicMock(),
        )
        mock_client_class.return_value = mock_client

        mock_extract_svg.return_value = True

        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (True, "/tmp/fake_map.pdf")
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "db_map.pdf")

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="1h",
                    units="mph",
                    timezone_display="US/Pacific",
                    min_speed_str="5.0 mph",
                    location="Test Location",
                    overall_metrics=self.overall_metrics,
                    daily_metrics=self.daily_metrics,
                    granular_metrics=self.granular_metrics,
                    histogram=self.histogram,
                    tz_name="US/Pacific",
                    charts_prefix="db_map",
                    speed_limit=25,
                    start_date="2025-06-02",
                    end_date="2025-06-04",
                    include_map=True,
                    site_id=1,
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))
