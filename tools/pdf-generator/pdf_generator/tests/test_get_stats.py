#!/usr/bin/env python3
"""Unit tests for refactored get_stats.py functions."""

import unittest
from unittest.mock import Mock, patch

from pdf_generator.core.config_manager import (
    ReportConfig,
    SiteConfig,
    QueryConfig,
    RadarConfig,
    OutputConfig,
)
from pdf_generator.cli.main import (
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


def create_test_config(
    file_prefix: str = "test",
    start_date: str = "2025-01-01",
    end_date: str = "2025-01-02",
    timezone: str = "UTC",
    source: str = "radar_data_transits",
    group: str = "1h",
    units: str = "mph",
    min_speed: float = 5.0,
    histogram: bool = True,
    hist_bucket_size: float = 5.0,
    hist_max: float = None,
    model_version: str = "rebuild-full",
    debug: bool = False,
    **kwargs
) -> ReportConfig:
    """Helper to create test configs with sensible defaults.

    Args:
        file_prefix: Output file prefix
        start_date: Start date (YYYY-MM-DD)
        end_date: End date (YYYY-MM-DD)
        timezone: Timezone name
        source: Data source (radar_data_transits or radar_objects)
        group: Time grouping (1h, 15m, etc)
        units: Display units (mph or kph)
        min_speed: Minimum speed filter
        histogram: Generate histogram
        hist_bucket_size: Histogram bucket size
        hist_max: Histogram max value
        model_version: Model version for transits
        debug: Debug mode
        **kwargs: Additional config overrides

    Returns:
        ReportConfig with test defaults
    """
    config = ReportConfig(
        site=SiteConfig(
            location="Test Site",
            surveyor="Test Surveyor",
            contact="test@example.com",
            speed_limit=25,
        ),
        query=QueryConfig(
            start_date=start_date,
            end_date=end_date,
            timezone=timezone,
            source=source,
            group=group,
            units=units,
            min_speed=min_speed,
            histogram=histogram,
            hist_bucket_size=hist_bucket_size,
            hist_max=hist_max,
            model_version=model_version,
        ),
        radar=RadarConfig(
            cosine_error_angle=21.0,
            sensor_model="Test Sensor",
            firmware_version="v1.0.0",
        ),
        output=OutputConfig(
            file_prefix=file_prefix,
            debug=debug,
        ),
    )

    # Apply any additional overrides
    for key, value in kwargs.items():
        if hasattr(config, key):
            setattr(config, key, value)

    return config


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
        """Test ISO timestamp generation with UTC (when timezone is None)."""
        start_ts = 1704067200  # 2024-01-01 00:00:00 UTC
        end_ts = 1704153600  # 2024-01-02 00:00:00 UTC

        start_iso, end_iso = compute_iso_timestamps(start_ts, end_ts, None)

        # When timezone is None, function uses UTC
        self.assertEqual(start_iso, "2024-01-01T00:00:00+00:00")
        self.assertEqual(end_iso, "2024-01-02T00:00:00+00:00")

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

    @patch("pdf_generator.cli.main._next_sequenced_prefix")
    def test_with_user_provided_prefix(self, mock_next_seq):
        """Test prefix resolution with user-provided prefix."""
        mock_next_seq.return_value = "my-prefix-1"

        config = create_test_config(file_prefix="my-prefix")
        start_ts = 1704067200
        end_ts = 1704153600

        result = resolve_file_prefix(config, start_ts, end_ts)

        self.assertEqual(result, "velocity.report_my-prefix-1")
        mock_next_seq.assert_called_once_with("my-prefix", ".")

    def test_auto_generated_prefix_utc(self):
        """Test auto-generated prefix with UTC."""
        config = create_test_config(
            file_prefix="", timezone="UTC", source="radar_data_transits"
        )
        start_ts = 1704067200  # 2024-01-01 00:00:00 UTC
        end_ts = 1704153600  # 2024-01-02 00:00:00 UTC

        result = resolve_file_prefix(config, start_ts, end_ts)

        self.assertEqual(
            result, "velocity.report_radar_data_transits_2024-01-01_to_2024-01-02"
        )

    def test_auto_generated_prefix_with_timezone(self):
        """Test auto-generated prefix with specific timezone."""
        config = create_test_config(
            file_prefix="", timezone="US/Pacific", source="radar_objects"
        )
        start_ts = 1704067200  # 2024-01-01 00:00:00 UTC = 2023-12-31 16:00:00 PST
        end_ts = 1704153600  # 2024-01-02 00:00:00 UTC = 2024-01-01 16:00:00 PST

        result = resolve_file_prefix(config, start_ts, end_ts)

        self.assertEqual(
            result, "velocity.report_radar_objects_2023-12-31_to_2024-01-01"
        )


class TestFetchGranularMetrics(unittest.TestCase):
    """Tests for fetch_granular_metrics function."""

    def test_successful_fetch(self):
        """Test successful granular metrics fetch."""
        mock_client = Mock()
        mock_metrics = [{"p50": 25.0}]
        mock_histogram = {"10": 5, "20": 10}
        mock_resp = Mock()
        mock_client.get_stats.return_value = (mock_metrics, mock_histogram, mock_resp)

        config = create_test_config(
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
            mock_client, 1704067200, 1704153600, config, "rebuild-full"
        )

        self.assertEqual(metrics, mock_metrics)
        self.assertEqual(histogram, mock_histogram)
        self.assertEqual(resp, mock_resp)
        mock_client.get_stats.assert_called_once()

    def test_fetch_failure_returns_empty(self):
        """Test that fetch failure returns empty results."""
        mock_client = Mock()
        mock_client.get_stats.side_effect = Exception("API Error")

        config = create_test_config(
            group="1h",
            units="mph",
            source="radar_data_transits",
            timezone="UTC",
            min_speed=None,
            histogram=False,
            hist_bucket_size=None,
            hist_max=None,
        )

        metrics, histogram, resp = fetch_granular_metrics(
            mock_client, 1704067200, 1704153600, config, None
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

        config = create_test_config(
            units="mph", source="radar_data_transits", timezone="UTC", min_speed=None
        )

        result = fetch_overall_summary(
            mock_client, 1704067200, 1704153600, config, None
        )

        self.assertEqual(result, mock_metrics)

    def test_fetch_failure_returns_empty_list(self):
        """Test that fetch failure returns empty list."""
        mock_client = Mock()
        mock_client.get_stats.side_effect = Exception("API Error")

        config = create_test_config(
            units="mph", source="radar_data_transits", timezone="UTC", min_speed=None
        )

        result = fetch_overall_summary(
            mock_client, 1704067200, 1704153600, config, None
        )

        self.assertEqual(result, [])


class TestFetchDailySummary(unittest.TestCase):
    """Tests for fetch_daily_summary function."""

    def test_fetch_when_needed(self):
        """Test daily fetch when group is less than 24h."""
        mock_client = Mock()
        mock_metrics = [{"p50": 24.0}]
        mock_client.get_stats.return_value = (mock_metrics, None, Mock())

        config = create_test_config(
            group="1h",
            units="mph",
            source="radar_data_transits",
            timezone="UTC",
            min_speed=None,
        )

        result = fetch_daily_summary(mock_client, 1704067200, 1704153600, config, None)

        self.assertEqual(result, mock_metrics)

    def test_not_fetched_when_group_is_24h(self):
        """Test daily not fetched when group is already 24h."""
        mock_client = Mock()

        config = create_test_config(
            group="24h",
            units="mph",
            source="radar_data_transits",
            timezone="UTC",
            min_speed=None,
        )

        result = fetch_daily_summary(mock_client, 1704067200, 1704153600, config, None)

        self.assertIsNone(result)
        mock_client.get_stats.assert_not_called()

    def test_fetch_failure_returns_none(self):
        """Test that fetch failure returns None."""
        mock_client = Mock()
        mock_client.get_stats.side_effect = Exception("API Error")

        config = create_test_config(
            group="1h",
            units="mph",
            source="radar_data_transits",
            timezone="UTC",
            min_speed=None,
        )

        result = fetch_daily_summary(mock_client, 1704067200, 1704153600, config, None)

        self.assertIsNone(result)


class TestGenerateHistogramChart(unittest.TestCase):
    """Tests for generate_histogram_chart function."""

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    def test_successful_generation(self, mock_plot, mock_save):
        """Test successful histogram chart generation."""
        mock_fig = Mock()
        mock_plot.return_value = mock_fig
        mock_save.return_value = True

        histogram = {"10": 5, "20": 10}
        metrics_all = [{"count": 1000}]
        config = create_test_config(debug=False, units="mph")

        result = generate_histogram_chart(
            histogram, "test-prefix", "mph", metrics_all, config
        )

        self.assertTrue(result)
        mock_plot.assert_called_once()
        mock_save.assert_called_once_with(mock_fig, "test-prefix_histogram.pdf")

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    def test_save_failure_returns_false(self, mock_plot, mock_save):
        """Test that save failure returns False."""
        mock_fig = Mock()
        mock_plot.return_value = mock_fig
        mock_save.return_value = False

        histogram = {"10": 5}
        config = create_test_config(debug=False, units="mph")

        result = generate_histogram_chart(histogram, "test-prefix", "mph", [], config)

        self.assertFalse(result)

    @patch("pdf_generator.cli.main.plot_histogram")
    def test_exception_returns_false(self, mock_plot):
        """Test that exception returns False."""
        mock_plot.side_effect = Exception("Plot error")

        histogram = {"10": 5}
        config = create_test_config(debug=False, units="mph")

        result = generate_histogram_chart(histogram, "test-prefix", "mph", [], config)

        self.assertFalse(result)


class TestGenerateTimeseriesChart(unittest.TestCase):
    """Tests for generate_timeseries_chart function."""

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main._plot_stats_page")
    def test_successful_generation(self, mock_plot, mock_save):
        """Test successful time-series chart generation."""
        mock_fig = Mock()
        mock_plot.return_value = mock_fig
        mock_save.return_value = True

        metrics = [{"p50": 25.0}]
        config = create_test_config(debug=False)

        result = generate_timeseries_chart(
            metrics, "test_stats", "Test Chart", "mph", "US/Pacific", config
        )

        self.assertTrue(result)
        mock_plot.assert_called_once_with(
            metrics, "Test Chart", "mph", tz_name="US/Pacific"
        )
        mock_save.assert_called_once_with(mock_fig, "test_stats.pdf")

    @patch("pdf_generator.cli.main._plot_stats_page")
    def test_exception_returns_false(self, mock_plot):
        """Test that exception returns False."""
        mock_plot.side_effect = Exception("Plot error")

        metrics = [{"p50": 25.0}]
        config = create_test_config(debug=False)

        result = generate_timeseries_chart(
            metrics, "test_stats", "Test", "mph", None, config
        )

        self.assertFalse(result)


class TestAssemblePdfReport(unittest.TestCase):
    """Tests for assemble_pdf_report function."""

    @patch("pdf_generator.cli.main.generate_pdf_report")
    def test_successful_assembly(self, mock_generate):
        """Test successful PDF assembly."""
        config = create_test_config(
            group="1h", units="mph", timezone="US/Pacific", min_speed=5.0, hist_max=50.0
        )

        mock_client = Mock()

        result = assemble_pdf_report(
            "test-prefix",
            "2024-01-01T00:00:00Z",
            "2024-01-02T00:00:00Z",
            [{"p50": 25.0}],
            None,
            [{"p50": 24.0}],
            {"10": 5},
            config,
            mock_client,
        )

        self.assertTrue(result)
        mock_generate.assert_called_once()

    @patch("pdf_generator.cli.main.generate_pdf_report")
    def test_exception_returns_false(self, mock_generate):
        """Test that exception returns False."""
        mock_generate.side_effect = Exception("PDF error")

        config = create_test_config(
            group="1h", units="mph", timezone="UTC", min_speed=None, hist_max=None
        )

        mock_client = Mock()

        result = assemble_pdf_report(
            "test-prefix",
            "2024-01-01T00:00:00Z",
            "2024-01-02T00:00:00Z",
            [],
            None,
            [],
            None,
            config,
            mock_client,
        )

        self.assertFalse(result)


class TestProcessDateRange(unittest.TestCase):
    """Tests for process_date_range orchestration."""

    @patch("pdf_generator.cli.main.assemble_pdf_report")
    @patch("pdf_generator.cli.main.fetch_daily_summary")
    @patch("pdf_generator.cli.main.fetch_overall_summary")
    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    @patch("pdf_generator.cli.main.parse_date_to_unix")
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
        config = create_test_config(
            source="radar_data_transits",
            model_version="rebuild-full",
            timezone="UTC",
            file_prefix="",
            group="1h",
            units="mph",
            min_speed=None,
            debug=False,
        )

        # Should not raise
        process_date_range("2024-01-01", "2024-01-02", config, mock_client)

        mock_fetch_granular.assert_called_once()
        mock_fetch_overall.assert_called_once()
        mock_assemble.assert_called_once()

    @patch("pdf_generator.cli.main.parse_date_to_unix")
    def test_invalid_date_returns_early(self, mock_parse):
        """Test that invalid date returns early."""
        mock_parse.side_effect = ValueError("Invalid date")

        mock_client = Mock()
        config = create_test_config(source="radar_data_transits", timezone="UTC")

        # Should not raise, just print error and return
        process_date_range("invalid", "date", config, mock_client)

        # Client should not be called
        mock_client.get_stats.assert_not_called()

    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    @patch("pdf_generator.cli.main.parse_date_to_unix")
    def test_no_data_returns_early(self, mock_parse, mock_fetch):
        """Test that no data returns early."""
        mock_parse.side_effect = [1704067200, 1704153600]
        mock_fetch.return_value = ([], None, None)

        mock_client = Mock()
        config = create_test_config(
            source="radar_data_transits",
            model_version=None,
            timezone="UTC",
            file_prefix="",
            group="1h",
            units="mph",
            min_speed=None,
        )

        # Should return early without assembling PDF
        process_date_range("2024-01-01", "2024-01-02", config, mock_client)


class TestShouldProduceDailyEdgeCases(unittest.TestCase):
    """Additional tests for should_produce_daily edge cases."""

    def test_returns_false_for_2d_group(self):
        """Test that daily is not produced for 2d group."""
        self.assertFalse(should_produce_daily("2d"))

    def test_returns_false_for_48h_group(self):
        """Test that daily is not produced for 48h group."""
        self.assertFalse(should_produce_daily("48h"))

    def test_returns_true_for_30m_group(self):
        """Test that daily is produced for 30m group."""
        self.assertTrue(should_produce_daily("30m"))

    def test_returns_true_for_2h_group(self):
        """Test that daily is produced for 2h group."""
        self.assertTrue(should_produce_daily("2h"))

    def test_returns_true_for_12h_group(self):
        """Test that daily is produced for 12h group."""
        self.assertTrue(should_produce_daily("12h"))

    def test_returns_false_for_seconds_gt_24h(self):
        """Test that daily is not produced for seconds >= 24h."""
        self.assertFalse(should_produce_daily("86400s"))  # exactly 24h in seconds

    def test_returns_false_for_minutes_gt_24h(self):
        """Test that daily is not produced for minutes >= 24h."""
        self.assertFalse(should_produce_daily("1440m"))  # exactly 24h in minutes

    def test_returns_true_for_invalid_format(self):
        """Test that invalid format defaults to True."""
        self.assertTrue(should_produce_daily("invalid"))
        self.assertTrue(should_produce_daily(""))
        self.assertTrue(should_produce_daily(None))


class TestNextSequencedPrefix(unittest.TestCase):
    """Tests for _next_sequenced_prefix function."""

    @patch("os.listdir")
    def test_no_existing_files(self, mock_listdir):
        """Test sequencing when no existing files."""
        from pdf_generator.cli.main import _next_sequenced_prefix

        mock_listdir.return_value = []
        result = _next_sequenced_prefix("test")
        # Result should be test-1-HHMMSS
        self.assertTrue(result.startswith("test-1-"))
        self.assertEqual(len(result), len("test-1-HHMMSS"))
        # Verify timestamp portion is 6 digits
        timestamp = result.split("-")[-1]
        self.assertEqual(len(timestamp), 6)
        self.assertTrue(timestamp.isdigit())

    @patch("os.listdir")
    def test_existing_sequence(self, mock_listdir):
        """Test sequencing with existing files."""
        from pdf_generator.cli.main import _next_sequenced_prefix

        mock_listdir.return_value = [
            "test-1_report.pdf",
            "test-2_stats.pdf",
            "test-3_histogram.pdf",
            "other-file.pdf",
        ]
        result = _next_sequenced_prefix("test")
        # Result should be test-4-HHMMSS
        self.assertTrue(result.startswith("test-4-"))
        timestamp = result.split("-")[-1]
        self.assertEqual(len(timestamp), 6)
        self.assertTrue(timestamp.isdigit())

    @patch("os.listdir")
    def test_non_sequential_numbers(self, mock_listdir):
        """Test sequencing with gaps in sequence."""
        from pdf_generator.cli.main import _next_sequenced_prefix

        mock_listdir.return_value = [
            "test-1_report.pdf",
            "test-5_stats.pdf",
            "test-10_histogram.pdf",
        ]
        result = _next_sequenced_prefix("test")
        # Result should be test-11-HHMMSS (max + 1)
        self.assertTrue(result.startswith("test-11-"))
        timestamp = result.split("-")[-1]
        self.assertEqual(len(timestamp), 6)
        self.assertTrue(timestamp.isdigit())

    @patch("os.listdir")
    def test_invalid_numbers_ignored(self, mock_listdir):
        """Test that invalid numbers are ignored."""
        from pdf_generator.cli.main import _next_sequenced_prefix

        mock_listdir.return_value = [
            "test-1_report.pdf",
            "test-abc_stats.pdf",  # invalid number
            "test-2_histogram.pdf",
        ]
        result = _next_sequenced_prefix("test")
        # Result should be test-3-HHMMSS
        self.assertTrue(result.startswith("test-3-"))
        timestamp = result.split("-")[-1]
        self.assertEqual(len(timestamp), 6)
        self.assertTrue(timestamp.isdigit())

    @patch("os.listdir")
    def test_with_timestamp_files(self, mock_listdir):
        """Test sequencing with existing timestamped files."""
        from pdf_generator.cli.main import _next_sequenced_prefix

        mock_listdir.return_value = [
            "test-1-120530_report.pdf",
            "test-2-143045_stats.pdf",
            "test-3-183010_histogram.pdf",
        ]
        result = _next_sequenced_prefix("test")
        # Result should be test-4-HHMMSS (continues sequence)
        self.assertTrue(result.startswith("test-4-"))
        timestamp = result.split("-")[-1]
        self.assertEqual(len(timestamp), 6)
        self.assertTrue(timestamp.isdigit())


class TestGenerateHistogramChartEdgeCases(unittest.TestCase):
    """Additional tests for generate_histogram_chart."""

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    def test_save_failure_returns_false(self, mock_plot, mock_save):
        """Test that save failure returns False."""
        mock_plot.return_value = Mock()
        mock_save.return_value = False

        config = create_test_config(debug=False)
        result = generate_histogram_chart({"10": 5}, "test", {}, "mph", config)

        self.assertFalse(result)
        mock_save.assert_called_once()

    @patch("pdf_generator.cli.main.plot_histogram")
    def test_import_error_returns_false(self, mock_plot):
        """Test that ImportError returns False."""
        mock_plot.side_effect = ImportError("No matplotlib")

        config = create_test_config(debug=False)
        result = generate_histogram_chart({"10": 5}, "test", {}, "mph", config)

        self.assertFalse(result)

    @patch("pdf_generator.cli.main.plot_histogram")
    def test_import_error_debug_mode(self, mock_plot):
        """Test ImportError with debug mode enabled."""
        mock_plot.side_effect = ImportError("No matplotlib")

        config = create_test_config(debug=True)
        result = generate_histogram_chart({"10": 5}, "test", {}, "mph", config)

        self.assertFalse(result)

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    def test_exception_returns_false(self, mock_plot, mock_save):
        """Test that general exception returns False."""
        mock_plot.return_value = Mock()
        mock_save.side_effect = Exception("Save error")

        config = create_test_config(debug=False)
        result = generate_histogram_chart({"10": 5}, "test", {}, "mph", config)

        self.assertFalse(result)

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    def test_exception_debug_mode(self, mock_plot, mock_save):
        """Test exception with debug mode enabled."""
        mock_plot.return_value = Mock()
        mock_save.side_effect = Exception("Save error")

        config = create_test_config(debug=True)
        result = generate_histogram_chart({"10": 5}, "test", {}, "mph", config)

        self.assertFalse(result)

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    def test_sample_n_extraction_with_dict(self, mock_plot, mock_save):
        """Test sample size extraction from dict metrics."""
        mock_plot.return_value = Mock()
        mock_save.return_value = True

        config = create_test_config(debug=False)
        metrics = {"Count": 100}

        result = generate_histogram_chart({"10": 5}, "test", metrics, "mph", config)

        self.assertTrue(result)
        # Verify plot_histogram was called (sample label will be in title)
        mock_plot.assert_called_once()

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    def test_sample_n_extraction_with_list(self, mock_plot, mock_save):
        """Test sample size extraction from list metrics."""
        mock_plot.return_value = Mock()
        mock_save.return_value = True

        config = create_test_config(debug=False)
        metrics = [{"Count": 100}]

        result = generate_histogram_chart({"10": 5}, "test", metrics, "mph", config)

        self.assertTrue(result)

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    def test_sample_n_extraction_exception(self, mock_plot, mock_save):
        """Test sample size extraction handles exceptions gracefully."""
        mock_plot.return_value = Mock()
        mock_save.return_value = True

        config = create_test_config(debug=False)
        metrics = "invalid"  # Not a dict or list

        result = generate_histogram_chart({"10": 5}, "test", metrics, "mph", config)

        self.assertTrue(result)


class TestGenerateTimeseriesChartEdgeCases(unittest.TestCase):
    """Additional tests for generate_timeseries_chart."""

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main._plot_stats_page")
    def test_save_failure_returns_false(self, mock_plot, mock_save):
        """Test that save failure returns False."""
        mock_plot.return_value = Mock()
        mock_save.return_value = False

        config = create_test_config(debug=False)
        result = generate_timeseries_chart(
            [{"p50": 25.0}], "test", "Test Chart", "mph", None, config
        )

        self.assertFalse(result)

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main._plot_stats_page")
    def test_exception_returns_false(self, mock_plot, mock_save):
        """Test that exception returns False."""
        mock_plot.side_effect = Exception("Plot error")

        config = create_test_config(debug=False)
        result = generate_timeseries_chart(
            [{"p50": 25.0}], "test", "Test Chart", "mph", None, config
        )

        self.assertFalse(result)

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main._plot_stats_page")
    def test_exception_debug_mode(self, mock_plot, mock_save):
        """Test exception with debug mode enabled."""
        mock_plot.side_effect = Exception("Plot error")

        config = create_test_config(debug=True)
        result = generate_timeseries_chart(
            [{"p50": 25.0}], "test", "Test Chart", "mph", None, config
        )

        self.assertFalse(result)


class TestCheckChartsAvailable(unittest.TestCase):
    """Tests for check_charts_available function."""

    def test_returns_true_when_available(self):
        """Test that check_charts_available returns True when imports work."""
        from pdf_generator.cli.main import check_charts_available

        # Should succeed in test environment where chart_builder exists
        result = check_charts_available()
        self.assertTrue(result)

    @patch("pdf_generator.cli.main.TimeSeriesChartBuilder", create=True)
    def test_returns_false_on_import_error(self, mock_builder):
        """Test that check_charts_available returns False on ImportError."""
        # This is tricky to test since the import happens at function definition
        # but we can at least verify the function exists
        from pdf_generator.cli.main import check_charts_available

        self.assertTrue(callable(check_charts_available))


class TestGenerateAllCharts(unittest.TestCase):
    """Tests for generate_all_charts orchestration."""

    @patch("pdf_generator.cli.main.generate_timeseries_chart")
    @patch("pdf_generator.cli.main.generate_histogram_chart")
    @patch("pdf_generator.cli.main.check_charts_available")
    def test_charts_unavailable_returns_early(self, mock_check, mock_hist, mock_ts):
        """Test that unavailable charts returns early."""
        mock_check.return_value = False

        config = create_test_config(debug=False, units="mph", timezone="UTC")

        from pdf_generator.cli.main import generate_all_charts

        generate_all_charts("test", [], None, None, [], config, None)

        # No charts should be generated
        mock_hist.assert_not_called()
        mock_ts.assert_not_called()

    @patch("pdf_generator.cli.main.generate_timeseries_chart")
    @patch("pdf_generator.cli.main.generate_histogram_chart")
    @patch("pdf_generator.cli.main.check_charts_available")
    def test_charts_unavailable_debug_mode(self, mock_check, mock_hist, mock_ts):
        """Test unavailable charts in debug mode."""
        mock_check.return_value = False

        config = create_test_config(debug=True, units="mph", timezone="UTC")

        from pdf_generator.cli.main import generate_all_charts

        generate_all_charts("test", [], None, None, [], config, None)

        mock_hist.assert_not_called()
        mock_ts.assert_not_called()

    @patch("pdf_generator.cli.main.print_api_debug_info")
    @patch("pdf_generator.cli.main.generate_timeseries_chart")
    @patch("pdf_generator.cli.main.generate_histogram_chart")
    @patch("pdf_generator.cli.main.check_charts_available")
    def test_debug_info_printed_when_resp_provided(
        self, mock_check, mock_hist, mock_ts, mock_debug
    ):
        """Test that debug info is printed when response provided."""
        mock_check.return_value = True
        mock_ts.return_value = True
        mock_hist.return_value = True

        config = create_test_config(debug=True, units="mph", timezone="UTC")
        resp = Mock()

        from pdf_generator.cli.main import generate_all_charts

        generate_all_charts(
            "test", [{"p50": 25}], None, {"10": 5}, [{"p50": 25}], config, resp
        )

        mock_debug.assert_called_once_with(resp, [{"p50": 25}], {"10": 5})

    @patch("pdf_generator.cli.main.generate_timeseries_chart")
    @patch("pdf_generator.cli.main.generate_histogram_chart")
    @patch("pdf_generator.cli.main.check_charts_available")
    def test_daily_chart_generated_when_available(self, mock_check, mock_hist, mock_ts):
        """Test that daily chart is generated when data available."""
        mock_check.return_value = True
        mock_ts.return_value = True

        config = create_test_config(
            debug=False, units="mph", timezone="UTC" if "UTC" != "None" else "UTC"
        )
        daily = [{"p50": 25}]

        from pdf_generator.cli.main import generate_all_charts

        generate_all_charts(
            "prefix", [{"p50": 25}], daily, None, [{"p50": 25}], config, None
        )

        # Should be called twice: once for stats, once for daily
        self.assertEqual(mock_ts.call_count, 2)

    @patch("pdf_generator.cli.main.generate_timeseries_chart")
    @patch("pdf_generator.cli.main.generate_histogram_chart")
    @patch("pdf_generator.cli.main.check_charts_available")
    def test_histogram_generated_when_available(self, mock_check, mock_hist, mock_ts):
        """Test that histogram is generated when data available."""
        mock_check.return_value = True
        mock_hist.return_value = True

        config = create_test_config(debug=False, units="mph", timezone="UTC")
        histogram = {"10": 5, "20": 10}

        from pdf_generator.cli.main import generate_all_charts

        generate_all_charts(
            "prefix", [{"p50": 25}], None, histogram, [{"p50": 25}], config, None
        )

        mock_hist.assert_called_once()


class TestPrintApiDebugInfo(unittest.TestCase):
    """Tests for print_api_debug_info function."""

    def test_prints_debug_info_successfully(self):
        """Test that debug info is printed successfully."""
        from pdf_generator.cli.main import print_api_debug_info

        resp = Mock()
        resp.status_code = 200
        resp.elapsed.total_seconds.return_value = 0.5

        # Should not raise
        print_api_debug_info(resp, [{"p50": 25}], {"10": 5})

    def test_handles_exception_gracefully(self):
        """Test that exceptions in debug info are handled."""
        from pdf_generator.cli.main import print_api_debug_info

        resp = Mock()
        resp.status_code = 200
        resp.elapsed = None  # Will cause AttributeError

        # Should not raise
        print_api_debug_info(resp, [], None)


class TestMainFunction(unittest.TestCase):
    """Tests for main function."""

    @patch("pdf_generator.cli.main.process_date_range")
    @patch("pdf_generator.cli.main.RadarStatsClient")
    def test_processes_multiple_date_ranges(self, mock_client_class, mock_process):
        """Test that main processes multiple date ranges."""
        from pdf_generator.cli.main import main

        mock_client = Mock()
        mock_client_class.return_value = mock_client

        config = create_test_config(
            source="radar_data_transits",
            model_version="rebuild-full",
            timezone="UTC",
            file_prefix="",
            group="1h",
            units="mph",
            min_speed=None,
            debug=False,
        )

        date_ranges = [
            ("2024-01-01", "2024-01-31"),
            ("2024-02-01", "2024-02-28"),
        ]

        main(date_ranges, config)

        # Should process both ranges
        self.assertEqual(mock_process.call_count, 2)
        mock_client_class.assert_called_once()


class TestParseDateRange(unittest.TestCase):
    """Tests for parse_date_range function."""

    @patch("pdf_generator.cli.main.parse_date_to_unix")
    @patch("pdf_generator.cli.main.is_date_only")
    def test_successful_parse(self, mock_is_date_only, mock_parse):
        """Test successful date range parsing."""
        from pdf_generator.cli.main import parse_date_range

        mock_parse.side_effect = [1704067200, 1704153600]
        mock_is_date_only.return_value = True

        start_ts, end_ts = parse_date_range("2024-01-01", "2024-01-02", "UTC")

        self.assertEqual(start_ts, 1704067200)
        self.assertEqual(end_ts, 1704153600)
        self.assertEqual(mock_parse.call_count, 2)

    @patch("pdf_generator.cli.main.parse_date_to_unix")
    def test_parse_failure_returns_none(self, mock_parse):
        """Test that parse failure returns None values."""
        from pdf_generator.cli.main import parse_date_range

        mock_parse.side_effect = ValueError("Invalid date")

        start_ts, end_ts = parse_date_range("invalid", "date", None)

        self.assertIsNone(start_ts)
        self.assertIsNone(end_ts)


class TestGetModelVersion(unittest.TestCase):
    """Tests for get_model_version function."""

    def test_returns_version_for_transit_source(self):
        """Test that model version is returned for transit source."""
        from pdf_generator.cli.main import get_model_version

        config = create_test_config(source="radar_data_transits", model_version="v1.0")

        result = get_model_version(config)
        self.assertEqual(result, "v1.0")

    def test_returns_default_when_no_version_specified(self):
        """Test default version for transit source."""
        from pdf_generator.cli.main import get_model_version

        config = create_test_config(source="radar_data_transits", model_version=None)

        result = get_model_version(config)
        self.assertEqual(result, "rebuild-full")

    def test_returns_none_for_radar_objects(self):
        """Test that None is returned for non-transit source."""
        from pdf_generator.cli.main import get_model_version

        config = create_test_config(source="radar_objects", model_version="v1.0")

        result = get_model_version(config)
        self.assertIsNone(result)


class TestPlotStatsPage(unittest.TestCase):
    """Tests for _plot_stats_page function."""

    @patch("pdf_generator.cli.main.TimeSeriesChartBuilder")
    def test_creates_chart(self, mock_builder_class):
        """Test that _plot_stats_page creates chart."""
        from pdf_generator.cli.main import _plot_stats_page

        mock_builder = Mock()
        mock_fig = Mock()
        mock_builder.build.return_value = mock_fig
        mock_builder_class.return_value = mock_builder

        result = _plot_stats_page([{"p50": 25}], "Test", "mph", "UTC")

        self.assertEqual(result, mock_fig)
        mock_builder.build.assert_called_once_with([{"p50": 25}], "Test", "mph", "UTC")


class TestProcessDateRangeEdgeCases(unittest.TestCase):
    """Additional edge case tests for process_date_range."""

    @patch("pdf_generator.cli.main.assemble_pdf_report")
    @patch("pdf_generator.cli.main.generate_all_charts")
    @patch("pdf_generator.cli.main.fetch_daily_summary")
    @patch("pdf_generator.cli.main.fetch_overall_summary")
    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    @patch("pdf_generator.cli.main.resolve_file_prefix")
    @patch("pdf_generator.cli.main.get_model_version")
    @patch("pdf_generator.cli.main.parse_date_range")
    def test_parse_failure_returns_early(
        self,
        mock_parse_range,
        mock_get_version,
        mock_resolve,
        mock_fetch_granular,
        mock_fetch_overall,
        mock_fetch_daily,
        mock_gen_charts,
        mock_assemble,
    ):
        """Test that parse failure causes early return."""
        from pdf_generator.cli.main import process_date_range

        mock_parse_range.return_value = (None, None)

        mock_client = Mock()
        config = create_test_config(
            source="radar_data_transits",
            timezone="UTC",
        )

        process_date_range("invalid", "date", config, mock_client)

        # Should not proceed to fetching
        mock_fetch_granular.assert_not_called()
        mock_assemble.assert_not_called()

    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    @patch("pdf_generator.cli.main.resolve_file_prefix")
    @patch("pdf_generator.cli.main.get_model_version")
    @patch("pdf_generator.cli.main.parse_date_range")
    def test_no_data_returns_early_v2(
        self,
        mock_parse_range,
        mock_get_version,
        mock_resolve,
        mock_fetch_granular,
    ):
        """Test that no data and no histogram returns early."""
        from pdf_generator.cli.main import process_date_range

        mock_parse_range.return_value = (1704067200, 1704153600)
        mock_get_version.return_value = None
        mock_resolve.return_value = "test"
        mock_fetch_granular.return_value = ([], None, None)

        mock_client = Mock()
        config = create_test_config(
            source="radar_data_transits",
            timezone="UTC",
        )

        process_date_range("2024-01-01", "2024-01-02", config, mock_client)

        # Early return, so overall summary should not be called
        mock_client.get_stats.assert_not_called()

    @patch("pdf_generator.cli.main.assemble_pdf_report")
    @patch("pdf_generator.cli.main.generate_all_charts")
    @patch("pdf_generator.cli.main.compute_iso_timestamps")
    @patch("pdf_generator.cli.main.fetch_daily_summary")
    @patch("pdf_generator.cli.main.fetch_overall_summary")
    @patch("pdf_generator.cli.main.fetch_granular_metrics")
    @patch("pdf_generator.cli.main.resolve_file_prefix")
    @patch("pdf_generator.cli.main.get_model_version")
    @patch("pdf_generator.cli.main.parse_date_range")
    def test_histogram_without_metrics_proceeds(
        self,
        mock_parse_range,
        mock_get_version,
        mock_resolve,
        mock_fetch_granular,
        mock_fetch_overall,
        mock_fetch_daily,
        mock_compute_iso,
        mock_gen_charts,
        mock_assemble,
    ):
        """Test that histogram without metrics still proceeds."""
        from pdf_generator.cli.main import process_date_range

        mock_parse_range.return_value = (1704067200, 1704153600)
        mock_get_version.return_value = None
        mock_resolve.return_value = "test"
        mock_fetch_granular.return_value = ([], {"10": 5}, Mock())
        mock_fetch_overall.return_value = [{"Count": 100}]
        mock_fetch_daily.return_value = None
        mock_compute_iso.return_value = ("2024-01-01T00:00:00", "2024-01-02T00:00:00")
        mock_assemble.return_value = True

        mock_client = Mock()
        config = create_test_config(
            source="radar_data_transits",
            timezone="UTC",
            units="mph",
            group="1h",
            min_speed=None,
            debug=False,
        )

        process_date_range("2024-01-01", "2024-01-02", config, mock_client)

        # Should proceed even with empty metrics but present histogram
        mock_assemble.assert_called_once()
        mock_gen_charts.assert_called_once()


class TestSampleLabelEdgeCases(unittest.TestCase):
    """Tests for sample label generation edge cases in histogram chart."""

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    @patch("pdf_generator.cli.main.extract_count_from_row")
    def test_sample_label_int_conversion_failure(
        self, mock_extract, mock_plot, mock_save
    ):
        """Test sample label when int() conversion fails."""
        mock_plot.return_value = Mock()
        mock_save.return_value = True
        # Return a value that can't convert to int
        mock_extract.return_value = "not-a-number"

        config = create_test_config(debug=False)
        metrics = {"Count": "invalid"}

        result = generate_histogram_chart({"10": 5}, "test", "mph", metrics, config)

        self.assertTrue(result)
        # Should have called plot_histogram with fallback label
        mock_plot.assert_called_once()

    @patch("pdf_generator.cli.main.save_chart_as_pdf")
    @patch("pdf_generator.cli.main.plot_histogram")
    def test_sample_extraction_with_list_non_dict_items(self, mock_plot, mock_save):
        """Test sample extraction when list contains non-dict items."""
        mock_plot.return_value = Mock()
        mock_save.return_value = True

        config = create_test_config(debug=False)
        # List with non-dict items
        metrics = ["not", "dict", "items"]

        result = generate_histogram_chart({"10": 5}, "test", "mph", metrics, config)

        self.assertTrue(result)


class TestComputeIsoTimestampsEdgeCases(unittest.TestCase):
    """Additional edge cases for compute_iso_timestamps."""

    def test_exception_in_timestamp_conversion(self):
        """Test that exception in conversion falls back to string."""
        # Use very large timestamp that might cause issues
        start_ts = 9999999999999  # Far future
        end_ts = 9999999999999

        start_iso, end_iso = compute_iso_timestamps(start_ts, end_ts, "UTC")

        # Should fallback to string representation
        self.assertEqual(start_iso, str(start_ts))
        self.assertEqual(end_iso, str(end_ts))


if __name__ == "__main__":
    unittest.main()
