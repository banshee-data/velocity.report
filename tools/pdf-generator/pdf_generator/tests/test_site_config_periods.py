"""Tests for site configuration period formatting."""

from pdf_generator.core.pdf_generator import _format_site_config_periods


def test_format_site_config_periods_with_notes():
    periods = [
        {
            "effective_start_unix": 0,
            "effective_end_unix": None,
            "cosine_error_angle": 12.5,
            "notes": "Initial configuration",
        }
    ]

    result = _format_site_config_periods(periods, "UTC")

    assert result == [
        {
            "key": "Period 1",
            "value": "Initial to Present • 12.5° (Initial configuration)",
        }
    ]


def test_format_site_config_periods_without_notes():
    periods = [
        {
            "effective_start_unix": 1700000000,
            "effective_end_unix": 1700003600,
            "cosine_error_angle": 5,
        }
    ]

    result = _format_site_config_periods(periods, "UTC")

    assert result == [
        {
            "key": "Period 1",
            "value": "2023-11-14 to 2023-11-14 • 5°",
        }
    ]
