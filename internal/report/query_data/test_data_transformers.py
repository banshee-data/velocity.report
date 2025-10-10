#!/usr/bin/env python3
"""Unit tests for data_transformers module."""

import unittest
import numpy as np
from data_transformers import (
    MetricsNormalizer,
    FIELD_ALIASES,
    extract_metrics_from_row,
    extract_count_from_row,
    extract_start_time_from_row,
    normalize_metrics_list,
    extract_metrics_arrays,
)


class TestMetricsNormalizer(unittest.TestCase):
    """Test MetricsNormalizer class."""

    def setUp(self):
        """Set up test fixtures."""
        self.normalizer = MetricsNormalizer()

    def test_get_value_with_primary_name(self):
        """Test get_value with primary field name."""
        row = {"p50": 25.5}
        result = self.normalizer.get_value(row, "p50")
        self.assertEqual(result, 25.5)

    def test_get_value_with_alias(self):
        """Test get_value with alias field name."""
        row = {"P50Speed": 25.5}
        result = self.normalizer.get_value(row, "p50")
        self.assertEqual(result, 25.5)

    def test_get_value_with_multiple_aliases(self):
        """Test get_value tries aliases in order."""
        row = {"P50Speed": 25.5, "p50speed": 26.0}
        # Should return first match in alias list
        result = self.normalizer.get_value(row, "p50")
        self.assertIn(result, [25.5, 26.0])

    def test_get_value_missing_field(self):
        """Test get_value returns default for missing field."""
        row = {}
        result = self.normalizer.get_value(row, "p50", default=0.0)
        self.assertEqual(result, 0.0)

    def test_get_value_none_value(self):
        """Test get_value skips None values."""
        row = {"p50": None, "P50Speed": 25.5}
        result = self.normalizer.get_value(row, "p50")
        self.assertEqual(result, 25.5)

    def test_get_numeric_valid_float(self):
        """Test get_numeric with valid float value."""
        row = {"p50": 25.5}
        result = self.normalizer.get_numeric(row, "p50")
        self.assertEqual(result, 25.5)

    def test_get_numeric_string_number(self):
        """Test get_numeric converts string to float."""
        row = {"p50": "25.5"}
        result = self.normalizer.get_numeric(row, "p50")
        self.assertEqual(result, 25.5)

    def test_get_numeric_invalid_value(self):
        """Test get_numeric returns default for invalid value."""
        row = {"p50": "invalid"}
        result = self.normalizer.get_numeric(row, "p50", default=0.0)
        self.assertEqual(result, 0.0)

    def test_get_numeric_missing_field(self):
        """Test get_numeric returns NaN for missing field."""
        row = {}
        result = self.normalizer.get_numeric(row, "p50")
        self.assertTrue(np.isnan(result))

    def test_normalize_adds_standard_fields(self):
        """Test normalize adds normalized field names."""
        row = {"P50Speed": 25.5, "Count": 42}
        result = self.normalizer.normalize(row)

        # Original fields preserved
        self.assertEqual(result["P50Speed"], 25.5)
        self.assertEqual(result["Count"], 42)

        # Normalized fields added
        self.assertEqual(result["p50"], 25.5)
        self.assertEqual(result["count"], 42)

    def test_normalize_preserves_all_original_fields(self):
        """Test normalize preserves fields not in alias map."""
        row = {"P50Speed": 25.5, "custom_field": "value"}
        result = self.normalizer.normalize(row)

        self.assertEqual(result["custom_field"], "value")
        self.assertEqual(result["p50"], 25.5)

    def test_field_aliases_complete(self):
        """Test FIELD_ALIASES contains expected fields."""
        expected_fields = ["p50", "p85", "p98", "max_speed", "start_time", "count"]
        for field in expected_fields:
            self.assertIn(field, FIELD_ALIASES)

    def test_custom_aliases(self):
        """Test normalizer with custom aliases."""
        custom_aliases = {"speed": ["velocity", "speed"]}
        normalizer = MetricsNormalizer(aliases=custom_aliases)
        row = {"velocity": 50}

        result = normalizer.get_value(row, "speed")
        self.assertEqual(result, 50)


class TestHelperFunctions(unittest.TestCase):
    """Test helper functions."""

    def test_extract_metrics_from_row(self):
        """Test extract_metrics_from_row."""
        row = {
            "P50Speed": 25.5,
            "P85Speed": 30.0,
            "P98Speed": 35.0,
            "MaxSpeed": 40.0,
        }
        result = extract_metrics_from_row(row)

        self.assertEqual(result["p50"], 25.5)
        self.assertEqual(result["p85"], 30.0)
        self.assertEqual(result["p98"], 35.0)
        self.assertEqual(result["max_speed"], 40.0)

    def test_extract_metrics_from_row_missing_fields(self):
        """Test extract_metrics_from_row with missing fields."""
        row = {"P50Speed": 25.5}
        result = extract_metrics_from_row(row)

        self.assertEqual(result["p50"], 25.5)
        self.assertTrue(np.isnan(result["p85"]))
        self.assertTrue(np.isnan(result["p98"]))
        self.assertTrue(np.isnan(result["max_speed"]))

    def test_extract_count_from_row(self):
        """Test extract_count_from_row."""
        row = {"Count": 42}
        result = extract_count_from_row(row)
        self.assertEqual(result, 42)

    def test_extract_count_from_row_string(self):
        """Test extract_count_from_row converts string."""
        row = {"Count": "42"}
        result = extract_count_from_row(row)
        self.assertEqual(result, 42)

    def test_extract_count_from_row_missing(self):
        """Test extract_count_from_row returns 0 for missing."""
        row = {}
        result = extract_count_from_row(row)
        self.assertEqual(result, 0)

    def test_extract_count_from_row_invalid(self):
        """Test extract_count_from_row returns 0 for invalid."""
        row = {"Count": "invalid"}
        result = extract_count_from_row(row)
        self.assertEqual(result, 0)

    def test_extract_start_time_from_row(self):
        """Test extract_start_time_from_row."""
        row = {"StartTime": "2024-01-01T00:00:00Z"}
        result = extract_start_time_from_row(row)
        self.assertEqual(result, "2024-01-01T00:00:00Z")

    def test_extract_start_time_from_row_alias(self):
        """Test extract_start_time_from_row with alias."""
        row = {"start_time": "2024-01-01T00:00:00Z"}
        result = extract_start_time_from_row(row)
        self.assertEqual(result, "2024-01-01T00:00:00Z")

    def test_extract_start_time_from_row_missing(self):
        """Test extract_start_time_from_row returns None for missing."""
        row = {}
        result = extract_start_time_from_row(row)
        self.assertIsNone(result)


class TestBatchProcessing(unittest.TestCase):
    """Test batch processing functions."""

    def test_normalize_metrics_list(self):
        """Test normalize_metrics_list."""
        metrics = [
            {"P50Speed": 25.5, "Count": 42},
            {"P50Speed": 30.0, "Count": 50},
        ]
        result = normalize_metrics_list(metrics)

        self.assertEqual(len(result), 2)
        self.assertEqual(result[0]["p50"], 25.5)
        self.assertEqual(result[0]["count"], 42)
        self.assertEqual(result[1]["p50"], 30.0)
        self.assertEqual(result[1]["count"], 50)

    def test_extract_metrics_arrays(self):
        """Test extract_metrics_arrays."""
        metrics = [
            {"P50Speed": 25.5, "P85Speed": 30.0, "P98Speed": 35.0, "MaxSpeed": 40.0},
            {"P50Speed": 26.0, "P85Speed": 31.0, "P98Speed": 36.0, "MaxSpeed": 41.0},
        ]
        result = extract_metrics_arrays(metrics)

        self.assertEqual(result["p50"], [25.5, 26.0])
        self.assertEqual(result["p85"], [30.0, 31.0])
        self.assertEqual(result["p98"], [35.0, 36.0])
        self.assertEqual(result["max_speed"], [40.0, 41.0])

    def test_extract_metrics_arrays_with_missing(self):
        """Test extract_metrics_arrays with missing values."""
        metrics = [
            {"P50Speed": 25.5},
            {"P85Speed": 30.0},
        ]
        result = extract_metrics_arrays(metrics)

        self.assertEqual(result["p50"][0], 25.5)
        self.assertTrue(np.isnan(result["p50"][1]))
        self.assertTrue(np.isnan(result["p85"][0]))
        self.assertEqual(result["p85"][1], 30.0)


if __name__ == "__main__":
    unittest.main()
