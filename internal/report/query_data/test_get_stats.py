#!/usr/bin/env python3
"""Unit tests for refactored get_stats.py functions."""

import unittest
from unittest.mock import Mock, MagicMock, patch, call
from datetime import datetime, timezone
from zoneinfo import ZoneInfo
import argparse

from get_stats import (
    should_produce_daily,
    compute_iso_timestamps,
    resolve_file_prefix,
    fetch_granular_metrics,
    fetch_overall_summary,
    fetch_daily_summary,
    generate_histogram_chart,
    generate_timeseries_chart,
    assemble_pdf_report,
    process_date_range,
)


class TestShouldProduceDaily(unittest.TestCase):
    """Tests for should_produce_daily function."""

    def test_returns_false_for_24h_group(self):
        """Test that daily is not produced for 24h group."""
        self.assertFalse(should_produce_daily("24h"))

    def test_returns_false_for_1d_group(self):
        """Test that daily is not produced for 1d group."""
        self.assertFalse(should_produce_daily("1d"))

    def test_returns_true_for_1h_group(self):
        """Test that daily is produced for 1h group."""
        self.assertTrue(should_produce_daily("1h"))

    def test_returns_true_for_15m_group(self):
        """Test that daily is produced for 15m group."""
        self.assertTrue(should_produce_daily("15m"))


class TestComputeIsoTimestamps(unittest.TestCase):
    """Tests for compute_iso_timestamps function."""

    def test_compute_with_utc(self):
        """Test ISO timestamp generation with UTC (fallback to string)."""
        start_ts = 1704067200  # 2024-01-01 00:00:00 UTC
        end_ts = 1704153600  # 2024-01-02 00:00:00 UTC

        start_iso, end_iso = compute_iso_timestamps(start_ts, end_ts, None)

        # When timezone is None, function falls back to string representation
        self.assertEqual(start_iso, "1704067200")
        self.assertEqual(end_iso, "1704153600")

    def test_compute_with_timezone(self):
        """Test ISO timestamp generation with specific timezone."""
        start_ts = 1704067200  # 2024-01-01 00:00:00 UTC
        end_ts = 1704153600  # 2024-01-02 00:00:00 UTC

        start_iso, end_iso = compute_iso_timestamps(start_ts, end_ts, "US/Pacific")

        self.assertIsInstance(start_iso, str)
        self.assertIsInstance(end_iso, str)
        # Pacific time will be different from UTC
        self.assertIn("2023-12-31", start_iso)  # PST is UTC-8

    def test_handles_invalid_timezone_gracefully(self):
        """Test fallback when timezone is invalid."""
        start_ts = 1704067200
        end_ts = 1704153600

        start_iso, end_iso = compute_iso_timestamps(start_ts, end_ts, "Invalid/TZ")

        # Should fallback to string representation
        self.assertIsInstance(start_iso, str)
        self.assertIsInstance(end_iso, str)


class TestResolveFilePrefix(unittest.TestCase):
    """Tests for resolve_file_prefix function."""

    @patch("get_stats._next_sequenced_prefix")
    def test_with_user_provided_prefix(self, mock_next_seq):
        """Test prefix resolution with user-provided prefix."""
        mock_next_seq.return_value = "my-prefix-1"

        args = argparse.Namespace(
            file_prefix="my-prefix", timezone=None, source="radar_data_transits"
        )
        start_ts = 1704067200
        end_ts = 1704153600

        result = resolve_file_prefix(args, start_ts, end_ts)

        self.assertEqual(result, "my-prefix-1")
        mock_next_seq.assert_called_once_with("my-prefix")

    def test_auto_generated_prefix_utc(self):
        """Test auto-generated prefix with UTC."""
        args = argparse.Namespace(
            file_prefix="", timezone=None, source="radar_data_transits"
        )
        start_ts = 1704067200  # 2024-01-01 00:00:00 UTC
        end_ts = 1704153600  # 2024-01-02 00:00:00 UTC

        result = resolve_file_prefix(args, start_ts, end_ts)

        self.assertEqual(result, "radar_data_transits_2024-01-01_to_2024-01-02")

    def test_auto_generated_prefix_with_timezone(self):
        """Test auto-generated prefix with specific timezone."""
        args = argparse.Namespace(
            file_prefix="", timezone="US/Pacific", source="radar_objects"
        )
        start_ts = 1704067200  # 2024-01-01 00:00:00 UTC = 2023-12-31 16:00:00 PST
        end_ts = 1704153600  # 2024-01-02 00:00:00 UTC = 2024-01-01 16:00:00 PST

        result = resolve_file_prefix(args, start_ts, end_ts)

        self.assertEqual(result, "radar_objects_2023-12-31_to_2024-01-01")


class TestFetchGranularMetrics(unittest.TestCase):
    """Tests for fetch_granular_metrics function."""

    def test_successful_fetch(self):
        """Test successful granular metrics fetch."""
        mock_client = Mock()
        mock_metrics = [{"p50": 25.0}]
        mock_histogram = {"10": 5, "20": 10}
        mock_resp = Mock()
        mock_client.get_stats.return_value = (mock_metrics, mock_histogram, mock_resp)

        args = argparse.Namespace(
            group="1h",
            units="mph",
            source="radar_data_transits",
            timezone="US/Pacific",
            min_speed=5.0,
            histogram=True,
            hist_bucket_size=5.0,
            hist_max=50.0,
        )

        metrics, histogram, resp = fetch_granular_metrics(
            mock_client, 1704067200, 1704153600, args, "rebuild-full"
        )

        self.assertEqual(metrics, mock_metrics)
        self.assertEqual(histogram, mock_histogram)
        self.assertEqual(resp, mock_resp)
        mock_client.get_stats.assert_called_once()

    def test_fetch_failure_returns_empty(self):
        """Test that fetch failure returns empty results."""
        mock_client = Mock()
        mock_client.get_stats.side_effect = Exception("API Error")

        args = argparse.Namespace(
            group="1h",
            units="mph",
            source="radar_data_transits",
            timezone=None,
            min_speed=None,
            histogram=False,
            hist_bucket_size=None,
            hist_max=None,
        )

        metrics, histogram, resp = fetch_granular_metrics(
            mock_client, 1704067200, 1704153600, args, None
        )

        self.assertEqual(metrics, [])
        self.assertIsNone(histogram)
        self.assertIsNone(resp)


class TestFetchOverallSummary(unittest.TestCase):
    """Tests for fetch_overall_summary function."""

    def test_successful_fetch(self):
        """Test successful overall summary fetch."""
        mock_client = Mock()
        mock_metrics = [{"p50": 25.0, "count": 1000}]
        mock_client.get_stats.return_value = (mock_metrics, None, Mock())

        args = argparse.Namespace(
            units="mph", source="radar_data_transits", timezone=None, min_speed=None
        )

        result = fetch_overall_summary(mock_client, 1704067200, 1704153600, args, None)

        self.assertEqual(result, mock_metrics)

    def test_fetch_failure_returns_empty_list(self):
        """Test that fetch failure returns empty list."""
        mock_client = Mock()
        mock_client.get_stats.side_effect = Exception("API Error")

        args = argparse.Namespace(
            units="mph", source="radar_data_transits", timezone=None, min_speed=None
        )

        result = fetch_overall_summary(mock_client, 1704067200, 1704153600, args, None)

        self.assertEqual(result, [])


class TestFetchDailySummary(unittest.TestCase):
    """Tests for fetch_daily_summary function."""

    def test_fetch_when_needed(self):
        """Test daily fetch when group is less than 24h."""
        mock_client = Mock()
        mock_metrics = [{"p50": 24.0}]
        mock_client.get_stats.return_value = (mock_metrics, None, Mock())

        args = argparse.Namespace(
            group="1h",
            units="mph",
            source="radar_data_transits",
            timezone=None,
            min_speed=None,
        )

        result = fetch_daily_summary(mock_client, 1704067200, 1704153600, args, None)

        self.assertEqual(result, mock_metrics)

    def test_not_fetched_when_group_is_24h(self):
        """Test daily not fetched when group is already 24h."""
        mock_client = Mock()

        args = argparse.Namespace(
            group="24h",
            units="mph",
            source="radar_data_transits",
            timezone=None,
            min_speed=None,
        )

        result = fetch_daily_summary(mock_client, 1704067200, 1704153600, args, None)

        self.assertIsNone(result)
        mock_client.get_stats.assert_not_called()

    def test_fetch_failure_returns_none(self):
        """Test that fetch failure returns None."""
        mock_client = Mock()
        mock_client.get_stats.side_effect = Exception("API Error")

        args = argparse.Namespace(
            group="1h",
            units="mph",
            source="radar_data_transits",
            timezone=None,
            min_speed=None,
        )

        result = fetch_daily_summary(mock_client, 1704067200, 1704153600, args, None)

        self.assertIsNone(result)


class TestGenerateHistogramChart(unittest.TestCase):
    """Tests for generate_histogram_chart function."""

    @patch("get_stats.save_chart_as_pdf")
    @patch("get_stats.plot_histogram")
    def test_successful_generation(self, mock_plot, mock_save):
        """Test successful histogram chart generation."""
        mock_fig = Mock()
        mock_plot.return_value = mock_fig
        mock_save.return_value = True

        histogram = {"10": 5, "20": 10}
        metrics_all = [{"count": 1000}]
        args = argparse.Namespace(debug=False, units="mph")

        result = generate_histogram_chart(
            histogram, "test-prefix", "mph", metrics_all, args
        )

        self.assertTrue(result)
        mock_plot.assert_called_once()
        mock_save.assert_called_once_with(mock_fig, "test-prefix_histogram.pdf")

    @patch("get_stats.save_chart_as_pdf")
    @patch("get_stats.plot_histogram")
    def test_save_failure_returns_false(self, mock_plot, mock_save):
        """Test that save failure returns False."""
        mock_fig = Mock()
        mock_plot.return_value = mock_fig
        mock_save.return_value = False

        histogram = {"10": 5}
        args = argparse.Namespace(debug=False, units="mph")

        result = generate_histogram_chart(histogram, "test-prefix", "mph", [], args)

        self.assertFalse(result)

    @patch("get_stats.plot_histogram")
    def test_exception_returns_false(self, mock_plot):
        """Test that exception returns False."""
        mock_plot.side_effect = Exception("Plot error")

        histogram = {"10": 5}
        args = argparse.Namespace(debug=False, units="mph")

        result = generate_histogram_chart(histogram, "test-prefix", "mph", [], args)

        self.assertFalse(result)


class TestGenerateTimeseriesChart(unittest.TestCase):
    """Tests for generate_timeseries_chart function."""

    @patch("get_stats.save_chart_as_pdf")
    @patch("get_stats._plot_stats_page")
    def test_successful_generation(self, mock_plot, mock_save):
        """Test successful time-series chart generation."""
        mock_fig = Mock()
        mock_plot.return_value = mock_fig
        mock_save.return_value = True

        metrics = [{"p50": 25.0}]
        args = argparse.Namespace(debug=False)

        result = generate_timeseries_chart(
            metrics, "test_stats", "Test Chart", "mph", "US/Pacific", args
        )

        self.assertTrue(result)
        mock_plot.assert_called_once_with(
            metrics, "Test Chart", "mph", tz_name="US/Pacific"
        )
        mock_save.assert_called_once_with(mock_fig, "test_stats.pdf")

    @patch("get_stats._plot_stats_page")
    def test_exception_returns_false(self, mock_plot):
        """Test that exception returns False."""
        mock_plot.side_effect = Exception("Plot error")

        metrics = [{"p50": 25.0}]
        args = argparse.Namespace(debug=False)

        result = generate_timeseries_chart(
            metrics, "test_stats", "Test", "mph", None, args
        )

        self.assertFalse(result)


class TestAssemblePdfReport(unittest.TestCase):
    """Tests for assemble_pdf_report function."""

    @patch("get_stats.generate_pdf_report")
    def test_successful_assembly(self, mock_generate):
        """Test successful PDF assembly."""
        args = argparse.Namespace(
            group="1h", units="mph", timezone="US/Pacific", min_speed=5.0, hist_max=50.0
        )

        result = assemble_pdf_report(
            "test-prefix",
            "2024-01-01T00:00:00Z",
            "2024-01-02T00:00:00Z",
            [{"p50": 25.0}],
            None,
            [{"p50": 24.0}],
            {"10": 5},
            args,
        )

        self.assertTrue(result)
        mock_generate.assert_called_once()

    @patch("get_stats.generate_pdf_report")
    def test_exception_returns_false(self, mock_generate):
        """Test that exception returns False."""
        mock_generate.side_effect = Exception("PDF error")

        args = argparse.Namespace(
            group="1h", units="mph", timezone=None, min_speed=None, hist_max=None
        )

        result = assemble_pdf_report(
            "test-prefix",
            "2024-01-01T00:00:00Z",
            "2024-01-02T00:00:00Z",
            [],
            None,
            [],
            None,
            args,
        )

        self.assertFalse(result)


class TestProcessDateRange(unittest.TestCase):
    """Tests for process_date_range orchestration."""

    @patch("get_stats.assemble_pdf_report")
    @patch("get_stats.fetch_daily_summary")
    @patch("get_stats.fetch_overall_summary")
    @patch("get_stats.fetch_granular_metrics")
    @patch("get_stats.parse_date_to_unix")
    def test_successful_processing(
        self,
        mock_parse,
        mock_fetch_granular,
        mock_fetch_overall,
        mock_fetch_daily,
        mock_assemble,
    ):
        """Test successful date range processing."""
        mock_parse.side_effect = [1704067200, 1704153600]
        mock_fetch_granular.return_value = ([{"p50": 25.0}], None, Mock())
        mock_fetch_overall.return_value = [{"p50": 25.0}]
        mock_fetch_daily.return_value = None
        mock_assemble.return_value = True

        mock_client = Mock()
        args = argparse.Namespace(
            source="radar_data_transits",
            model_version="rebuild-full",
            timezone=None,
            file_prefix="",
            group="1h",
            units="mph",
            min_speed=None,
            debug=False,
        )

        # Should not raise
        process_date_range("2024-01-01", "2024-01-02", args, mock_client)

        mock_fetch_granular.assert_called_once()
        mock_fetch_overall.assert_called_once()
        mock_assemble.assert_called_once()

    @patch("get_stats.parse_date_to_unix")
    def test_invalid_date_returns_early(self, mock_parse):
        """Test that invalid date returns early."""
        mock_parse.side_effect = ValueError("Invalid date")

        mock_client = Mock()
        args = argparse.Namespace(source="radar_data_transits", timezone=None)

        # Should not raise, just print error and return
        process_date_range("invalid", "date", args, mock_client)

        # Client should not be called
        mock_client.get_stats.assert_not_called()

    @patch("get_stats.fetch_granular_metrics")
    @patch("get_stats.parse_date_to_unix")
    def test_no_data_returns_early(self, mock_parse, mock_fetch):
        """Test that no data returns early."""
        mock_parse.side_effect = [1704067200, 1704153600]
        mock_fetch.return_value = ([], None, None)

        mock_client = Mock()
        args = argparse.Namespace(
            source="radar_data_transits",
            model_version=None,
            timezone=None,
            file_prefix="",
            group="1h",
            units="mph",
            min_speed=None,
        )

        # Should return early without assembling PDF
        process_date_range("2024-01-01", "2024-01-02", args, mock_client)


if __name__ == "__main__":
    unittest.main()
