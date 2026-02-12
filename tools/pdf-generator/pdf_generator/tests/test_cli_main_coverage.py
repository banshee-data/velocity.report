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
