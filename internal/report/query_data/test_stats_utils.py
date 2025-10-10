#!/usr/bin/env python3

"""Tests for stats_utils module."""

import unittest
from unittest.mock import patch, MagicMock
import tempfile
import os

from stats_utils import (
    format_time,
    format_number,
    process_histogram,
    count_in_histogram_range,
    count_histogram_ge,
    plot_histogram,
    save_chart_as_pdf,
    chart_exists,
)


class TestStatsUtils(unittest.TestCase):
    """Test stats utilities functions."""

    def test_format_number(self):
        """Test numeric formatting."""
        self.assertEqual(format_number(3.14159), "3.14")
        self.assertEqual(format_number(None), "--")
        self.assertEqual(format_number("invalid"), "--")

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
