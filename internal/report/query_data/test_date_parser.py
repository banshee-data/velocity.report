"""Tests for date_parser module."""

import pytest
from datetime import datetime, timezone
from zoneinfo import ZoneInfo

from .date_parser import parse_date_to_unix, parse_server_time, is_date_only


class TestParseDateToUnix:
    """Tests for parse_date_to_unix function."""

    def test_parse_unix_timestamp_int(self):
        """Test parsing an integer unix timestamp."""
        ts = 1717545600  # 2024-06-05 00:00:00 UTC
        result = parse_date_to_unix(ts)
        assert result == ts

    def test_parse_unix_timestamp_string(self):
        """Test parsing a string unix timestamp."""
        result = parse_date_to_unix("1717545600")
        assert result == 1717545600

    def test_parse_date_only_utc(self):
        """Test parsing YYYY-MM-DD in UTC."""
        result = parse_date_to_unix("2024-06-05")
        expected = datetime(2024, 6, 5, 0, 0, 0, tzinfo=timezone.utc)
        assert result == int(expected.timestamp())

    def test_parse_date_only_with_timezone(self):
        """Test parsing YYYY-MM-DD with specific timezone."""
        result = parse_date_to_unix("2024-06-05", tz_name="US/Pacific")
        tz = ZoneInfo("US/Pacific")
        expected = datetime(2024, 6, 5, 0, 0, 0, tzinfo=tz)
        assert result == int(expected.timestamp())

    def test_parse_date_end_of_day(self):
        """Test parsing YYYY-MM-DD as end of day."""
        result = parse_date_to_unix("2024-06-05", end_of_day=True)
        expected = datetime(2024, 6, 5, 23, 59, 59, tzinfo=timezone.utc)
        assert result == int(expected.timestamp())

    def test_parse_iso_datetime_with_z(self):
        """Test parsing ISO datetime with Z suffix."""
        result = parse_date_to_unix("2024-06-05T12:30:00Z")
        expected = datetime(2024, 6, 5, 12, 30, 0, tzinfo=timezone.utc)
        assert result == int(expected.timestamp())

    def test_parse_iso_datetime_with_offset(self):
        """Test parsing ISO datetime with timezone offset."""
        result = parse_date_to_unix("2024-06-05T12:30:00-07:00")
        expected = datetime(2024, 6, 5, 19, 30, 0, tzinfo=timezone.utc)
        assert result == int(expected.timestamp())

    def test_parse_iso_datetime_naive_with_tz(self):
        """Test parsing naive ISO datetime with timezone parameter."""
        result = parse_date_to_unix("2024-06-05T12:30:00", tz_name="US/Pacific")
        tz = ZoneInfo("US/Pacific")
        expected = datetime(2024, 6, 5, 12, 30, 0, tzinfo=tz)
        assert result == int(expected.timestamp())

    def test_invalid_date_format(self):
        """Test that invalid date format raises ValueError."""
        with pytest.raises(ValueError, match="Invalid date format"):
            parse_date_to_unix("not-a-date")

    def test_unknown_timezone(self):
        """Test that unknown timezone raises ValueError."""
        with pytest.raises(ValueError, match="unknown timezone"):
            parse_date_to_unix("2024-06-05", tz_name="Invalid/Timezone")


class TestParseServerTime:
    """Tests for parse_server_time function."""

    def test_parse_unix_timestamp(self):
        """Test parsing unix timestamp."""
        result = parse_server_time(1717545600)
        expected = datetime(2024, 6, 5, 0, 0, 0, tzinfo=timezone.utc)
        assert result == expected

    def test_parse_rfc3339_with_z(self):
        """Test parsing RFC3339 string with Z."""
        result = parse_server_time("2024-06-05T12:30:00Z")
        expected = datetime(2024, 6, 5, 12, 30, 0, tzinfo=timezone.utc)
        assert result == expected

    def test_parse_iso_with_offset(self):
        """Test parsing ISO string with offset."""
        result = parse_server_time("2024-06-05T12:30:00-07:00")
        # Result should preserve the offset
        assert result.year == 2024
        assert result.month == 6
        assert result.day == 5
        assert result.hour == 12
        assert result.minute == 30

    def test_invalid_type(self):
        """Test that invalid type raises ValueError."""
        with pytest.raises(ValueError, match="unsupported time format"):
            parse_server_time({"invalid": "object"})


class TestIsDateOnly:
    """Tests for is_date_only function."""

    def test_valid_date_only(self):
        """Test valid YYYY-MM-DD format."""
        assert is_date_only("2024-06-05") is True

    def test_date_with_time(self):
        """Test date with time is not date-only."""
        assert is_date_only("2024-06-05T12:30:00") is False

    def test_invalid_format(self):
        """Test invalid format is not date-only."""
        assert is_date_only("06/05/2024") is False

    def test_with_whitespace(self):
        """Test date with whitespace is still recognized."""
        assert is_date_only("  2024-06-05  ") is True

    def test_non_string(self):
        """Test non-string returns False."""
        assert is_date_only(123) is False
