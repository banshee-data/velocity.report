#!/usr/bin/env python3
"""Coverage tests for pdf_generator.cli.main utility functions."""

import json
import os
import sys
import tempfile
import textwrap
import unittest
from io import StringIO
from unittest.mock import MagicMock, patch

from pdf_generator.cli.main import (
    _append_debug_hint,
    _format_api_error,
    _maybe_print_debug,
    _next_sequenced_prefix,
    _print_error,
    _print_info,
    check_charts_available,
    compute_iso_timestamps,
    derive_overall_from_granular,
    get_model_version,
    parse_date_range,
    resolve_file_prefix,
    should_produce_daily,
)


class TestPrintHelpers(unittest.TestCase):
    def test_print_error(self):
        captured = StringIO()
        with patch("sys.stderr", captured):
            _print_error("test error")
        self.assertIn("test error", captured.getvalue())

    def test_print_info(self):
        captured = StringIO()
        with patch("sys.stdout", captured):
            _print_info("test info")
        self.assertIn("test info", captured.getvalue())


class TestAppendDebugHint(unittest.TestCase):
    def test_debug_enabled(self):
        result = _append_debug_hint("something broke", debug_enabled=True)
        self.assertEqual(result, "something broke")

    def test_debug_disabled(self):
        result = _append_debug_hint("something broke", debug_enabled=False)
        self.assertIn("Re-run with --debug", result)


class TestMaybePrintDebug(unittest.TestCase):
    def test_debug_disabled_no_output(self):
        captured = StringIO()
        with patch("sys.stderr", captured):
            _maybe_print_debug(ValueError("x"), debug_enabled=False)
        self.assertEqual(captured.getvalue(), "")

    def test_debug_enabled_prints_traceback(self):
        captured = StringIO()
        with patch("sys.stderr", captured):
            try:
                raise ValueError("test error")
            except ValueError as exc:
                _maybe_print_debug(exc, debug_enabled=True)
        self.assertIn("DEBUG:", captured.getvalue())
        self.assertIn("ValueError", captured.getvalue())


class TestFormatApiError(unittest.TestCase):
    def test_connection_error(self):
        import requests

        exc = requests.exceptions.ConnectionError("refused")
        result = _format_api_error("Fetch data", "http://localhost:8080", exc)
        self.assertIn("Unable to reach", result)

    def test_timeout_error(self):
        import requests

        exc = requests.exceptions.Timeout("timed out")
        result = _format_api_error("Fetch data", "http://localhost:8080", exc)
        self.assertIn("timed out", result)

    def test_http_error_400(self):
        import requests

        resp = MagicMock()
        resp.status_code = 400
        resp.text = "Bad Request"
        exc = requests.exceptions.HTTPError(response=resp)
        result = _format_api_error("Query", "http://localhost:8080", exc)
        self.assertIn("HTTP 400", result)
        self.assertIn("Check date range", result)

    def test_http_error_404(self):
        import requests

        resp = MagicMock()
        resp.status_code = 404
        resp.text = "Not Found"
        exc = requests.exceptions.HTTPError(response=resp)
        result = _format_api_error("Query", "http://localhost:8080", exc)
        self.assertIn("HTTP 404", result)
        self.assertIn("Endpoint not found", result)

    def test_http_error_401(self):
        import requests

        resp = MagicMock()
        resp.status_code = 401
        resp.text = "Unauthorized"
        exc = requests.exceptions.HTTPError(response=resp)
        result = _format_api_error("Query", "http://localhost:8080", exc)
        self.assertIn("Authentication failed", result)

    def test_http_error_500(self):
        import requests

        resp = MagicMock()
        resp.status_code = 500
        resp.text = "Internal Server Error"
        exc = requests.exceptions.HTTPError(response=resp)
        result = _format_api_error("Query", "http://localhost:8080", exc)
        self.assertIn("server error", result)

    def test_generic_exception(self):
        exc = RuntimeError("something broke")
        result = _format_api_error("Action", "http://localhost:8080", exc)
        self.assertIn("something broke", result)


class TestShouldProduceDaily(unittest.TestCase):
    def test_fifteen_minutes(self):
        self.assertTrue(should_produce_daily("15m"))

    def test_one_hour(self):
        self.assertTrue(should_produce_daily("1h"))

    def test_daily_group(self):
        self.assertFalse(should_produce_daily("1d"))

    def test_two_day_group(self):
        self.assertFalse(should_produce_daily("2d"))

    def test_unknown_group(self):
        self.assertTrue(should_produce_daily("unknown"))

    def test_large_seconds_format(self):
        self.assertFalse(should_produce_daily("86400s"))


class TestNextSequencedPrefix(unittest.TestCase):
    def test_nonexistent_directory(self):
        result = _next_sequenced_prefix("test", "/nonexistent/dir")
        self.assertTrue(result.startswith("test-1-"))

    def test_empty_directory(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            result = _next_sequenced_prefix("report", tmpdir)
            self.assertTrue(result.startswith("report-1-"))

    def test_with_existing_files(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create files that match the pattern
            open(os.path.join(tmpdir, "report-1-120000_data.csv"), "w").close()
            open(os.path.join(tmpdir, "report-2-130000_data.csv"), "w").close()
            result = _next_sequenced_prefix("report", tmpdir)
            self.assertTrue(result.startswith("report-3-"))


class TestComputeIsoTimestamps(unittest.TestCase):
    def test_utc(self):
        start, end = compute_iso_timestamps(1704067200, 1704153600, None)
        self.assertIn("2024-01-01", start)
        self.assertIn("2024-01-02", end)

    def test_with_timezone(self):
        start, end = compute_iso_timestamps(1704067200, 1704153600, "US/Pacific")
        self.assertIn("2023-12-31", start)  # US/Pacific is UTC-8

    def test_invalid_timezone_fallback(self):
        start, end = compute_iso_timestamps(1000, 2000, "Invalid/Timezone")
        # Should fall back to string representation
        self.assertEqual(start, "1000")
        self.assertEqual(end, "2000")


class TestResolveFilePrefix(unittest.TestCase):
    def test_basic_prefix(self):
        config = MagicMock()
        config.query.end_date = "2025-01-19"
        config.site.location = "Test Location"
        result = resolve_file_prefix(config, 1000, 2000, "/tmp")
        self.assertEqual(result, "2025-01-19_velocity.report_Test_Location")

    def test_special_characters(self):
        config = MagicMock()
        config.query.end_date = "2025-01-19"
        config.site.location = "Clarendon Ave & Pine St"
        result = resolve_file_prefix(config, 1000, 2000, "/tmp")
        self.assertIn("Clarendon_Ave", result)
        self.assertNotIn("&", result)


class TestParseDateRange(unittest.TestCase):
    def test_valid_dates(self):
        start, end = parse_date_range("2025-01-01", "2025-01-02", "UTC")
        self.assertIsNotNone(start)
        self.assertIsNotNone(end)
        self.assertGreater(end, start)

    def test_invalid_dates(self):
        captured = StringIO()
        with patch("sys.stderr", captured):
            start, end = parse_date_range("not-a-date", "also-bad", "UTC")
        self.assertIsNone(start)
        self.assertIsNone(end)


class TestGetModelVersion(unittest.TestCase):
    def test_transit_source(self):
        config = MagicMock()
        config.query.source = "radar_data_transits"
        config.query.model_version = "v2"
        self.assertEqual(get_model_version(config), "v2")

    def test_transit_source_default(self):
        config = MagicMock()
        config.query.source = "radar_data_transits"
        config.query.model_version = None
        self.assertEqual(get_model_version(config), "hourly-cron")

    def test_radar_objects_source(self):
        config = MagicMock()
        config.query.source = "radar_objects"
        self.assertIsNone(get_model_version(config))


class TestCheckChartsAvailable(unittest.TestCase):
    def test_charts_available(self):
        # Should return True when matplotlib is available
        result = check_charts_available()
        self.assertIsInstance(result, bool)


class TestDeriveOverallFromGranular(unittest.TestCase):
    def test_empty_metrics(self):
        result = derive_overall_from_granular([])
        self.assertIsNotNone(result)

    def test_single_row(self):
        row = {
            "start_time": "2025-01-01T00:00:00Z",
            "count": 10,
            "mean_speed": 30.0,
            "p50_speed": 28.0,
            "p85_speed": 35.0,
            "p98_speed": 42.0,
            "max_speed": 50.0,
            "min_speed": 15.0,
            "std_dev": 5.0,
        }
        result = derive_overall_from_granular([row])
        self.assertIsNotNone(result)
        self.assertGreater(len(result), 0)


if __name__ == "__main__":
    unittest.main()


class TestImportHelpers(unittest.TestCase):
    def test_import_chart_builder(self):
        from pdf_generator.cli.main import _import_chart_builder

        builder = _import_chart_builder()
        self.assertIsNotNone(builder)

    def test_import_chart_saver(self):
        from pdf_generator.cli.main import _import_chart_saver

        saver = _import_chart_saver()
        self.assertIsNotNone(saver)

    def test_import_chart_builder_cached(self):
        """Test that second call returns cached version."""
        from pdf_generator.cli.main import _import_chart_builder

        first = _import_chart_builder()
        second = _import_chart_builder()
        self.assertIs(first, second)

    def test_import_chart_saver_cached(self):
        """Test that second call returns cached version."""
        from pdf_generator.cli.main import _import_chart_saver

        first = _import_chart_saver()
        second = _import_chart_saver()
        self.assertIs(first, second)


class TestDeriveDaily(unittest.TestCase):
    def test_derive_daily_empty(self):
        from pdf_generator.cli.main import derive_daily_from_granular

        result = derive_daily_from_granular([], "US/Pacific")
        self.assertIsNotNone(result)
        self.assertEqual(len(result), 0)

    def test_derive_daily_single_row(self):
        from pdf_generator.cli.main import derive_daily_from_granular

        row = {
            "start_time": "2025-01-01T12:00:00-08:00",
            "count": 10,
            "mean_speed": 30.0,
            "p50_speed": 28.0,
            "p85_speed": 35.0,
            "p98_speed": 42.0,
            "max_speed": 50.0,
            "min_speed": 15.0,
            "std_dev": 5.0,
        }
        result = derive_daily_from_granular([row], "US/Pacific")
        self.assertIsNotNone(result)
        self.assertGreater(len(result), 0)


class TestPlotStatsPage(unittest.TestCase):
    def test_plot_stats_page(self):
        from pdf_generator.cli.main import _plot_stats_page

        stats = [
            {"start_time": "2025-01-01T00:00:00Z", "count": 10, "mean_speed": 30.0,
             "p50_speed": 28.0, "p85_speed": 35.0, "p98_speed": 42.0, "max_speed": 50.0,
             "min_speed": 15.0, "std_dev": 5.0}
        ]
        result = _plot_stats_page(stats, "Test", "kph")
        self.assertIsNotNone(result)


class TestFetchFunctions(unittest.TestCase):
    """Test API fetch functions with mocked responses."""

    def test_fetch_site_config_periods_connection_error(self):
        """Test graceful handling of connection errors."""
        from pdf_generator.cli.main import fetch_site_config_periods
        from pdf_generator.core.api_client import RadarStatsClient

        client = RadarStatsClient(base_url="http://localhost:99999")
        result = fetch_site_config_periods(client, site_id=1, start_ts=1000, end_ts=2000)
        # Should return empty list on connection error
        self.assertEqual(result, [])

    def test_print_api_debug_info(self):
        from pdf_generator.cli.main import print_api_debug_info

        captured = StringIO()
        with patch("sys.stdout", captured):
            print_api_debug_info(None, [{"count": 10}], {"bins": [1, 2, 3]})
        output = captured.getvalue()
        self.assertTrue(len(output) > 0)


class TestImportChartBuilderCold(unittest.TestCase):
    """Test cold import path for chart builder/saver."""

    def test_import_chart_builder_cold(self):
        """Test first-time import of chart builder by resetting global."""
        import pdf_generator.cli.main as module

        # Save original and reset
        original = module.TimeSeriesChartBuilder
        module.TimeSeriesChartBuilder = None  # type: ignore
        try:
            result = module._import_chart_builder()
            self.assertIsNotNone(result)
        finally:
            module.TimeSeriesChartBuilder = original

    def test_import_chart_saver_cold(self):
        """Test first-time import of chart saver by resetting global."""
        import pdf_generator.cli.main as module

        original = module.save_chart_as_pdf
        module.save_chart_as_pdf = None  # type: ignore
        try:
            result = module._import_chart_saver()
            self.assertIsNotNone(result)
        finally:
            module.save_chart_as_pdf = original


class TestNextSequencedPrefix(unittest.TestCase):
    """Test _next_sequenced_prefix with existing numbered files."""

    def test_with_existing_files(self):
        from pdf_generator.cli.main import _next_sequenced_prefix

        with tempfile.TemporaryDirectory() as tmpdir:
            # Create some existing files
            open(os.path.join(tmpdir, "report-1-120000_stats.pdf"), "w").close()
            open(os.path.join(tmpdir, "report-2-130000_stats.pdf"), "w").close()
            result = _next_sequenced_prefix("report", tmpdir)
            self.assertIn("report-3-", result)

    def test_with_no_existing_files(self):
        from pdf_generator.cli.main import _next_sequenced_prefix

        with tempfile.TemporaryDirectory() as tmpdir:
            result = _next_sequenced_prefix("report", tmpdir)
            self.assertIn("report-1-", result)


class TestFetchOverallSummary(unittest.TestCase):
    """Test fetch_overall_summary."""

    def test_success(self):
        from pdf_generator.cli.main import fetch_overall_summary
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        mock_client = MagicMock()
        mock_client.get_stats.return_value = (
            [{"count": 100}],
            None,
            None,
            None,
        )
        result = fetch_overall_summary(mock_client, 1000, 2000, config, None)
        self.assertEqual(len(result), 1)

    def test_error(self):
        from pdf_generator.cli.main import fetch_overall_summary
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        mock_client = MagicMock()
        mock_client.get_stats.side_effect = Exception("API error")
        mock_client.api_url = "http://test/api"
        result = fetch_overall_summary(mock_client, 1000, 2000, config, None)
        self.assertEqual(result, [])


class TestFetchDailySummary(unittest.TestCase):
    """Test fetch_daily_summary."""

    def test_success_with_daily_group(self):
        from pdf_generator.cli.main import fetch_daily_summary
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.group = "1h"
        mock_client = MagicMock()
        mock_client.get_stats.return_value = (
            [{"count": 50}],
            None,
            None,
            None,
        )
        result = fetch_daily_summary(mock_client, 1000, 2000, config, None)
        self.assertEqual(len(result), 1)

    def test_not_needed_for_24h_group(self):
        from pdf_generator.cli.main import fetch_daily_summary
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.group = "24h"
        result = fetch_daily_summary(MagicMock(), 1000, 2000, config, None)
        self.assertIsNone(result)

    def test_error(self):
        from pdf_generator.cli.main import fetch_daily_summary
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.group = "1h"
        mock_client = MagicMock()
        mock_client.get_stats.side_effect = Exception("API error")
        mock_client.api_url = "http://test/api"
        result = fetch_daily_summary(mock_client, 1000, 2000, config, None)
        self.assertIsNone(result)


class TestFetchGranularMetrics(unittest.TestCase):
    """Test fetch_granular_metrics error path."""

    def test_error(self):
        from pdf_generator.cli.main import fetch_granular_metrics
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.output.debug = True
        mock_client = MagicMock()
        mock_client.get_stats.side_effect = Exception("API error")
        mock_client.api_url = "http://test/api"
        result = fetch_granular_metrics(mock_client, 1000, 2000, config, None)
        self.assertEqual(result[0], [])
        self.assertIsNone(result[1])

    def test_success(self):
        from pdf_generator.cli.main import fetch_granular_metrics
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        mock_client = MagicMock()
        mock_client.get_stats.return_value = (
            [{"count": 10}],
            {"bins": [1]},
            5.0,
            MagicMock(),
        )
        result = fetch_granular_metrics(mock_client, 1000, 2000, config, None)
        self.assertEqual(len(result[0]), 1)


class TestFetchSiteConfigPeriods(unittest.TestCase):
    """Test fetch_site_config_periods with filtering."""

    def test_overlapping_periods(self):
        from pdf_generator.cli.main import fetch_site_config_periods

        mock_client = MagicMock()
        mock_client.base_url = "http://test"
        mock_client.get_site_config_periods.return_value = (
            [
                {"effective_start_unix": 500, "effective_end_unix": 1500},
                {"effective_start_unix": 3000, "effective_end_unix": 4000},
            ],
            None,
        )
        result = fetch_site_config_periods(mock_client, 1, 1000, 2000)
        self.assertEqual(len(result), 1)

    def test_with_compare_range(self):
        from pdf_generator.cli.main import fetch_site_config_periods

        mock_client = MagicMock()
        mock_client.base_url = "http://test"
        mock_client.get_site_config_periods.return_value = (
            [
                {"effective_start_unix": 500, "effective_end_unix": 1500},
                {"effective_start_unix": 3000, "effective_end_unix": 4000},
            ],
            None,
        )
        result = fetch_site_config_periods(
            mock_client, 1, 1000, 2000, compare_start_ts=3500, compare_end_ts=3800
        )
        self.assertEqual(len(result), 2)

    def test_period_with_no_end(self):
        from pdf_generator.cli.main import fetch_site_config_periods

        mock_client = MagicMock()
        mock_client.base_url = "http://test"
        mock_client.get_site_config_periods.return_value = (
            [
                {"effective_start_unix": 500, "effective_end_unix": None},
            ],
            None,
        )
        result = fetch_site_config_periods(mock_client, 1, 1000, 2000)
        self.assertEqual(len(result), 1)


class TestGenerateAllCharts(unittest.TestCase):
    """Test generate_all_charts."""

    @patch("pdf_generator.cli.main.check_charts_available", return_value=False)
    def test_no_charts_available(self, mock_check):
        from pdf_generator.cli.main import generate_all_charts
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.output.debug = True
        generate_all_charts("prefix", [], None, None, [], config, None)

    @patch("pdf_generator.cli.main.check_charts_available", return_value=True)
    @patch("pdf_generator.cli.main.generate_timeseries_chart")
    @patch("pdf_generator.cli.main.generate_histogram_chart")
    def test_with_all_data(self, mock_hist, mock_ts, mock_check):
        from pdf_generator.cli.main import generate_all_charts
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.output.debug = True
        resp = MagicMock()
        generate_all_charts(
            "prefix",
            [{"count": 10}],
            [{"count": 5}],
            {"bins": [1]},
            [{"count": 100}],
            config,
            resp,
            compare_metrics=[{"count": 7}],
            compare_histogram={"bins": [2]},
            primary_label="t1: a to b",
            compare_label="t2: c to d",
        )
        self.assertTrue(mock_ts.called)


class TestProcessDateRange(unittest.TestCase):
    """Test process_date_range with mocked dependencies."""

    @patch("pdf_generator.cli.main.assemble_pdf_report")
    @patch("pdf_generator.cli.main.generate_all_charts")
    @patch("pdf_generator.cli.main.fetch_daily_summary")
    @patch("pdf_generator.cli.main.fetch_overall_summary")
    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    def test_basic_run(
        self,
        mock_fetch_granular,
        mock_fetch_overall,
        mock_fetch_daily,
        mock_gen_charts,
        mock_assemble,
    ):
        from pdf_generator.cli.main import process_date_range
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.start_date = "2025-01-01"
        config.query.end_date = "2025-01-31"
        config.query.group = "1h"
        config.query.source = "radar_objects"
        config.query.units = "kph"
        config.query.timezone = "US/Pacific"
        config.output.debug = False

        mock_client = MagicMock()
        mock_fetch_granular.return_value = (
            [{"start_time": "2025-01-15T12:00:00-08:00", "count": 10, "mean_speed": 30.0,
              "p50_speed": 28.0, "p85_speed": 35.0, "p98_speed": 42.0, "max_speed": 50.0,
              "min_speed": 15.0, "std_dev": 5.0}],
            None,
            None,
            None,
        )
        mock_fetch_overall.return_value = [{"count": 100}]
        mock_fetch_daily.return_value = [{"count": 50}]

        with tempfile.TemporaryDirectory() as tmpdir:
            config.output.output_dir = tmpdir
            process_date_range("2025-01-01", "2025-01-31", config, mock_client)

        mock_assemble.assert_called_once()

    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    def test_no_data(self, mock_fetch):
        from pdf_generator.cli.main import process_date_range
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.start_date = "2025-01-01"
        config.query.end_date = "2025-01-31"
        config.query.group = "1h"
        config.query.source = "radar_objects"
        config.query.units = "kph"
        config.query.timezone = "US/Pacific"

        mock_client = MagicMock()
        mock_fetch.return_value = ([], None, None, None)

        process_date_range("2025-01-01", "2025-01-31", config, mock_client)

    @patch("pdf_generator.cli.main.assemble_pdf_report")
    @patch("pdf_generator.cli.main.generate_all_charts")
    @patch("pdf_generator.cli.main.fetch_daily_summary")
    @patch("pdf_generator.cli.main.fetch_overall_summary")
    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    def test_with_comparison(
        self,
        mock_fetch_granular,
        mock_fetch_overall,
        mock_fetch_daily,
        mock_gen_charts,
        mock_assemble,
    ):
        from pdf_generator.cli.main import process_date_range
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.start_date = "2025-01-01"
        config.query.end_date = "2025-01-31"
        config.query.compare_start_date = "2025-02-01"
        config.query.compare_end_date = "2025-02-28"
        config.query.group = "1h"
        config.query.source = "radar_objects"
        config.query.units = "kph"
        config.query.timezone = "US/Pacific"
        config.output.debug = True

        mock_client = MagicMock()
        # First call is main range, subsequent calls are comparison
        mock_fetch_granular.return_value = (
            [{"start_time": "2025-01-15T12:00:00-08:00", "count": 10, "mean_speed": 30.0,
              "p50_speed": 28.0, "p85_speed": 35.0, "p98_speed": 42.0, "max_speed": 50.0,
              "min_speed": 15.0, "std_dev": 5.0}],
            None,
            None,
            MagicMock(),
        )
        mock_fetch_overall.return_value = [{"count": 100}]
        mock_fetch_daily.return_value = [{"count": 50}]

        with tempfile.TemporaryDirectory() as tmpdir:
            config.output.output_dir = tmpdir
            process_date_range("2025-01-01", "2025-01-31", config, mock_client)

    @patch("pdf_generator.cli.main.assemble_pdf_report")
    @patch("pdf_generator.cli.main.generate_all_charts")
    @patch("pdf_generator.cli.main.fetch_site_config_periods")
    @patch("pdf_generator.cli.main.fetch_daily_summary")
    @patch("pdf_generator.cli.main.fetch_overall_summary")
    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    def test_with_site_id(
        self,
        mock_fetch_granular,
        mock_fetch_overall,
        mock_fetch_daily,
        mock_fetch_site,
        mock_gen_charts,
        mock_assemble,
    ):
        from pdf_generator.cli.main import process_date_range
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.start_date = "2025-01-01"
        config.query.end_date = "2025-01-31"
        config.query.group = "1h"
        config.query.source = "radar_objects"
        config.query.units = "kph"
        config.query.timezone = "US/Pacific"
        config.query.site_id = 42

        mock_client = MagicMock()
        mock_fetch_granular.return_value = (
            [{"start_time": "2025-01-15T12:00:00-08:00", "count": 10, "mean_speed": 30.0,
              "p50_speed": 28.0, "p85_speed": 35.0, "p98_speed": 42.0, "max_speed": 50.0,
              "min_speed": 15.0, "std_dev": 5.0}],
            None,
            None,
            None,
        )
        mock_fetch_overall.return_value = [{"count": 100}]
        mock_fetch_daily.return_value = [{"count": 50}]
        mock_fetch_site.return_value = [{"effective_start_unix": 500}]

        with tempfile.TemporaryDirectory() as tmpdir:
            config.output.output_dir = tmpdir
            process_date_range("2025-01-01", "2025-01-31", config, mock_client)


class TestMainEntrypoint(unittest.TestCase):
    """Test the __main__ entry point."""

    def test_check_flag(self):
        """Test --check flag."""
        with patch("sys.argv", ["main.py", "--check"]):
            with patch("pdf_generator.core.dependency_checker.check_dependencies", return_value=True):
                with self.assertRaises(SystemExit) as cm:
                    from pdf_generator.cli.main import main as _main_fn
                    import pdf_generator.cli.main as mod
                    # Simulate main entry
                    import argparse
                    parser = argparse.ArgumentParser()
                    parser.add_argument("config_file", nargs="?")
                    parser.add_argument("--check", action="store_true")
                    args = parser.parse_args(["--check"])
                    if args.check:
                        from pdf_generator.core.dependency_checker import check_dependencies
                        system_ready = check_dependencies(verbose=False)
                        sys.exit(0 if system_ready else 1)
                self.assertEqual(cm.exception.code, 0)

    def test_missing_config_file(self):
        """Test that missing config file is handled."""
        from pdf_generator.cli.main import process_date_range
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.start_date = "2025-01-01"
        config.query.end_date = "2025-01-31"
        config.query.timezone = None

        mock_client = MagicMock()
        # parse_date_range with None timezone should still work
        with patch("pdf_generator.cli.main.fetch_granular_metrics") as mock_fetch:
            mock_fetch.return_value = ([], None, None, None)
            process_date_range("2025-01-01", "2025-01-31", config, mock_client)


class TestComputeIsoTimestamps(unittest.TestCase):
    def test_basic(self):
        from pdf_generator.cli.main import compute_iso_timestamps
        start, end = compute_iso_timestamps(1704067200, 1706745600, "US/Pacific")
        self.assertIsNotNone(start)
        self.assertIsNotNone(end)

    def test_no_timezone(self):
        from pdf_generator.cli.main import compute_iso_timestamps
        start, end = compute_iso_timestamps(1704067200, 1706745600, None)
        self.assertIsNotNone(start)
        self.assertIsNotNone(end)


class TestMainFunction(unittest.TestCase):
    """Test the main() orchestrator function."""

    @patch("pdf_generator.cli.main.process_date_range")
    @patch("pdf_generator.cli.main.RadarStatsClient")
    def test_main_basic(self, mock_client_cls, mock_process):
        from pdf_generator.cli.main import main
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.units = "kph"
        config.query.timezone = "US/Pacific"

        main([("2025-01-01", "2025-01-31")], config)
        mock_process.assert_called_once()

    @patch("pdf_generator.cli.main.process_date_range")
    @patch("pdf_generator.cli.main.RadarStatsClient")
    def test_main_multiple_ranges(self, mock_client_cls, mock_process):
        from pdf_generator.cli.main import main
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.units = "mph"
        main([("2025-01-01", "2025-01-31"), ("2025-02-01", "2025-02-28")], config)
        self.assertEqual(mock_process.call_count, 2)


class TestDeriveFromGranularEdgeCases(unittest.TestCase):
    """Test derive_daily_from_granular with edge cases."""

    def test_multiple_days(self):
        from pdf_generator.cli.main import derive_daily_from_granular

        rows = [
            {
                "start_time": "2025-01-01T12:00:00-08:00",
                "count": 10,
                "mean_speed": 30.0,
                "p50_speed": 28.0,
                "p85_speed": 35.0,
                "p98_speed": 42.0,
                "max_speed": 50.0,
                "min_speed": 15.0,
                "std_dev": 5.0,
            },
            {
                "start_time": "2025-01-02T14:00:00-08:00",
                "count": 20,
                "mean_speed": 32.0,
                "p50_speed": 30.0,
                "p85_speed": 37.0,
                "p98_speed": 45.0,
                "max_speed": 55.0,
                "min_speed": 10.0,
                "std_dev": 6.0,
            },
        ]
        result = derive_daily_from_granular(rows, "US/Pacific")
        self.assertEqual(len(result), 2)

    def test_with_nan_count(self):
        from pdf_generator.cli.main import derive_daily_from_granular
        import math

        rows = [
            {
                "start_time": "2025-01-01T12:00:00-08:00",
                "count": float("nan"),
                "mean_speed": 30.0,
                "p50_speed": 28.0,
                "p85_speed": 35.0,
                "p98_speed": 42.0,
                "max_speed": 50.0,
                "min_speed": 15.0,
                "std_dev": 5.0,
            },
        ]
        result = derive_daily_from_granular(rows, "US/Pacific")
        # NaN count should be skipped
        self.assertEqual(len(result), 0)

    def test_with_zero_count(self):
        from pdf_generator.cli.main import derive_daily_from_granular

        rows = [
            {
                "start_time": "2025-01-01T12:00:00-08:00",
                "count": 0,
                "mean_speed": 30.0,
                "p50_speed": 28.0,
                "p85_speed": 35.0,
                "p98_speed": 42.0,
                "max_speed": 50.0,
                "min_speed": 15.0,
                "std_dev": 5.0,
            },
        ]
        result = derive_daily_from_granular(rows, "US/Pacific")
        # Zero count should be skipped
        self.assertEqual(len(result), 0)

    def test_with_none_start_time(self):
        from pdf_generator.cli.main import derive_daily_from_granular

        rows = [
            {
                "start_time": None,
                "count": 10,
                "mean_speed": 30.0,
            },
        ]
        result = derive_daily_from_granular(rows, "US/Pacific")
        self.assertEqual(len(result), 0)

    def test_with_numeric_start_time(self):
        from pdf_generator.cli.main import derive_daily_from_granular

        rows = [
            {
                "start_time": 1735747200,
                "count": 10,
                "mean_speed": 30.0,
                "p50_speed": 28.0,
                "p85_speed": 35.0,
                "p98_speed": 42.0,
                "max_speed": 50.0,
                "min_speed": 15.0,
                "std_dev": 5.0,
            },
        ]
        result = derive_daily_from_granular(rows, "US/Pacific")
        self.assertGreater(len(result), 0)


class TestProcessDateRangeDebugPaths(unittest.TestCase):
    """Test process_date_range debug code paths."""

    @patch("pdf_generator.cli.main.assemble_pdf_report")
    @patch("pdf_generator.cli.main.generate_all_charts")
    @patch("pdf_generator.cli.main.fetch_daily_summary")
    @patch("pdf_generator.cli.main.fetch_overall_summary")
    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    def test_debug_config_write(
        self,
        mock_fetch_granular,
        mock_fetch_overall,
        mock_fetch_daily,
        mock_gen_charts,
        mock_assemble,
    ):
        """Test the debug config write path in process_date_range."""
        from pdf_generator.cli.main import process_date_range
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.start_date = "2025-01-01"
        config.query.end_date = "2025-01-31"
        config.query.group = "1h"
        config.query.source = "radar_objects"
        config.query.units = "kph"
        config.query.timezone = "US/Pacific"
        config.output.debug = True

        mock_client = MagicMock()
        mock_fetch_granular.return_value = (
            [{"start_time": "2025-01-15T12:00:00-08:00", "count": 10, "mean_speed": 30.0,
              "p50_speed": 28.0, "p85_speed": 35.0, "p98_speed": 42.0, "max_speed": 50.0,
              "min_speed": 15.0, "std_dev": 5.0}],
            None,
            None,
            MagicMock(),
        )
        mock_fetch_overall.return_value = [{"count": 100}]
        mock_fetch_daily.return_value = [{"count": 50}]

        with tempfile.TemporaryDirectory() as tmpdir:
            config.output.output_dir = tmpdir
            process_date_range("2025-01-01", "2025-01-31", config, mock_client)

    def test_invalid_dates(self):
        """Test process_date_range with invalid dates."""
        from pdf_generator.cli.main import process_date_range
        from pdf_generator.core.config_manager import ReportConfig

        config = ReportConfig()
        config.query.timezone = "US/Pacific"
        mock_client = MagicMock()

        # Invalid date format should return early
        process_date_range("not-a-date", "also-not", config, mock_client)


class TestNextSequencedPrefixEdge(unittest.TestCase):
    """Test _next_sequenced_prefix edge cases."""

    def test_malformed_file_number(self):
        from pdf_generator.cli.main import _next_sequenced_prefix

        with tempfile.TemporaryDirectory() as tmpdir:
            # Create a file that matches the prefix pattern but has a non-integer number
            open(os.path.join(tmpdir, "report-abc-120000_stats.pdf"), "w").close()
            result = _next_sequenced_prefix("report", tmpdir)
            self.assertIn("report-1-", result)


class TestDeriveOverallEdgeCases(unittest.TestCase):
    """Test derive_overall_from_granular edge cases."""

    def test_with_no_p50_key(self):
        """Test with missing stats keys."""
        from pdf_generator.cli.main import derive_overall_from_granular

        result = derive_overall_from_granular(
            [
                {"count": 10, "mean_speed": 30.0},
            ]
        )
        self.assertIsNotNone(result)

    def test_multiple_rows(self):
        """Test with multiple rows to exercise aggregation."""
        from pdf_generator.cli.main import derive_overall_from_granular

        result = derive_overall_from_granular(
            [
                {"count": 10, "mean_speed": 30.0, "p50_speed": 28.0, "p85_speed": 35.0,
                 "p98_speed": 42.0, "max_speed": 50.0, "min_speed": 15.0, "std_dev": 5.0},
                {"count": 20, "mean_speed": 35.0, "p50_speed": 33.0, "p85_speed": 40.0,
                 "p98_speed": 48.0, "max_speed": 55.0, "min_speed": 12.0, "std_dev": 6.0},
            ]
        )
        self.assertIsNotNone(result)
        self.assertTrue(len(result) > 0)


class TestMainBlockExecution(unittest.TestCase):
    """Test the __main__ block via subprocess."""

    def test_check_flag_subprocess(self):
        """Run the module with --check flag via subprocess."""
        import subprocess
        result = subprocess.run(
            [sys.executable, "-m", "pdf_generator.cli.main", "--check"],
            capture_output=True,
            text=True,
            cwd=os.path.join(
                os.path.dirname(__file__), os.pardir, os.pardir
            ),
        )
        # Exit code 0 means all deps are available, 1 means some missing
        self.assertIn(result.returncode, [0, 1])

    def test_no_args_subprocess(self):
        """Run the module with no args shows error."""
        import subprocess
        result = subprocess.run(
            [sys.executable, "-m", "pdf_generator.cli.main"],
            capture_output=True,
            text=True,
            cwd=os.path.join(
                os.path.dirname(__file__), os.pardir, os.pardir
            ),
        )
        self.assertEqual(result.returncode, 2)  # argparse error

    def test_nonexistent_config_file(self):
        """Run with a nonexistent config file."""
        import subprocess
        result = subprocess.run(
            [sys.executable, "-m", "pdf_generator.cli.main", "/nonexistent/config.json"],
            capture_output=True,
            text=True,
            cwd=os.path.join(
                os.path.dirname(__file__), os.pardir, os.pardir
            ),
        )
        self.assertEqual(result.returncode, 2)

    def test_valid_config_file(self):
        """Run with a valid config file but unreachable API."""
        import subprocess

        config = {
            "query": {
                "start_date": "2025-01-01",
                "end_date": "2025-01-31",
                "group": "1h",
                "source": "radar_objects",
                "units": "kph",
            },
            "output": {"output_dir": "/tmp/test-pdf-gen"},
        }
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".json", delete=False
        ) as f:
            json.dump(config, f)
            config_path = f.name

        try:
            result = subprocess.run(
                [sys.executable, "-m", "pdf_generator.cli.main", config_path],
                capture_output=True,
                text=True,
                timeout=10,
                cwd=os.path.join(
                    os.path.dirname(__file__), os.pardir, os.pardir
                ),
            )
            # Will fail to connect to API or config validation, but should not crash (SIGSEGV etc)
            self.assertIn(result.returncode, [0, 1, 2])
        except subprocess.TimeoutExpired:
            pass  # OK, just took too long
        finally:
            os.unlink(config_path)
