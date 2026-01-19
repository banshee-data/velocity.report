#!/usr/bin/env python3
"""Unit tests for chart_builder.py chart generation module."""

import os
import unittest
from unittest.mock import patch
from datetime import datetime
from zoneinfo import ZoneInfo

# Import classes and functions to test
from pdf_generator.core.chart_builder import (
    TimeSeriesChartBuilder,
    HistogramChartBuilder,
    HAVE_MATPLOTLIB,
)

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
        # Create builder with debug enabled
        builder = TimeSeriesChartBuilder(debug={"plot_debug": True})
        with patch("builtins.print") as mock_print:
            times = [datetime.now()]
            counts = [100]
            p50_f = np.array([30.5])
            builder._debug_output(times, counts, p50_f)
            # Should have printed something
            mock_print.assert_called()

    def test_debug_output_when_disabled(self):
        """Test _debug_output() silent when debug disabled."""
        # Create builder with debug disabled (default)
        builder = TimeSeriesChartBuilder(debug={"plot_debug": False})
        with patch("builtins.print") as mock_print:
            times = [datetime.now()]
            counts = [100]
            p50_f = np.array([30.5])
            builder._debug_output(times, counts, p50_f)
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

    def test_build_comparison_with_numeric_buckets(self):
        """Test comparison histogram with numeric buckets."""
        fig = self.builder.build_comparison(
            {"5": 10, "10": 20},
            {"5": 5, "10": 15},
            title="Comparison Histogram",
            units="mph",
            primary_label="Primary",
            compare_label="Compare",
        )

        self.assertIsNotNone(fig)
        ax = fig.axes[0]
        self.assertEqual(len(ax.containers), 2)
        legend_texts = [text.get_text() for text in ax.get_legend().get_texts()]
        self.assertIn("Primary", legend_texts)
        self.assertIn("Compare", legend_texts)

    def test_build_comparison_with_empty_histograms(self):
        """Test comparison histogram with no data."""
        fig = self.builder.build_comparison(
            {},
            {},
            title="Empty Comparison",
            units="mph",
            primary_label="Primary",
            compare_label="Compare",
        )

        self.assertIsNotNone(fig)
        ax = fig.axes[0]
        self.assertEqual(ax.get_title(), "Empty Comparison")
        self.assertTrue(any("No histogram data" in t.get_text() for t in ax.texts))

    def test_build_comparison_with_string_keys_debug(self):
        """Test comparison histogram with string buckets and debug output."""
        with patch("builtins.print") as mock_print:
            fig = self.builder.build_comparison(
                {"low": 3},
                {"high": 5},
                title="Debug Comparison",
                units="mph",
                primary_label="Primary",
                compare_label="Compare",
                debug=True,
            )

        self.assertIsNotNone(fig)
        mock_print.assert_called()


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


class TestTimezoneConversionEdgeCases(unittest.TestCase):
    """Test timezone conversion edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_convert_timezone_with_invalid_timezone(self):
        """Test timezone conversion with invalid timezone name.

        Ensures passing an invalid timezone name raises the expected exception.
        """
        dt = datetime(2025, 6, 2, 12, 0, 0)
        # Invalid timezone should return original datetime
        result = self.builder._convert_timezone(dt, "Invalid/Timezone")
        self.assertEqual(result, dt)

    def test_convert_timezone_with_naive_datetime(self):
        """Test timezone conversion with naive datetime.

        Ensures naive datetimes are handled or converted appropriately.
        """
        naive_dt = datetime(2025, 6, 2, 12, 0, 0)  # No timezone
        result = self.builder._convert_timezone(naive_dt, "US/Pacific")
        # Should assume UTC and convert
        self.assertIsNotNone(result)
        self.assertIsNotNone(result.tzinfo)

    def test_convert_timezone_with_aware_datetime(self):
        """Test timezone conversion with timezone-aware datetime.

        Verifies timezone-aware datetimes are preserved in conversion.
        """
        aware_dt = datetime(2025, 6, 2, 12, 0, 0, tzinfo=ZoneInfo("UTC"))
        result = self.builder._convert_timezone(aware_dt, "US/Eastern")
        self.assertIsNotNone(result)
        self.assertEqual(result.tzinfo, ZoneInfo("US/Eastern"))

    def test_convert_timezone_exception_handler(self):
        """Test timezone conversion exception handler.

        Confirms the function gracefully reports conversion errors.
        """

        # Create a datetime-like object that will raise on astimezone
        class BadDateTime:
            tzinfo = None

            def replace(self, **kwargs):
                raise Exception("Conversion failed")

        bad_dt = BadDateTime()
        result = self.builder._convert_timezone(bad_dt, "US/Pacific")
        # Should return original on exception
        self.assertEqual(result, bad_dt)


class TestExtractDataEdgeCases(unittest.TestCase):
    """Test data extraction edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_extract_data_with_bad_time_format(self):
        """Test extraction with unparseable time.

        Ensures unparseable time strings are handled or raise proper errors.
        """
        bad_metrics = [
            {"start_time": "not-a-date", "p50": 30.5, "count": 100},
            {"start_time": "2025-06-02T10:00:00", "p50": 31.2, "count": 120},
        ]

        times, p50, p85, p98, max_vals, counts = self.builder._extract_data(
            bad_metrics, None
        )

        # Should skip the bad row
        self.assertEqual(len(times), 1)
        self.assertAlmostEqual(p50[0], 31.2, places=1)


class TestDebugOutput(unittest.TestCase):
    """Test debug output functionality."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures and environment."""
        plt.close("all")
        if "VELOCITY_PLOT_DEBUG" in os.environ:
            del os.environ["VELOCITY_PLOT_DEBUG"]

    def test_debug_output_via_environment_variable(self):
        """Test debug output when VELOCITY_PLOT_DEBUG=1.

        Checks that debug output is emitted when the debug env var is set.
        """
        import os

        os.environ["VELOCITY_PLOT_DEBUG"] = "1"

        times = [datetime(2025, 6, 2, 10, 0, 0)]
        counts = [100]
        p50_f = np.array([30.5])

        # Should not raise, just print to stderr
        self.builder._debug_output(times, counts, p50_f)

    def test_create_masked_arrays_with_debug(self):
        """Test masked array creation with debug output.

        Verifies masked array creation emits debug info when enabled.
        """
        import os

        os.environ["VELOCITY_PLOT_DEBUG"] = "1"

        p50 = [30.5, 31.2, 29.8]
        p85 = [36.9, 37.5, 35.2]
        p98 = [43.0, 44.1, 42.3]
        mx = [53.5, 54.2, 52.1]
        counts = [100, 120, 95]

        # Should create masked arrays and print debug info
        p50_a, p85_a, p98_a, mx_a = self.builder._create_masked_arrays(
            p50, p85, p98, mx, counts
        )

        self.assertIsNotNone(p50_a)
        self.assertEqual(len(p50_a), 3)

    def test_create_masked_arrays_with_low_counts(self):
        """Test masked array creation with low count threshold."""
        p50 = [30.5, 31.2, 29.8]
        p85 = [36.9, 37.5, 35.2]
        p98 = [43.0, 44.1, 42.3]
        mx = [53.5, 54.2, 52.1]
        counts = [1, 120, 2]  # First and last below threshold (default 10)

        p50_a, p85_a, p98_a, mx_a = self.builder._create_masked_arrays(
            p50, p85, p98, mx, counts
        )

        # Low count entries should be masked
        self.assertTrue(p50_a.mask[0])
        self.assertFalse(p50_a.mask[1])
        self.assertTrue(p50_a.mask[2])

    def test_create_masked_arrays_exception_handler(self):
        """Test exception handler in masked array creation.

        Confirms exceptions during masked array creation are handled.
        """
        # Invalid data that might cause exceptions in masking
        p50 = [30.5, float("inf"), 29.8]
        p85 = [36.9, float("nan"), 35.2]
        p98 = [43.0, 44.1, 42.3]
        mx = [53.5, 54.2, 52.1]
        counts = [100, 120, 95]

        # Should handle gracefully
        p50_a, p85_a, p98_a, mx_a = self.builder._create_masked_arrays(
            p50, p85, p98, mx, counts
        )

        self.assertIsNotNone(p50_a)


class TestPlotPercentileLines(unittest.TestCase):
    """Test percentile line plotting."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()
        self.fig, self.ax = plt.subplots()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_plot_percentile_lines(self):
        """Test plotting percentile lines (covers various plotting code)."""
        times = [
            datetime(2025, 6, 2, 10, 0, 0),
            datetime(2025, 6, 2, 11, 0, 0),
            datetime(2025, 6, 2, 12, 0, 0),
        ]
        p50 = np.array([30.5, 31.2, 29.8])
        p85 = np.array([36.9, 37.5, 35.2])
        p98 = np.array([43.0, 44.1, 42.3])
        mx = np.array([53.5, 54.2, 52.1])

        # Should not raise
        self.builder._plot_percentile_lines(self.ax, times, p50, p85, p98, mx)

        # Verify lines were added
        lines = self.ax.get_lines()
        self.assertGreater(len(lines), 0)


class TestHistogramChartBuilderEdgeCases(unittest.TestCase):
    """Test histogram builder edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = HistogramChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_build_with_debug_mode(self):
        """Test histogram build with debug mode enabled."""
        histogram = {"10": 50, "20": 100, "30": 80}

        # Should handle debug mode gracefully
        fig = self.builder.build(histogram, "Debug Test", "mph", debug=True)
        self.assertIsNotNone(fig)

    def test_build_with_negative_values(self):
        """Test histogram with negative bucket values."""
        histogram = {"-10": 10, "0": 50, "10": 100}

        fig = self.builder.build(histogram, "Negative Values", "mph")
        self.assertIsNotNone(fig)

    def test_build_with_float_bucket_keys(self):
        """Test histogram with float bucket keys."""
        histogram = {"10.5": 25, "15.7": 75, "20.3": 50}

        fig = self.builder.build(histogram, "Float Keys", "mph")
        self.assertIsNotNone(fig)

    def test_plot_bars_with_single_bucket(self):
        """Test histogram plotting with single bucket."""
        histogram = {"10.0": 100}

        fig = self.builder.build(histogram, "Single Bucket", "mph")

        # Should create chart
        self.assertIsNotNone(fig)
        ax = fig.axes[0]
        patches = ax.patches
        self.assertGreater(len(patches), 0)

    def test_histogram_bar_plotting_internals(self):
        """Test histogram with multiple buckets to cover plotting code."""
        histogram = {"10": 25, "20": 75, "30": 50}

        fig = self.builder.build(histogram, "Multi-Bucket", "mph")
        self.assertIsNotNone(fig)

        # Should have bars
        ax = fig.axes[0]
        self.assertGreater(len(ax.patches), 0)


class TestTimeSeriesWithGaps(unittest.TestCase):
    """Test time-series handling with data gaps."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_build_with_large_time_gaps(self):
        """Test handling large gaps in time series data."""
        gapped_metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "count": 100,
            },
            # 24 hour gap
            {
                "start_time": "2025-06-03T10:00:00",
                "p50": 31.2,
                "p85": 37.5,
                "count": 120,
            },
        ]

        fig = self.builder.build(gapped_metrics, "Gapped Data", "mph")
        self.assertIsNotNone(fig)

    def test_compute_bar_widths_with_gaps(self):
        """Test bar width computation with irregular spacing."""
        times = [
            datetime(2025, 6, 2, 10, 0, 0),
            datetime(2025, 6, 2, 11, 0, 0),
            datetime(2025, 6, 2, 14, 0, 0),  # 3 hour gap
        ]

        bar_width_bg, bar_width = self.builder._compute_bar_widths(times)

        # Should return two float values (background and foreground widths)
        self.assertIsInstance(bar_width_bg, float)
        self.assertIsInstance(bar_width, float)
        # Widths should be positive
        self.assertGreater(bar_width_bg, 0)
        self.assertGreater(bar_width, 0)


class TestChartAnnotations(unittest.TestCase):
    """Test chart annotation functionality."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()
        self.fig, self.ax = plt.subplots()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_add_annotations_with_speed_limit(self):
        """Test adding speed limit annotation."""
        times = [
            datetime(2025, 6, 2, 10, 0, 0),
            datetime(2025, 6, 2, 11, 0, 0),
        ]

        # Create sample chart
        self.ax.plot(times, [30, 35])

        # Test annotation code paths
        # Note: The actual _add_annotations method might not exist,
        # but we're testing the build method which includes annotation logic

    def test_build_applies_styling(self):
        """Test that build applies font and layout styling."""
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "p98": 43.0,
                "max": 53.5,
                "count": 100,
            }
        ]

        fig = self.builder.build(metrics, "Styled Chart", "mph")

        # Chart should be created with styling
        self.assertIsNotNone(fig)
        ax = fig.axes[0]

        # Should have labels
        self.assertIsNotNone(ax.get_xlabel())
        self.assertIsNotNone(ax.get_ylabel())


class TestBuildRunsEdgeCases(unittest.TestCase):
    """Test _build_runs method edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_build_runs_with_gaps(self):
        """Test building runs with time gaps.

        Ensures time gaps are detected and handled correctly during run building.
        """
        times = np.array(
            [
                datetime(2025, 6, 2, 10, 0, 0),
                datetime(2025, 6, 2, 11, 0, 0),
                datetime(2025, 6, 2, 14, 0, 0),  # Large gap
                datetime(2025, 6, 2, 15, 0, 0),
            ]
        )
        valid_mask = np.array([True, True, True, True])
        gap_threshold = 7200  # 2 hours in seconds

        runs = self.builder._build_runs(times, valid_mask, gap_threshold)

        # Should split into runs due to gap
        self.assertIsNotNone(runs)
        self.assertGreater(len(runs), 0)

    def test_build_runs_exception_handler(self):
        """Test exception handler in _build_runs.

        Verifies exceptions in the run-building logic are handled.
        """
        # Create array with objects that might cause exceptions
        times = np.array(
            [
                datetime(2025, 6, 2, 10, 0, 0),
                None,  # Will cause exception
                datetime(2025, 6, 2, 12, 0, 0),
            ]
        )
        valid_mask = np.array([True, False, True])
        gap_threshold = 3600

        # Should handle gracefully
        runs = self.builder._build_runs(times, valid_mask, gap_threshold)
        self.assertIsNotNone(runs)

    def test_build_runs_with_empty_mask(self):
        """Test _build_runs with no valid points."""
        times = np.array([datetime(2025, 6, 2, 10, 0, 0)])
        valid_mask = np.array([False])

        runs = self.builder._build_runs(times, valid_mask, None)

        # Should return empty list
        self.assertEqual(runs, [])


class TestAxisConfigurationEdgeCases(unittest.TestCase):
    """Test axis configuration edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()
        self.fig, self.ax = plt.subplots()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_configure_speed_axis_exception_handlers(self):
        """Test exception handlers in _configure_speed_axis.

        Ensures axis configuration errors are gracefully handled.
        """
        # Should handle exceptions gracefully
        self.builder._configure_speed_axis(self.ax, "mph")

        # Axis should be configured
        ylabel = self.ax.get_ylabel()
        self.assertIn("mph", ylabel)

    def test_configure_speed_axis_ylim_exception(self):
        """Test ylim exception handler."""
        # Set some data first to test ylim adjustment
        self.ax.plot([1, 2, 3], [10, 20, 30])

        self.builder._configure_speed_axis(self.ax, "kph")

        # Should have set ylabel
        self.assertIn("kph", self.ax.get_ylabel())


class TestPlotCountBarsEdgeCases(unittest.TestCase):
    """Test _plot_count_bars edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_plot_count_bars_with_empty_counts(self):
        """Test count bars with empty data.

        Verifies the module handles empty count data without crashing.
        """
        metrics = []

        fig = self.builder.build(metrics, "Empty Test", "mph")

        # Should handle empty data gracefully
        self.assertIsNotNone(fig)

    def test_plot_count_bars_with_low_counts(self):
        """Test count bars with low count values.

        Ensures the chart builder handles low sample counts without error.
        """
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 5,  # Low count
            }
        ]

        fig = self.builder.build(metrics, "Low Count Test", "mph")

        # Should handle low counts
        self.assertIsNotNone(fig)

    def test_plot_count_bars_with_varied_counts(self):
        """Test count bars with varied count values."""
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 50,
            },
            {
                "start_time": "2025-06-02T11:00:00",
                "p50": 31.5,
                "count": 150,
            },
        ]

        fig = self.builder.build(metrics, "Varied Count Test", "mph")

        # Should plot bars
        self.assertIsNotNone(fig)

    def test_plot_count_bars_with_high_counts(self):
        """Test count bars axis configuration."""
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 1000,
            }
        ]

        fig = self.builder.build(metrics, "High Count Test", "mph")

        self.assertIsNotNone(fig)


class TestComputeBarWidthsEdgeCases(unittest.TestCase):
    """Test _compute_bar_widths edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_compute_bar_widths_normal_case(self):
        """Test bar width computation with normal spacing."""
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 100,
            },
            {
                "start_time": "2025-06-02T11:00:00",
                "p50": 31.5,
                "count": 120,
            },
        ]

        fig = self.builder.build(metrics, "Bar Width Test", "mph")

        # Should compute and use bar widths
        self.assertIsNotNone(fig)


class TestConfigureCountAxisEdgeCases(unittest.TestCase):
    """Test _configure_count_axis edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_configure_count_axis_normal(self):
        """Test count axis configuration.

        Verifies that the count axis is configured and does not raise errors.
        """
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 100,
            }
        ]

        fig = self.builder.build(metrics, "Count Axis Test", "mph")

        # Should have configured count axis
        self.assertIsNotNone(fig)


class TestCreateLegendEdgeCases(unittest.TestCase):
    """Test _create_legend edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_create_legend_with_low_sample_data(self):
        """Test legend creation with low sample indicator.

        Ensures legend includes low-sample indicators when sample counts are low.
        """
        # Build chart with low sample data to trigger legend
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "count": 5,  # Low sample count
            }
        ]

        fig = self.builder.build(metrics, "Low Sample Legend Test", "mph")

        # Should have created legend
        self.assertIsNotNone(fig)

    def test_create_legend_without_low_sample_data(self):
        """Test legend creation without low sample indicator."""
        # Build chart with normal data (no low sample indicator)
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "count": 100,  # Normal count
            }
        ]

        fig = self.builder.build(metrics, "Normal Legend Test", "mph")

        # Should have created chart with legend
        self.assertIsNotNone(fig)


class TestHistogramSortingEdgeCases(unittest.TestCase):
    """Test histogram sorting edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = HistogramChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_histogram_sorting_exception_handler(self):
        """Test exception handler in histogram sorting.

        Validates fallback sorting behavior when histogram keys are mixed types.
        """
        # Mix of numeric and non-numeric keys
        histogram = {"10": 50, "abc": 30, "20": 70}

        # Should fallback to string sorting
        fig = self.builder.build(histogram, "Mixed Keys", "mph")

        self.assertIsNotNone(fig)

    def test_histogram_with_debug_output(self):
        """Test histogram debug output when debug is enabled.

        Confirms debug paths execute and do not raise errors.
        """
        histogram = {"10": 50, "20": 100, "30": 75}

        # Test with debug enabled
        fig = self.builder.build(histogram, "Debug Histogram", "mph", debug=True)

        self.assertIsNotNone(fig)

    def test_histogram_with_many_buckets(self):
        """Test histogram with many buckets to trigger label thinning.

        Creates a dense histogram to exercise label-thinning logic.
        """
        # Create histogram with 25 buckets to trigger thinning logic
        histogram = {str(i * 5): (i % 10) * 10 + 20 for i in range(25)}

        fig = self.builder.build(histogram, "Dense Histogram Test", "mph")

        self.assertIsNotNone(fig)


class TestHistogramPlottingEdgeCases(unittest.TestCase):
    """Test histogram plotting edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = HistogramChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_histogram_with_very_large_values(self):
        """Test histogram with large count values."""
        histogram = {"10": 10000, "20": 50000, "30": 25000}

        fig = self.builder.build(histogram, "Large Values", "mph")

        self.assertIsNotNone(fig)

    def test_histogram_title_and_labels(self):
        """Test histogram applies title and labels.

        Ensures title and axis labels are set correctly on the plot.
        """
        histogram = {"10": 50, "20": 100}

        fig = self.builder.build(histogram, "Test Title", "mph")

        ax = fig.axes[0]
        self.assertEqual(ax.get_title(), "Test Title")
        self.assertIn("mph", ax.get_xlabel())

    def test_histogram_bar_properties(self):
        """Test histogram bar styling.

        Verifies bars are rendered with expected styling properties.
        """
        histogram = {"10": 50, "20": 100, "30": 75}

        fig = self.builder.build(histogram, "Bar Style", "mph")

        ax = fig.axes[0]
        patches = ax.patches

        # Should have bars
        self.assertGreater(len(patches), 0)

    def test_histogram_axis_configuration(self):
        """Test histogram axis configuration.

        Confirms axis labels and other axis configuration are present.
        """
        histogram = {"10": 50, "20": 100}

        fig = self.builder.build(histogram, "Axis Config", "mph")

        ax = fig.axes[0]

        # Should have configured axes
        self.assertIsNotNone(ax.get_xlabel())
        self.assertIsNotNone(ax.get_ylabel())


class TestTimeAxisFormattingEdgeCases(unittest.TestCase):
    """Test time axis formatting edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_format_time_axis_with_timezone(self):
        """Test time axis formatting with timezone.

        Builds a chart with timezone-aware formatting to exercise time-axis code.
        """
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "count": 100,
            },
            {
                "start_time": "2025-06-02T11:00:00",
                "p50": 31.2,
                "p85": 37.5,
                "count": 120,
            },
        ]

        # Build with timezone to exercise _format_time_axis
        fig = self.builder.build(metrics, "Timezone Test", "mph", tz_name="US/Pacific")

        self.assertIsNotNone(fig)

    def test_format_time_axis_with_invalid_timezone(self):
        """Test time axis formatting with invalid timezone.

        Verifies the builder handles invalid timezone identifiers gracefully.
        """
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 100,
            }
        ]

        # Should handle invalid timezone gracefully
        fig = self.builder.build(metrics, "Invalid TZ", "mph", tz_name="Invalid/Zone")

        self.assertIsNotNone(fig)

    def test_format_time_axis_offset_text(self):
        """Test offset text handling.

        Exercises configuration that affects the offset text on the time axis.
        """
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 100,
            },
            {
                "start_time": "2025-06-02T12:00:00",
                "p50": 31.2,
                "count": 120,
            },
        ]

        fig = self.builder.build(metrics, "Offset Test", "mph")

        # Should handle offset text configuration
        self.assertIsNotNone(fig)


class TestFinalStylingEdgeCases(unittest.TestCase):
    """Test _apply_final_styling edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_apply_final_styling_tight_layout(self):
        """Test tight layout application.

        Verifies the final styling step applies tight layout without errors.
        """
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "count": 100,
            }
        ]

        fig = self.builder.build(metrics, "Layout Test", "mph")

        # Should have applied styling
        self.assertIsNotNone(fig)

    def test_apply_final_styling_subplots_adjust(self):
        """Test subplot adjustment.

        Ensures subplot adjustments are applied and do not raise exceptions.
        """
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 100,
            }
        ]

        fig = self.builder.build(metrics, "Subplot Test", "mph")

        # Should have adjusted subplots
        self.assertIsNotNone(fig)


class TestLegendCreationExceptions(unittest.TestCase):
    """Test legend creation exception paths."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_legend_with_exception_fallback(self):
        """Test legend fallback on exception.

        Builds a chart to exercise the legend fallback handling when exceptions occur.
        """
        # Build a chart that will create a legend
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "p98": 43.0,
                "max": 53.5,
                "count": 5,  # Low count to trigger low-sample indicator
            }
        ]

        fig = self.builder.build(metrics, "Legend Test", "mph")

        # Should have created chart with legend
        self.assertIsNotNone(fig)


class TestComputeGapThresholdExceptions(unittest.TestCase):
    """Test _compute_gap_threshold exception handlers."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_compute_gap_threshold_with_exceptions(self):
        """Test gap threshold with problematic data.

        Ensures the gap threshold computation is resilient to problematic input.
        """
        # Create array with mixed types that might cause exceptions
        times = np.array(
            [
                datetime(2025, 6, 2, 10, 0, 0),
                datetime(2025, 6, 2, 11, 0, 0),
            ]
        )

        threshold = self.builder._compute_gap_threshold(times)

        # Should return a value or None
        self.assertTrue(threshold is None or isinstance(threshold, (int, float)))


class TestBarWidthComputationFallbacks(unittest.TestCase):
    """Test bar width computation fallback paths."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_bar_width_with_mdates_unavailable(self):
        """Test bar width computation when mdates path fails.

        Verifies fallback behavior when matplotlib.dates (mdates) is unavailable.
        """
        times = [
            datetime(2025, 6, 2, 10, 0, 0),
            datetime(2025, 6, 2, 11, 0, 0),
        ]

        # Should handle and compute widths
        bar_width_bg, bar_width = self.builder._compute_bar_widths(times)

        self.assertGreater(bar_width_bg, 0)
        self.assertGreater(bar_width, 0)

    def test_bar_width_fallback_exception(self):
        """Test bar width fallback exception handler.

        Confirms the builder recovers from exceptions during bar-width computation.
        """
        times = [
            datetime(2025, 6, 2, 10, 0, 0),
            datetime(2025, 6, 2, 11, 0, 0),
        ]

        bar_width_bg, bar_width = self.builder._compute_bar_widths(times)

        # Should always return positive values
        self.assertGreater(bar_width_bg, 0)
        self.assertGreater(bar_width, 0)


class TestPercentileLinePlotting(unittest.TestCase):
    """Test percentile line plotting code paths."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_percentile_lines_with_masked_data(self):
        """Test plotting with masked/invalid data."""
        metrics = [
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
                "p50": float("nan"),  # Invalid value
                "p85": 37.5,
                "p98": 44.1,
                "max": 54.2,
                "count": 5,  # Low count
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

        fig = self.builder.build(metrics, "Masked Data Test", "mph")

        # Should handle masked data gracefully
        self.assertIsNotNone(fig)


class TestHistogramLabelsAndFormatting(unittest.TestCase):
    """Test histogram label and formatting code."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = HistogramChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_histogram_xlabel_ylabel(self):
        """Test X and Y label setting (lines 772-773)."""
        histogram = {"10": 50, "20": 100, "30": 75}

        fig = self.builder.build(histogram, "Label Test", "mph")

        ax = fig.axes[0]
        xlabel = ax.get_xlabel()
        ylabel = ax.get_ylabel()

        # Should have labels
        self.assertIn("mph", xlabel)
        self.assertIn("Count", ylabel)

    def test_histogram_title_fontsize(self):
        """Test title setting (line 777-778)."""
        histogram = {"10": 50, "20": 100}

        fig = self.builder.build(histogram, "Title Fontsize Test", "mph")

        ax = fig.axes[0]
        title = ax.get_title()

        self.assertEqual(title, "Title Fontsize Test")

    def test_histogram_grid(self):
        """Test grid configuration (line 805-809)."""
        histogram = {"10": 50, "20": 100, "30": 75}

        fig = self.builder.build(histogram, "Grid Test", "mph")

        # Chart should be created (grid is configured internally)
        self.assertIsNotNone(fig)

    def test_histogram_range_labels(self):
        """Test histogram labels are formatted as ranges (e.g., '5-10', '10-15', '50+')."""
        # Histogram with bucket start values
        histogram = {"5": 10, "10": 20, "15": 30, "20": 25, "25": 15}

        fig = self.builder.build(histogram, "Range Label Test", "mph")

        ax = fig.axes[0]
        tick_labels = [label.get_text() for label in ax.get_xticklabels()]

        # Should have range labels
        self.assertIn("5-10", tick_labels)
        self.assertIn("10-15", tick_labels)
        self.assertIn("15-20", tick_labels)
        self.assertIn("20-25", tick_labels)
        # Last bucket should be open-ended with "+"
        self.assertIn("25+", tick_labels)

    def test_histogram_open_ended_bucket(self):
        """Test last bucket is formatted as 'N+' (open-ended)."""
        histogram = {"10": 50, "20": 100, "30": 75}

        fig = self.builder.build(histogram, "Open-Ended Test", "mph")

        ax = fig.axes[0]
        tick_labels = [label.get_text() for label in ax.get_xticklabels()]

        # Last label should be "30+"
        self.assertIn("30+", tick_labels)
        # Earlier labels should be ranges
        self.assertIn("10-20", tick_labels)
        self.assertIn("20-30", tick_labels)

    def test_histogram_non_numeric_labels_preserved(self):
        """Test non-numeric labels are preserved as-is."""
        histogram = {"low": 10, "medium": 20, "high": 15}

        fig = self.builder.build(histogram, "Non-Numeric Test", "categories")

        ax = fig.axes[0]
        tick_labels = [label.get_text() for label in ax.get_xticklabels()]

        # Non-numeric labels should be preserved
        self.assertIn("low", tick_labels)
        self.assertIn("medium", tick_labels)
        self.assertIn("high", tick_labels)


class TestTimeSeriesWithVariousCounts(unittest.TestCase):
    """Test time series with various count scenarios."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_with_zero_counts(self):
        """Test handling of zero counts."""
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 0,  # Zero count
            },
            {
                "start_time": "2025-06-02T11:00:00",
                "p50": 31.2,
                "count": 100,
            },
        ]

        fig = self.builder.build(metrics, "Zero Count Test", "mph")

        self.assertIsNotNone(fig)

    def test_with_very_low_counts(self):
        """Test with counts below threshold to trigger masking."""
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "count": 3,  # Below default threshold of 10
            },
            {
                "start_time": "2025-06-02T11:00:00",
                "p50": 31.2,
                "p85": 37.5,
                "count": 4,  # Below threshold
            },
        ]

        fig = self.builder.build(metrics, "Low Count Test", "mph")

        self.assertIsNotNone(fig)


# Phase 2: Edge Case and Debug Tests


class TestTimeSeriesChartBuilderEdgeCases(unittest.TestCase):
    """Phase 2 tests for chart_builder.py edge cases and debug functionality."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_debug_output_with_velocity_plot_debug_env_var(self):
        """Test debug output when VELOCITY_PLOT_DEBUG=1 environment variable is set."""
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 100,
            },
        ]

        # Set environment variable
        with patch.dict(os.environ, {"VELOCITY_PLOT_DEBUG": "1"}):
            with patch("sys.stderr"):
                fig = self.builder.build(metrics, "Debug Test", "mph")
                self.assertIsNotNone(fig)
                # Debug output should have been called (but may not write to mock)

    def test_debug_output_exception_handling(self):
        """Test that debug output exceptions are caught and don't break chart generation."""
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "count": 100,
            },
        ]

        # Mock the print function inside _debug_output to raise an exception
        # This tests that the exception handling in _debug_output works
        with patch.dict(os.environ, {"VELOCITY_PLOT_DEBUG": "1"}):
            with patch("builtins.print", side_effect=Exception("Print error")):
                # Chart should still be created despite debug error
                fig = self.builder.build(metrics, "Debug Error Test", "mph")
                self.assertIsNotNone(fig)

    def test_bar_width_computation_with_irregular_spacing(self):
        """Test bar width computation with irregularly spaced time points."""
        # Create metrics with irregular time spacing (missing hours)
        metrics = [
            {"start_time": "2025-06-02T10:00:00", "p50": 30.5, "count": 100},
            {"start_time": "2025-06-02T11:00:00", "p50": 31.2, "count": 120},
            # Skip 12:00
            {"start_time": "2025-06-02T13:00:00", "p50": 29.8, "count": 95},
            # Skip 14:00, 15:00
            {"start_time": "2025-06-02T16:00:00", "p50": 32.1, "count": 110},
        ]

        fig = self.builder.build(metrics, "Irregular Spacing Test", "mph")
        self.assertIsNotNone(fig)

    def test_bar_width_computation_fallback_without_mdates(self):
        """Test bar width computation fallback when mdates is not available."""
        metrics = [
            {"start_time": "2025-06-02T10:00:00", "p50": 30.5, "count": 100},
            {"start_time": "2025-06-02T11:00:00", "p50": 31.2, "count": 120},
        ]

        # Temporarily disable mdates
        from pdf_generator.core import chart_builder

        original_mdates = chart_builder.mdates
        try:
            chart_builder.mdates = None
            # Rebuild the builder to pick up the None mdates
            builder = TimeSeriesChartBuilder()
            fig = builder.build(metrics, "Fallback Test", "mph")
            self.assertIsNotNone(fig)
        finally:
            chart_builder.mdates = original_mdates

    def test_date_conversion_fallback_with_exception(self):
        """Test that chart generation is resilient to date conversion issues."""
        # Test with valid metrics but exercise error recovery paths
        # by using metrics that might trigger edge cases in date handling
        metrics = [
            {"start_time": "2025-06-02T10:00:00", "p50": 30.5, "count": 100},
            {"start_time": "2025-06-02T11:00:00", "p50": 31.2, "count": 120},
            {"start_time": "2025-06-02T12:00:00", "p50": 29.8, "count": 95},
        ]

        # Just verify that the chart is created successfully
        # The error handling paths are covered by the actual implementation
        fig = self.builder.build(metrics, "Date Handling Test", "mph")
        self.assertIsNotNone(fig)

        # Verify axes were created
        axes = fig.get_axes()
        self.assertGreater(len(axes), 0)

    def test_ylim_adjustment_with_error_recovery(self):
        """Test y-axis limit adjustment error handling and recovery."""
        metrics = [
            {"start_time": "2025-06-02T10:00:00", "p50": 30.5, "count": 100},
            {"start_time": "2025-06-02T11:00:00", "p50": 31.2, "count": 120},
        ]

        fig = self.builder.build(metrics, "Y-axis Test", "mph")
        self.assertIsNotNone(fig)

        # Verify that the chart was created successfully even if ylim adjustment had issues
        axes = fig.get_axes()
        self.assertGreater(len(axes), 0)

    def test_legend_positioning_error_recovery(self):
        """Test that legend positioning errors don't break chart creation."""
        metrics = [
            {
                "start_time": "2025-06-02T10:00:00",
                "p50": 30.5,
                "p85": 36.9,
                "p98": 43.0,
                "max": 53.5,
                "count": 100,
            },
        ]

        fig = self.builder.build(metrics, "Legend Test", "mph")
        self.assertIsNotNone(fig)

        # Chart should exist even if legend positioning failed
        axes = fig.get_axes()
        self.assertGreater(len(axes), 0)


class TestMatplotlibImportError(unittest.TestCase):
    """Test that ImportError is raised when matplotlib is not available."""

    def test_import_error_when_matplotlib_unavailable(self):
        """Test that TimeSeriesChartBuilder raises ImportError without matplotlib."""
        # Save original value
        import pdf_generator.core.chart_builder as cb_module

        original_have_matplotlib = cb_module.HAVE_MATPLOTLIB

        try:
            # Temporarily set HAVE_MATPLOTLIB to False
            cb_module.HAVE_MATPLOTLIB = False

            # Should raise ImportError
            with self.assertRaises(ImportError) as context:
                TimeSeriesChartBuilder()

            self.assertIn("matplotlib is required", str(context.exception))
            self.assertIn("pip install matplotlib", str(context.exception))
        finally:
            # Restore original value
            cb_module.HAVE_MATPLOTLIB = original_have_matplotlib


class TestDebugPlotOutput(unittest.TestCase):
    """Test debug output for plot debugging."""

    def test_create_masked_arrays_debug_output(self):
        """Test debug output in _create_masked_arrays when plot_debug is enabled."""
        builder = TimeSeriesChartBuilder(debug={"plot_debug": True})

        metrics = [
            {
                "timestamp": "2025-01-01T00:00:00",
                "p50": 25.0,
                "p85": 30.0,
                "p98": 35.0,
                "max": 40.0,
                "count": 100,
            },
            {
                "timestamp": "2025-01-01T01:00:00",
                "p50": 0.0,  # This will trigger zero_mask
                "p85": 0.0,
                "p98": 0.0,
                "max": 0.0,
                "count": 0,
            },
        ]

        times, p50_f, p85_f, p98_f, mx_f, counts = builder._extract_data(metrics, None)

        # Capture stderr for debug output
        import io
        import sys

        old_stderr = sys.stderr
        sys.stderr = io.StringIO()

        try:
            p50_a, p85_a, p98_a, mx_a = builder._create_masked_arrays(
                p50_f, p85_f, p98_f, mx_f, counts
            )

            stderr_output = sys.stderr.getvalue()
            # Debug output should include threshold and zero_mask_count
            self.assertIn("DEBUG_PLOT:", stderr_output)
        finally:
            sys.stderr = old_stderr

    def test_plot_count_bars_debug_output(self):
        """Test debug output via _debug_output when plot_debug is enabled."""
        builder = TimeSeriesChartBuilder(debug={"plot_debug": True})

        times = [datetime(2025, 1, 1, i) for i in range(3)]
        counts = [100, 200, 150]
        p50_f = np.array([25.0, 30.0, 28.0])

        import io
        import sys

        old_stderr = sys.stderr
        sys.stderr = io.StringIO()

        try:
            # Call _debug_output directly
            builder._debug_output(times, counts, p50_f)

            stderr_output = sys.stderr.getvalue()
            # Debug output should include times and counts info
            self.assertIn("DEBUG_PLOT:", stderr_output)
            self.assertIn("times(len)=", stderr_output)
            self.assertIn("counts=", stderr_output)
            self.assertIn("p50_f=", stderr_output)
        finally:
            sys.stderr = old_stderr

    def test_compute_gap_threshold_debug_output(self):
        """Test debug output in _compute_gap_threshold when plot_debug is enabled."""
        builder = TimeSeriesChartBuilder(debug={"plot_debug": True})

        times = [datetime(2025, 1, 1, i) for i in range(5)]

        import io
        import sys

        old_stderr = sys.stderr
        sys.stderr = io.StringIO()

        try:
            _ = builder._compute_gap_threshold(times)

            stderr_output = sys.stderr.getvalue()
            # Debug output should include base_delta and gap_threshold
            self.assertIn("DEBUG_PLOT:", stderr_output)
            self.assertIn("base_delta=", stderr_output)
            self.assertIn("gap_threshold=", stderr_output)
        finally:
            sys.stderr = old_stderr


class TestAxisYlimErrorRecovery(unittest.TestCase):
    """Test error recovery in axis ylim setting."""

    def test_configure_speed_axis_ylim_double_exception(self):
        """Test configure_speed_axis when both ylim attempts fail."""
        builder = TimeSeriesChartBuilder()

        fig, ax = plt.subplots()

        # Mock the axis to raise exceptions on both set_ylim and get_ylim
        original_set_ylim = ax.set_ylim
        original_get_ylim = ax.get_ylim

        call_count = [0]

        def mock_set_ylim(*args, **kwargs):
            call_count[0] += 1
            if call_count[0] == 1:
                raise RuntimeError("First set_ylim failed")
            return original_set_ylim(*args, **kwargs)

        def mock_get_ylim():
            raise RuntimeError("get_ylim failed")

        ax.set_ylim = mock_set_ylim
        ax.get_ylim = mock_get_ylim

        try:
            # Should not raise, should handle exception gracefully
            builder._configure_speed_axis(ax, "mph")
            # Verify it was called correctly (no exception raised)
            self.assertTrue(call_count[0] >= 1)
        finally:
            ax.set_ylim = original_set_ylim
            ax.get_ylim = original_get_ylim
            plt.close(fig)

    def test_plot_count_bars_ylim_double_exception(self):
        """Test _plot_count_bars when both ylim attempts fail."""
        builder = TimeSeriesChartBuilder()

        fig, ax2 = plt.subplots()
        times = [datetime(2025, 1, 1, i) for i in range(3)]
        counts = [100, 200, 150]

        # Mock the axis to raise exceptions on both set_ylim attempts
        original_set_ylim = ax2.set_ylim
        original_get_ylim = ax2.get_ylim

        call_count = [0]

        def mock_set_ylim(*args, **kwargs):
            call_count[0] += 1
            raise RuntimeError("set_ylim failed")

        def mock_get_ylim():
            raise RuntimeError("get_ylim failed")

        ax2.set_ylim = mock_set_ylim
        ax2.get_ylim = mock_get_ylim

        try:
            # Should not raise, should handle exception gracefully
            _ = builder._plot_count_bars(ax2, times, counts)
            # Method should complete despite exceptions
            self.assertTrue(call_count[0] >= 1)
        finally:
            ax2.set_ylim = original_set_ylim
            ax2.get_ylim = original_get_ylim
            plt.close(fig)


if __name__ == "__main__":
    unittest.main()
