"""Tests for api_client module."""

import pytest
import responses
from datetime import datetime, timezone

from pdf_generator.core.api_client import RadarStatsClient, SUPPORTED_GROUPS


class TestRadarStatsClient:
    """Tests for RadarStatsClient class."""

    def test_init_default_url(self):
        """Test client initialization with default URL."""
        client = RadarStatsClient()
        assert client.base_url == "http://localhost:8080"
        assert client.api_url == "http://localhost:8080/api/radar_stats"

    def test_init_custom_url(self):
        """Test client initialization with custom URL."""
        client = RadarStatsClient(base_url="http://example.com:9000")
        assert client.base_url == "http://example.com:9000"
        assert client.api_url == "http://example.com:9000/api/radar_stats"

    def test_init_url_trailing_slash(self):
        """Test that trailing slash is removed from base URL."""
        client = RadarStatsClient(base_url="http://example.com/")
        assert client.base_url == "http://example.com"

    @responses.activate
    def test_get_stats_basic(self):
        """Test basic stats query."""
        responses.add(
            responses.GET,
            "http://localhost:8080/api/radar_stats",
            json={
                "metrics": [
                    {
                        "StartTime": "2024-06-05T00:00:00Z",
                        "Count": 100,
                        "P50Speed": 25.5,
                        "P85Speed": 30.2,
                        "P98Speed": 35.8,
                        "MaxSpeed": 42.1,
                    }
                ],
                "histogram": {},
            },
            status=200,
        )

        client = RadarStatsClient()
        metrics, histogram, resp = client.get_stats(
            start_ts=1717545600, end_ts=1717632000, group="1h", units="mph"
        )

        assert len(metrics) == 1
        assert metrics[0]["Count"] == 100
        assert metrics[0]["P50Speed"] == 25.5
        assert histogram == {}
        assert resp.status_code == 200
        assert "model_version" not in responses.calls[0].request.url

    @responses.activate
    def test_get_stats_with_histogram(self):
        """Test stats query with histogram data."""
        responses.add(
            responses.GET,
            "http://localhost:8080/api/radar_stats",
            json={
                "metrics": [],
                "histogram": {
                    "0.0": 5,
                    "5.0": 10,
                    "10.0": 15,
                },
            },
            status=200,
        )

        client = RadarStatsClient()
        metrics, histogram, resp = client.get_stats(
            start_ts=1717545600,
            end_ts=1717632000,
            compute_histogram=True,
            hist_bucket_size=5.0,
        )

        assert len(metrics) == 0
        assert len(histogram) == 3
        assert histogram["5.0"] == 10

    @responses.activate
    def test_get_stats_with_all_params(self):
        """Test stats query with all parameters."""
        responses.add(
            responses.GET,
            "http://localhost:8080/api/radar_stats",
            json={"metrics": [], "histogram": {}},
            status=200,
        )

        client = RadarStatsClient()
        client.get_stats(
            start_ts=1717545600,
            end_ts=1717632000,
            group="24h",
            units="kph",
            source="radar_data_transits",
            model_version="custom-version",
            timezone="US/Pacific",
            min_speed=5.0,
            compute_histogram=True,
            hist_bucket_size=10.0,
            hist_max=100.0,
        )

        # Verify request was made with correct parameters
        assert len(responses.calls) == 1
        request = responses.calls[0].request
        assert "start=1717545600" in request.url
        assert "end=1717632000" in request.url
        assert "group=24h" in request.url
        assert "units=kph" in request.url
        assert "source=radar_data_transits" in request.url
        assert "model_version=custom-version" in request.url
        assert "timezone=US%2FPacific" in request.url
        assert "min_speed=5.0" in request.url
        assert "compute_histogram=true" in request.url
        assert "hist_bucket_size=10.0" in request.url
        assert "hist_max=100.0" in request.url

    @responses.activate
    def test_get_stats_http_error(self):
        """Test that HTTP errors are raised."""
        responses.add(
            responses.GET,
            "http://localhost:8080/api/radar_stats",
            json={"error": "Bad Request"},
            status=400,
        )

        client = RadarStatsClient()
        with pytest.raises(Exception):  # requests.HTTPError
            client.get_stats(start_ts=1717545600, end_ts=1717632000)

    @responses.activate
    def test_get_stats_legacy_format(self):
        """Test handling of legacy API response format (plain array)."""
        responses.add(
            responses.GET,
            "http://localhost:8080/api/radar_stats",
            json=[
                {
                    "StartTime": "2024-06-05T00:00:00Z",
                    "Count": 50,
                    "P50Speed": 20.0,
                }
            ],
            status=200,
        )

        client = RadarStatsClient()
        metrics, histogram, resp = client.get_stats(
            start_ts=1717545600, end_ts=1717632000
        )

        assert len(metrics) == 1
        assert metrics[0]["Count"] == 50
        assert histogram == {}


class TestSupportedGroups:
    """Tests for SUPPORTED_GROUPS constant."""

    def test_supported_groups_values(self):
        """Test that supported groups have correct second values."""
        assert SUPPORTED_GROUPS["15m"] == 15 * 60
        assert SUPPORTED_GROUPS["30m"] == 30 * 60
        assert SUPPORTED_GROUPS["1h"] == 60 * 60
        assert SUPPORTED_GROUPS["2h"] == 2 * 60 * 60
        assert SUPPORTED_GROUPS["24h"] == 24 * 60 * 60

    def test_supported_groups_completeness(self):
        """Test that all expected groups are present."""
        expected_groups = [
            "15m",
            "30m",
            "1h",
            "2h",
            "3h",
            "4h",
            "6h",
            "8h",
            "12h",
            "24h",
        ]
        for group in expected_groups:
            assert group in SUPPORTED_GROUPS
