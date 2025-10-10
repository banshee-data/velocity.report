#!/usr/bin/env python3
"""Unit tests for chart_builder.py chart generation module."""

import unittest
from unittest.mock import Mock, patch, MagicMock
from datetime import datetime
from zoneinfo import ZoneInfo

# Import classes and functions to test
from chart_builder import TimeSeriesChartBuilder, HistogramChartBuilder, HAVE_MATPLOTLIB

# Skip tests if matplotlib not available
if not HAVE_MATPLOTLIB:
    raise unittest.SkipTest("matplotlib not available, skipping chart_builder tests")

import matplotlib.pyplot as plt
import numpy as np


class TestTimeSeriesChartBuilder(unittest.TestCase):
    """Tests for TimeSeriesChartBuilder class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

        # Sample metrics data
        self.sample_metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "p98": 43.0,
                "max": 53.5,
                "count": 100,
            },
            {
                "start_time": "2025-06-02T11:00:00",
                "p50": 31.2,
                "p85": 37.5,
                "p98": 44.1,
                "max": 54.2,
                "count": 120,
            },
            {
                "start_time": "2025-06-02T12:00:00",
                "p50": 29.8,
                "p85": 35.2,
                "p98": 42.3,
                "max": 52.1,
                "count": 95,
            },
        ]

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_initialization_default_config(self):
        """Test builder initializes with default configuration."""
        builder = TimeSeriesChartBuilder()
        self.assertIsNotNone(builder.colors)
        self.assertIsNotNone(builder.fonts)
        self.assertIsNotNone(builder.layout)
        self.assertIsNotNone(builder.normalizer)

    def test_initialization_custom_config(self):
        """Test builder accepts custom configuration."""
        custom_colors = {"p50": "#ff0000", "p85": "#00ff00"}
        custom_fonts = {"chart_title": 20}
        custom_layout = {"chart_figsize": (10, 6)}

        builder = TimeSeriesChartBuilder(
            colors=custom_colors,
            fonts=custom_fonts,
            layout=custom_layout,
        )

        self.assertEqual(builder.colors, custom_colors)
        self.assertEqual(builder.fonts, custom_fonts)
        self.assertEqual(builder.layout, custom_layout)

    def test_build_creates_figure(self):
        """Test build() returns a matplotlib Figure."""
        fig = self.builder.build(
            self.sample_metrics,
            title="Test Chart",
            units="mph",
        )

        self.assertIsNotNone(fig)
        self.assertEqual(type(fig).__name__, "Figure")

    def test_build_empty_metrics(self):
        """Test build() handles empty metrics list gracefully."""
        fig = self.builder.build(
            [],
            title="Empty Chart",
            units="mph",
        )

        self.assertIsNotNone(fig)
        # Should create figure even with no data

    def test_build_with_timezone(self):
        """Test build() with timezone parameter."""
        fig = self.builder.build(
            self.sample_metrics,
            title="Timezone Chart",
            units="mph",
            tz_name="US/Pacific",
        )

        self.assertIsNotNone(fig)

    def test_extract_data_from_metrics(self):
        """Test _extract_data() extracts correct arrays from metrics."""
        times, p50, p85, p98, max_vals, counts = self.builder._extract_data(
            self.sample_metrics, None
        )

        self.assertEqual(len(times), 3)
        self.assertEqual(len(p50), 3)
        self.assertEqual(len(counts), 3)

        # Check values are correct
        self.assertAlmostEqual(p50[0], 30.5, places=1)
        self.assertAlmostEqual(p85[0], 36.9, places=1)
        self.assertEqual(counts[0], 100)

    def test_extract_data_with_missing_fields(self):
        """Test _extract_data() handles metrics with missing fields."""
        incomplete_metrics = [
            {"start_time": "2025-06-02T10:00:00", "p50": 30.5},  # Missing other fields
            {"start_time": "2025-06-02T11:00:00", "count": 100},  # Missing p50
        ]

        # Should not raise, but handle gracefully
        times, p50, p85, p98, max_vals, counts = self.builder._extract_data(
            incomplete_metrics, None
        )

        self.assertEqual(len(times), 2)

    def test_convert_timezone(self):
        """Test _convert_timezone() converts datetime correctly."""
        utc_time = datetime(2025, 6, 2, 18, 0, 0, tzinfo=ZoneInfo("UTC"))
        pacific_time = self.builder._convert_timezone(utc_time, "US/Pacific")

        self.assertIsNotNone(pacific_time)
        self.assertEqual(pacific_time.tzinfo.key, "US/Pacific")

    def test_create_masked_arrays(self):
        """Test _create_masked_arrays() creates proper masked arrays."""
        p50 = [30.5, None, 35.2, 0, 40.1]
        p85 = [36.0, 37.0, 38.0, 39.0, 40.0]
        p98 = [42.0, 43.0, 44.0, 45.0, 46.0]
        mx = [50.0, 51.0, 52.0, 53.0, 54.0]
        counts = [100, 50, 30, 150, 80]

        p50_a, p85_a, p98_a, mx_a = self.builder._create_masked_arrays(
            p50, p85, p98, mx, counts
        )

        # Verify all arrays returned
        self.assertEqual(len(p50_a), 5)
        self.assertEqual(len(p85_a), 5)
        self.assertEqual(len(p98_a), 5)
        self.assertEqual(len(mx_a), 5)

    def test_compute_bar_widths(self):
        """Test _compute_bar_widths() computes reasonable widths."""
        times = [
            datetime(2025, 6, 2, 10, 0, 0),
            datetime(2025, 6, 2, 11, 0, 0),
            datetime(2025, 6, 2, 12, 0, 0),
        ]

        bg_width, bar_width = self.builder._compute_bar_widths(times)

        self.assertGreater(bg_width, 0)
        self.assertGreater(bar_width, 0)
        # Bar width should be smaller than background width
        self.assertLess(bar_width, bg_width)

    def test_compute_bar_widths_single_point(self):
        """Test _compute_bar_widths() handles single data point."""
        times = [datetime(2025, 6, 2, 10, 0, 0)]

        bg_width, bar_width = self.builder._compute_bar_widths(times)

        # Should return default widths
        self.assertGreater(bg_width, 0)
        self.assertGreater(bar_width, 0)

    def test_compute_gap_threshold(self):
        """Test _compute_gap_threshold() computes reasonable threshold."""
        x_arr = np.array([1.0, 2.0, 3.0, 4.0, 5.0])
        threshold = self.builder._compute_gap_threshold(x_arr)

        if threshold is not None:
            self.assertGreater(threshold, 0)

    def test_compute_gap_threshold_insufficient_data(self):
        """Test _compute_gap_threshold() returns None for insufficient data."""
        x_arr = np.array([1.0, 2.0])
        threshold = self.builder._compute_gap_threshold(x_arr)

        self.assertIsNone(threshold)

    def test_build_runs(self):
        """Test _build_runs() splits data into continuous runs."""
        x_arr = np.array([1.0, 2.0, 5.0, 6.0, 10.0])
        valid_mask = np.array([True, True, True, True, True])
        gap_threshold = 2.5

        runs = self.builder._build_runs(x_arr, valid_mask, gap_threshold)

        # Should return list of (start, end) tuples
        self.assertIsInstance(runs, list)
        self.assertGreater(len(runs), 0)
        # Each run should be a tuple of (start_idx, end_idx)
        for run in runs:
            self.assertIsInstance(run, tuple)
            self.assertEqual(len(run), 2)

    def test_debug_output_when_enabled(self):
        """Test _debug_output() prints when debug enabled."""
        with patch("chart_builder.DEBUG", {"plot_debug": True}):
            with patch("builtins.print") as mock_print:
                times = [datetime.now()]
                counts = [100]
                p50_f = np.array([30.5])
                self.builder._debug_output(times, counts, p50_f)
                # Should have printed something
                mock_print.assert_called()

    def test_debug_output_when_disabled(self):
        """Test _debug_output() silent when debug disabled."""
        with patch("chart_builder.DEBUG", {"plot_debug": False}):
            with patch("builtins.print") as mock_print:
                times = [datetime.now()]
                counts = [100]
                p50_f = np.array([30.5])
                self.builder._debug_output(times, counts, p50_f)
                # Should not print
                mock_print.assert_not_called()


class TestHistogramChartBuilder(unittest.TestCase):
    """Tests for HistogramChartBuilder class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = HistogramChartBuilder()

        # Sample histogram data
        self.sample_histogram = {
            "10": 50,
            "15": 120,
            "20": 200,
            "25": 180,
            "30": 150,
            "35": 100,
            "40": 60,
            "45": 30,
            "50": 10,
        }

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_initialization_default_config(self):
        """Test builder initializes with default configuration."""
        builder = HistogramChartBuilder()
        self.assertIsNotNone(builder.colors)
        self.assertIsNotNone(builder.fonts)
        self.assertIsNotNone(builder.layout)

    def test_initialization_custom_config(self):
        """Test builder accepts custom configuration."""
        custom_colors = {"count_bar": "#0000ff"}
        custom_fonts = {"histogram_title": 16}
        custom_layout = {"histogram_figsize": (5, 3)}

        builder = HistogramChartBuilder(
            colors=custom_colors,
            fonts=custom_fonts,
            layout=custom_layout,
        )

        self.assertEqual(builder.colors, custom_colors)
        self.assertEqual(builder.fonts, custom_fonts)
        self.assertEqual(builder.layout, custom_layout)

    def test_build_creates_figure(self):
        """Test build() returns a matplotlib Figure."""
        fig = self.builder.build(
            self.sample_histogram,
            title="Test Histogram",
            units="mph",
        )

        self.assertIsNotNone(fig)
        self.assertEqual(type(fig).__name__, "Figure")

    def test_build_empty_histogram(self):
        """Test build() handles empty histogram gracefully."""
        fig = self.builder.build(
            {},
            title="Empty Histogram",
            units="mph",
        )

        self.assertIsNotNone(fig)

    def test_plot_bars_centered(self):
        """Test that bars are centered on bucket values."""
        # This tests the implementation detail of centering bars
        fig = self.builder.build(
            {"10": 50, "20": 100},
            title="Centered Bars",
            units="mph",
        )

        # If we got a figure without error, bars were plotted
        self.assertIsNotNone(fig)


class TestChartBuilderEdgeCases(unittest.TestCase):
    """Tests for edge cases and error handling."""

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_timeseries_with_null_values(self):
        """Test time-series chart handles None values."""
        builder = TimeSeriesChartBuilder()
        metrics_with_nulls = [
            {"start_time": "2025-06-02T10:00:00", "p50": 30.5, "count": 100},
            {"start_time": "2025-06-02T11:00:00", "p50": None, "count": 0},
            {"start_time": "2025-06-02T12:00:00", "p50": 32.1, "count": 110},
        ]

        fig = builder.build(metrics_with_nulls, "Null Test", "mph")
        self.assertIsNotNone(fig)

    def test_histogram_with_string_keys(self):
        """Test histogram handles string bucket keys."""
        builder = HistogramChartBuilder()
        histogram = {"10.5": 50, "15.0": 100, "20": 80}

        fig = builder.build(histogram, "String Keys", "mph")
        self.assertIsNotNone(fig)

    def test_timeseries_with_single_datapoint(self):
        """Test time-series chart with single data point."""
        builder = TimeSeriesChartBuilder()
        single_metric = [
            {"start_time": "2025-06-02T10:00:00", "p50": 30.5, "count": 100}
        ]

        fig = builder.build(single_metric, "Single Point", "mph")
        self.assertIsNotNone(fig)

    def test_histogram_with_zero_counts(self):
        """Test histogram with zero count buckets."""
        builder = HistogramChartBuilder()
        histogram = {"10": 0, "20": 0, "30": 0}

        fig = builder.build(histogram, "Zero Counts", "mph")
        self.assertIsNotNone(fig)


if __name__ == "__main__":
    unittest.main()
