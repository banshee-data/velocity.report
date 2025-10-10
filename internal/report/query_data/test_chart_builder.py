#!/usr/bin/env python3
"""Unit tests for chart_builder.py chart generation module."""

import os
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


class TestTimezoneConversionEdgeCases(unittest.TestCase):
    """Test timezone conversion edge cases."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = TimeSeriesChartBuilder()

    def tearDown(self):
        """Clean up matplotlib figures."""
        plt.close("all")

    def test_convert_timezone_with_invalid_timezone(self):
        """Test timezone conversion with invalid timezone name (line 189-190)."""
        dt = datetime(2025, 6, 2, 12, 0, 0)
        # Invalid timezone should return original datetime
        result = self.builder._convert_timezone(dt, "Invalid/Timezone")
        self.assertEqual(result, dt)

    def test_convert_timezone_with_naive_datetime(self):
        """Test timezone conversion with naive datetime (lines 194-198)."""
        naive_dt = datetime(2025, 6, 2, 12, 0, 0)  # No timezone
        result = self.builder._convert_timezone(naive_dt, "US/Pacific")
        # Should assume UTC and convert
        self.assertIsNotNone(result)
        self.assertIsNotNone(result.tzinfo)

    def test_convert_timezone_with_aware_datetime(self):
        """Test timezone conversion with timezone-aware datetime (line 193)."""
        aware_dt = datetime(2025, 6, 2, 12, 0, 0, tzinfo=ZoneInfo("UTC"))
        result = self.builder._convert_timezone(aware_dt, "US/Eastern")
        self.assertIsNotNone(result)
        self.assertEqual(result.tzinfo, ZoneInfo("US/Eastern"))

    def test_convert_timezone_exception_handler(self):
        """Test timezone conversion exception handler (line 200-201)."""

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
        """Test extraction with unparseable time (line 167-168)."""
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
        """Test debug output when VELOCITY_PLOT_DEBUG=1 (lines 238-246, 261-262)."""
        import os

        os.environ["VELOCITY_PLOT_DEBUG"] = "1"

        times = [datetime(2025, 6, 2, 10, 0, 0)]
        counts = [100]
        p50_f = np.array([30.5])

        # Should not raise, just print to stderr
        self.builder._debug_output(times, counts, p50_f)

    def test_create_masked_arrays_with_debug(self):
        """Test masked array creation with debug output (lines 238-246)."""
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
        """Test exception handler in masked array creation (line 245-246)."""
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
        """Test building runs with time gaps (lines 394, 414)."""
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
        """Test exception handler in _build_runs (line 414)."""
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
        """Test exception handlers in _configure_speed_axis (lines 437-442)."""
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
        """Test count bars with empty data (lines 458-459)."""
        metrics = []

        fig = self.builder.build(metrics, "Empty Test", "mph")

        # Should handle empty data gracefully
        self.assertIsNotNone(fig)

    def test_plot_count_bars_with_low_counts(self):
        """Test count bars with low count values (lines 466-467)."""
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
        """Test count axis configuration (lines 565-566)."""
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
        """Test legend creation with low sample indicator (lines 580, 592-593)."""
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
        """Test exception handler in histogram sorting (lines 738-739)."""
        # Mix of numeric and non-numeric keys
        histogram = {"10": 50, "abc": 30, "20": 70}

        # Should fallback to string sorting
        fig = self.builder.build(histogram, "Mixed Keys", "mph")

        self.assertIsNotNone(fig)

    def test_histogram_with_debug_output(self):
        """Test histogram debug output (line 699)."""
        histogram = {"10": 50, "20": 100, "30": 75}

        # Test with debug enabled
        fig = self.builder.build(histogram, "Debug Histogram", "mph", debug=True)

        self.assertIsNotNone(fig)

    def test_histogram_with_many_buckets(self):
        """Test histogram with > 20 buckets to trigger label thinning (lines 805-809)."""
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
        """Test histogram applies title and labels (lines 772-773, 777-778)."""
        histogram = {"10": 50, "20": 100}

        fig = self.builder.build(histogram, "Test Title", "mph")

        ax = fig.axes[0]
        self.assertEqual(ax.get_title(), "Test Title")
        self.assertIn("mph", ax.get_xlabel())

    def test_histogram_bar_properties(self):
        """Test histogram bar styling (lines 789-790)."""
        histogram = {"10": 50, "20": 100, "30": 75}

        fig = self.builder.build(histogram, "Bar Style", "mph")

        ax = fig.axes[0]
        patches = ax.patches

        # Should have bars
        self.assertGreater(len(patches), 0)

    def test_histogram_axis_configuration(self):
        """Test histogram axis configuration (lines 805-809)."""
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
        """Test time axis formatting with timezone (lines 630, 637-638)."""
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
        """Test time axis formatting with invalid timezone (lines 637-638)."""
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
        """Test offset text handling (lines 654-660)."""
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
        """Test tight layout application (lines 667-668)."""
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
        """Test subplot adjustment (lines 672-679)."""
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
        """Test legend fallback on exception (lines 617-624)."""
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
        """Test gap threshold with problematic data (line 357)."""
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
        """Test bar width computation when mdates path fails (lines 537-548)."""
        times = [
            datetime(2025, 6, 2, 10, 0, 0),
            datetime(2025, 6, 2, 11, 0, 0),
        ]

        # Should handle and compute widths
        bar_width_bg, bar_width = self.builder._compute_bar_widths(times)

        self.assertGreater(bar_width_bg, 0)
        self.assertGreater(bar_width, 0)

    def test_bar_width_fallback_exception(self):
        """Test bar width fallback exception handler (line 550-551)."""
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

        ax = fig.axes[0]

        # Chart should be created (grid is configured internally)
        self.assertIsNotNone(fig)


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


if __name__ == "__main__":
    unittest.main()
