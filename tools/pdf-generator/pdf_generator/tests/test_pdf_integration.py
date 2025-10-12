#!/usr/bin/env python3
"""Integration tests for PDF generation pipeline.

Tests the full end-to-end workflow with mocked dependencies
and validates generated .tex file content.
"""

import unittest
import tempfile
import os
from unittest.mock import patch, MagicMock

from pdf_generator.core.pdf_generator import generate_pdf_report


class TestPDFIntegration(unittest.TestCase):
    """End-to-end integration tests for PDF generation."""

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
    def test_generate_pdf_creates_tex_file(self, mock_chart_exists, mock_map_processor):
        """Test that PDF generation creates .tex file when LaTeX fails."""
        # Mock chart_exists to return False (no charts)
        mock_chart_exists.return_value = False

        # Mock MapProcessor
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

            # Run PDF generation - it will create .tex file
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
                )
            except Exception:
                # Expected to fail without LaTeX compiler
                pass

            # .tex file should be created
            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(
                os.path.exists(tex_path), f"Expected .tex file at {tex_path}"
            )

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_tex_file_contains_expected_structure(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test that generated .tex file has expected LaTeX structure."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            with open(tex_path, "r") as f:
                content = f.read()

            # Check LaTeX document structure
            self.assertIn(r"\documentclass", content)
            self.assertIn(r"\begin{document}", content)
            self.assertIn(r"\end{document}", content)

            # Check required packages
            self.assertIn(r"\usepackage{fontspec}", content)
            self.assertIn(r"\usepackage{graphicx}", content)

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_tex_file_contains_correct_metrics(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test that .tex file contains correct metric values."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            with open(tex_path, "r") as f:
                content = f.read()

            # Check vehicle count (may be formatted with comma)
            self.assertTrue(
                "3469" in content or "3,469" in content,
                "Vehicle count should appear in .tex file",
            )

            # Check key metrics appear
            self.assertIn("53.52", content)  # Max speed
            self.assertIn("43.05", content)  # p98
            self.assertIn("36.94", content)  # p85
            self.assertIn("30.54", content)  # p50

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_tex_file_contains_location_info(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test that .tex file contains location information."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            with open(tex_path, "r") as f:
                content = f.read()

            # Check location appears
            self.assertIn("Clarendon Avenue, San Francisco", content)

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_tex_file_contains_date_range(self, mock_chart_exists, mock_map_processor):
        """Test that .tex file contains correct date range."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            with open(tex_path, "r") as f:
                content = f.read()

            # Check dates appear in content
            self.assertIn("2025-06-02", content)
            self.assertIn("2025-06-04", content)

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_tex_file_contains_footer_with_dates_and_page_numbers(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test that .tex file contains footer with date range and page numbers."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            with open(tex_path, "r") as f:
                content = f.read()

            # Check footer contains date range on left
            self.assertIn(r"\fancyfoot[L]{\small 2025-06-02 to 2025-06-04}", content)
            # Check footer contains page number on right
            self.assertIn(r"\fancyfoot[R]{\small Page \thepage}", content)
            # Check footer rule is present
            self.assertIn(r"\renewcommand{\footrulewidth}{0.8pt}", content)
            # Check that date range is NOT in header center anymore
            self.assertNotIn(r"\fancyhead[C]", content)

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_tex_file_contains_histogram_table(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test that .tex file contains histogram table data."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            with open(tex_path, "r") as f:
                content = f.read()

            # Check for histogram table content
            # Should have bucket ranges and counts
            self.assertIn("66", content)  # Count for first bucket
            self.assertIn("971", content)  # Highest count

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_tex_file_contains_daily_metrics_table(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test that .tex file contains daily metrics table."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            with open(tex_path, "r") as f:
                content = f.read()

            # Check for daily metrics from our test data
            self.assertIn("891", content)  # Count from first day
            self.assertIn("1315", content)  # Count from second day
            self.assertIn("1263", content)  # Count from third day

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_tex_file_contains_survey_parameters(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test that .tex file contains survey parameters section."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            with open(tex_path, "r") as f:
                content = f.read()

            # Check for survey parameters
            self.assertIn("Survey Parameters", content)
            self.assertIn("1h", content)  # Roll-up period
            self.assertIn("mph", content)  # Units
            self.assertIn("US/Pacific", content)  # Timezone


class TestPDFGenerationEdgeCases(unittest.TestCase):
    """Test edge cases in PDF generation."""

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_generate_without_overall_metrics(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test PDF generation without overall metrics (lines 284-289)."""
        # Return True for histogram chart
        mock_chart_exists.return_value = True
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

            # Create a dummy histogram PDF
            hist_path = os.path.join(tmpdir, "test_histogram.pdf")
            with open(hist_path, "w") as f:
                f.write("dummy pdf")

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
                    overall_metrics=None,  # No overall metrics
                    daily_metrics=None,
                    granular_metrics=[],
                    histogram={"10": 50},
                    tz_name="UTC",
                    charts_prefix="test",
                    speed_limit=25,
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_with_stats_chart(self, mock_chart_exists, mock_map_processor):
        """Test PDF generation with stats chart available (lines 368-373)."""

        # Return True only for stats chart
        def chart_side_effect(prefix, chart_type):
            return chart_type == "stats"

        mock_chart_exists.side_effect = chart_side_effect
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                    charts_prefix="test",
                    speed_limit=25,
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_with_map_success(self, mock_chart_exists, mock_map_processor):
        """Test PDF generation with successful map processing (lines 398-401)."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        # Map processing succeeds with a path
        mock_processor.process_map.return_value = (True, "test_map.pdf")
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

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
                    charts_prefix="test",
                    speed_limit=25,
                )
            except Exception:
                pass

            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_mono_font_fallback(self, mock_chart_exists, mock_map_processor):
        """Test mono font fallback when font file doesn't exist (line 232)."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

            # Mock os.path.exists to return False for mono font
            with patch(
                "pdf_generator.core.pdf_generator.os.path.exists"
            ) as mock_exists:
                # First call is for font dir, return False for mono font check
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
                        charts_prefix="test",
                        speed_limit=25,
                    )
                except Exception:
                    pass

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_generate_with_empty_histogram(self, mock_chart_exists, mock_map_processor):
        """Test PDF generation with empty histogram."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

            # Should not raise exception with empty histogram
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
                    overall_metrics=[{"Count": 0}],
                    daily_metrics=None,
                    granular_metrics=[],
                    histogram={},  # Empty histogram
                    tz_name="UTC",
                    charts_prefix="test",
                    speed_limit=25,
                )
            except Exception:
                pass  # LaTeX will fail but generation should work

            # .tex file should exist
            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_generate_with_no_daily_metrics(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test PDF generation without daily metrics."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

            try:
                generate_pdf_report(
                    output_path=output_path,
                    start_iso="2025-06-02T00:00:00-07:00",
                    end_iso="2025-06-04T23:59:59-07:00",
                    group="24h",  # 24h group shouldn't produce daily
                    units="mph",
                    timezone_display="UTC",
                    min_speed_str="0.0 mph",
                    location="Test Location",
                    overall_metrics=[{"Count": 100}],
                    daily_metrics=None,  # No daily metrics
                    granular_metrics=[],
                    histogram={"10": 50, "20": 50},
                    tz_name="UTC",
                    charts_prefix="test",
                    speed_limit=25,
                )
            except Exception:
                pass

            # .tex file should exist
            tex_path = output_path.replace(".pdf", ".tex")
            self.assertTrue(os.path.exists(tex_path))

    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    def test_pdf_generation_all_engines_fail(
        self, mock_chart_exists, mock_map_processor
    ):
        """Test when all PDF engines fail and TEX generation also fails (lines 416-429)."""
        mock_chart_exists.return_value = False
        mock_processor = MagicMock()
        mock_processor.process_map.return_value = (False, None)
        mock_map_processor.return_value = mock_processor

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test_report.pdf")

            # Mock DocumentBuilder to return a document that fails generation
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
                        charts_prefix="test",
                        speed_limit=25,
                    )

                self.assertIn("PDF generation failed", str(context.exception))


if __name__ == "__main__":
    unittest.main()
