#!/usr/bin/env python3

"""Tests for stats_utils module."""

import unittest
from unittest.mock import patch, MagicMock
import tempfile
import os

from pdf_generator.core.stats_utils import (
    format_time,
    format_number,
    process_histogram,
    count_in_histogram_range,
    count_histogram_ge,
    plot_histogram,
    chart_exists,
)
from pdf_generator.core.chart_saver import save_chart_as_pdf


class TestStatsUtils(unittest.TestCase):
    """Test stats utilities functions."""

    def test_format_time_with_timezone(self):
        """Test time formatting with timezone conversion."""
        # Test with valid datetime string and timezone
        result = format_time("2025-10-10T12:00:00Z", "America/New_York")
        self.assertIn("10/10", result)

    def test_format_time_naive_datetime(self):
        """Test time formatting with naive datetime (no timezone info)."""
        # Ensure naive datetime strings (no tz) are parsed and formatted
        # Pass a datetime string without timezone info
        result = format_time("2025-10-10 12:00:00", "America/New_York")
        self.assertIn("10/10", result)

    def test_format_time_with_invalid_timezone(self):
        """Test time formatting with invalid timezone falls back to UTC."""
        # Verify function falls back sensibly when timezone is invalid
        result = format_time("2025-10-10T12:00:00Z", "Invalid/Timezone")
        self.assertIn("10/10", result)

    def test_format_time_exception(self):
        """Test time formatting with unparseable input."""
        # Should return the original value when parsing fails
        result = format_time("not a date", None)
        self.assertEqual(result, "not a date")

    def test_format_number(self):
        """Test numeric formatting."""
        self.assertEqual(format_number(3.14159), "3.14")
        self.assertEqual(format_number(None), "--")
        self.assertEqual(format_number("invalid"), "--")

    def test_format_number_with_nan(self):
        """Test numeric formatting with NaN."""
        import numpy as np

        # Ensure NaN is handled gracefully by numeric formatting
        self.assertEqual(format_number(np.nan), "--")

    def test_process_histogram(self):
        """Test histogram processing."""
        histogram = {
            "10": 25,
            "15": 45,
            "20": 125,
            "25": 238,
            "30": 321,
        }

        numeric_buckets, total, ranges = process_histogram(
            histogram, cutoff=5.0, bucket_size=5.0, max_bucket=50.0
        )

        self.assertEqual(total, 754)
        self.assertEqual(len(ranges), 9)  # (50-5)/5 = 9 ranges
        self.assertEqual(ranges[0], (5.0, 10.0))
        self.assertEqual(ranges[-1], (45.0, 50.0))

    def test_process_histogram_with_invalid_keys(self):
        """Test histogram processing with invalid keys that can't convert to float."""
        # Ensure invalid keys/values are skipped and valid entries processed
        histogram = {
            "10": 25,
            "invalid": 45,  # Can't convert to float, but value is valid int
            "not_a_number": "also_invalid",  # Both key and value invalid
        }

        numeric_buckets, total, ranges = process_histogram(histogram)

        # "10" should be processed successfully
        self.assertIn(10.0, numeric_buckets)
        self.assertEqual(numeric_buckets[10.0], 25)

        # Total should include the valid int from "invalid" key and skip fully invalid entries
        # and should skip the fully invalid entry
        self.assertEqual(total, 70)  # 25 + 45

    def test_process_histogram_with_nan_value(self):
        """Test histogram processing with NaN in value."""
        import numpy as np

        # Verify NaN values in histogram entries are handled gracefully
        histogram = {
            "10": 25,
            "15": np.nan,  # This will trigger the int() conversion exception
        }

        numeric_buckets, total, ranges = process_histogram(histogram)

        # Should handle the exception and only count the valid entry
        self.assertEqual(total, 25)

    def test_count_functions(self):
        """Test histogram counting functions."""
        numeric_buckets = {10.0: 25, 15.0: 45, 20.0: 125, 25.0: 238}

        # Test range counting
        count = count_in_histogram_range(numeric_buckets, 10.0, 20.0)
        self.assertEqual(count, 70)  # 25 + 45

        # Test >= counting
        count_ge = count_histogram_ge(numeric_buckets, 20.0)
        self.assertEqual(count_ge, 363)  # 125 + 238

    def test_chart_exists(self):
        """Test chart existence checking."""
        with tempfile.TemporaryDirectory() as tmpdir:
            test_chart = os.path.join(tmpdir, "test_stats.pdf")

            # File doesn't exist
            self.assertFalse(chart_exists(os.path.join(tmpdir, "test"), "stats"))

            # Create file
            with open(test_chart, "w") as f:
                f.write("test")

            # File exists
            self.assertTrue(chart_exists(os.path.join(tmpdir, "test"), "stats"))

    @patch("matplotlib.pyplot.subplots")
    def test_plot_histogram_success(self, mock_subplots):
        """Test successful histogram plotting."""
        mock_fig = MagicMock()
        mock_ax = MagicMock()
        mock_subplots.return_value = (mock_fig, mock_ax)

        histogram = {"10-15": 25, "15-20": 45}

        result = plot_histogram(histogram, "Test", "mph")

        self.assertEqual(result, mock_fig)
        mock_subplots.assert_called_once()
        mock_ax.bar.assert_called_once()

    @patch("matplotlib.pyplot.subplots")
    def test_plot_histogram_no_data(self, mock_subplots):
        """Test histogram plotting with no data."""
        mock_fig = MagicMock()
        mock_ax = MagicMock()
        mock_subplots.return_value = (mock_fig, mock_ax)

        result = plot_histogram({}, "Test", "mph")

        self.assertEqual(result, mock_fig)
        mock_ax.text.assert_called_once()

    def test_save_chart_as_pdf_success(self):
        """Test successful chart saving."""
        mock_fig = MagicMock()

        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "test.pdf")

            result = save_chart_as_pdf(mock_fig, output_path)

            self.assertTrue(result)
            # Check that savefig was called (with bbox and pad kwargs)
            mock_fig.savefig.assert_called()
            # Verify the output path was used
            self.assertIn(output_path, str(mock_fig.savefig.call_args))

    def test_save_chart_as_pdf_failure(self):
        """Test chart saving failure."""
        mock_fig = MagicMock()
        mock_fig.savefig.side_effect = Exception("Save failed")

        result = save_chart_as_pdf(mock_fig, "/invalid/path/test.pdf")

        self.assertFalse(result)


if __name__ == "__main__":
    unittest.main()
