#!/usr/bin/env python3
"""Tests for max_bucket parameter in histogram generation."""

import unittest
from unittest.mock import MagicMock, patch

from pdf_generator.core.chart_builder import HistogramChartBuilder
from pdf_generator.core.table_builders import HistogramTableBuilder


class TestMaxBucketHistogramChart(unittest.TestCase):
    """Test histogram chart respects max_bucket parameter."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = HistogramChartBuilder()
        # Histogram with data from 0 to 100 in steps of 5
        self.histogram = {str(i): 10 for i in range(0, 105, 5)}

    def test_format_labels_without_max_bucket(self):
        """Test label formatting without max_bucket (default behavior)."""
        labels = ["0", "5", "10", "15", "20"]
        formatted = self.builder._format_labels(labels, max_bucket=None)

        # Without max_bucket, last bucket should be "20+"
        self.assertEqual(formatted[0], "0-5")
        self.assertEqual(formatted[1], "5-10")
        self.assertEqual(formatted[2], "10-15")
        self.assertEqual(formatted[3], "15-20")
        self.assertEqual(formatted[4], "20+")

    def test_format_labels_with_max_bucket_at_75(self):
        """Test label formatting with max_bucket=75."""
        labels = [
            "0",
            "5",
            "10",
            "15",
            "20",
            "25",
            "30",
            "35",
            "40",
            "45",
            "50",
            "55",
            "60",
            "65",
            "70",
            "75",
        ]
        formatted = self.builder._format_labels(labels, max_bucket=75.0)

        # With max_bucket=75, we should see ranges up to 70-75, then 75+
        self.assertEqual(formatted[0], "0-5")
        self.assertEqual(formatted[-2], "70-75")  # Second to last
        self.assertEqual(formatted[-1], "75+")  # Last bucket at max_bucket value

    def test_format_labels_with_max_bucket_cutoff(self):
        """Test that max_bucket properly cuts off at the specified value."""
        labels = ["65", "70", "75"]
        formatted = self.builder._format_labels(labels, max_bucket=75.0)

        # 65-70, 70-75, 75+
        self.assertEqual(formatted[0], "65-70")
        self.assertEqual(formatted[1], "70-75")
        self.assertEqual(formatted[2], "75+")

    def test_format_labels_without_max_bucket_shows_last_as_plus(self):
        """Test that without max_bucket, last bucket is always N+."""
        labels = ["65", "70"]
        formatted = self.builder._format_labels(labels, max_bucket=None)

        # Without max_bucket, last should be 70+
        self.assertEqual(formatted[0], "65-70")
        self.assertEqual(formatted[1], "70+")

    def test_format_labels_with_data_beyond_max_bucket(self):
        """Test behavior when data exists beyond max_bucket."""
        # This simulates receiving histogram data that extends past the cutoff
        labels = ["65", "70", "75", "80", "85"]
        formatted = self.builder._format_labels(labels, max_bucket=75.0)

        # With max_bucket=75, the 75 bucket should be "75+"
        # and buckets beyond should be regular ranges (though they shouldn't appear in practice)
        self.assertEqual(formatted[0], "65-70")
        self.assertEqual(formatted[1], "70-75")
        self.assertEqual(formatted[2], "75+")  # max_bucket cutoff
        self.assertEqual(formatted[3], "80-85")  # Beyond cutoff
        self.assertEqual(formatted[4], "85+")  # Last bucket

    @patch("pdf_generator.core.chart_builder.plt")
    def test_build_with_max_bucket_parameter(self, mock_plt):
        """Test that build() accepts and uses max_bucket parameter."""
        mock_fig = MagicMock()
        mock_ax = MagicMock()
        mock_plt.subplots.return_value = (mock_fig, mock_ax)

        # Build histogram with max_bucket
        histogram = {"65": 10, "70": 10, "75": 30}
        result = self.builder.build(
            histogram=histogram, title="Test", units="mph", debug=False, max_bucket=75.0
        )

        # Verify it created a figure
        self.assertIsNotNone(result)
        mock_plt.subplots.assert_called_once()


class TestMaxBucketHistogramTable(unittest.TestCase):
    """Test histogram table respects max_bucket parameter."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = HistogramTableBuilder()

    @patch("pdf_generator.core.table_builders.count_histogram_ge")
    @patch("pdf_generator.core.table_builders.count_in_histogram_range")
    @patch("pdf_generator.core.table_builders.process_histogram")
    @patch("pdf_generator.core.table_builders.Center")
    @patch("pdf_generator.core.table_builders.Tabular")
    def test_table_with_max_bucket_shows_proper_buckets(
        self, mock_tabular, mock_center, mock_process, mock_range, mock_ge
    ):
        """Test that table generation creates proper buckets with max_bucket=75."""
        # Setup mocks
        numeric_buckets = {float(i): 10 for i in range(0, 105, 5)}
        mock_process.return_value = (
            numeric_buckets,
            210,  # total
            [(5.0, 10.0), (10.0, 15.0)],  # fallback ranges
        )

        # Mock count functions to return reasonable values
        mock_range.return_value = 10  # Each bucket has 10 items
        mock_ge.return_value = 60  # Items >= 75

        mock_table = MagicMock()
        mock_tabular.return_value = mock_table
        mock_centered = MagicMock()
        mock_center.return_value = mock_centered

        # Build table with max_bucket=75
        histogram = {str(i): 10 for i in range(0, 105, 5)}
        self.builder.build(
            histogram=histogram,
            units="mph",
            cutoff=5.0,
            bucket_size=5.0,
            max_bucket=75.0,
        )

        # Check that rows were added
        self.assertTrue(mock_table.add_row.called)

        # Get all the row calls
        row_calls = [call[0][0] for call in mock_table.add_row.call_args_list]

        # Convert NoEscape objects to strings for easier testing
        row_strings = []
        for row in row_calls:
            row_str = []
            for cell in row:
                # Extract the string from NoEscape objects
                cell_str = str(cell)
                if hasattr(cell, "data"):
                    cell_str = cell.data
                row_str.append(cell_str)
            row_strings.append(row_str)

        # Find the bucket column (first column)
        bucket_labels = [row[0] for row in row_strings if len(row) > 0]

        # Filter out header rows
        bucket_labels = [
            label
            for label in bucket_labels
            if not any(x in str(label) for x in ["Bucket", "multicolumn", "sffamily"])
        ]

        # We should see buckets like: 0-5, 5-10, ..., 70-75, 75+
        # The last regular bucket should be 70-75 (not 70+)
        # And there should be a 75+ bucket
        has_70_75 = any(
            "70{-}75" in str(label) or "70-75" in str(label) for label in bucket_labels
        )
        has_75_plus = any("75+" in str(label) for label in bucket_labels)
        has_70_plus_only = any(
            "70+" in str(label)
            and "70-75" not in str(label)
            and "70{-}75" not in str(label)
            for label in bucket_labels
        )

        self.assertTrue(
            has_70_75, f"Should have 70-75 bucket. Got labels: {bucket_labels}"
        )
        self.assertTrue(
            has_75_plus, f"Should have 75+ bucket. Got labels: {bucket_labels}"
        )
        self.assertFalse(
            has_70_plus_only,
            f"Should NOT have 70+ bucket (should be 70-75 and 75+). Got labels: {bucket_labels}",
        )


class TestMaxBucketIntegration(unittest.TestCase):
    """Integration tests for max_bucket across table and chart generation."""

    def test_histogram_data_structure_with_max_bucket(self):
        """Test that histogram data structure is correct when max_bucket is set."""
        # When max_bucket=75, the backend should return histogram with:
        # - Regular buckets: 0, 5, 10, ..., 65, 70
        # - Cutoff bucket: 75 (containing all data >= 75)

        # Simulate backend data
        histogram = {
            "0": 10,
            "5": 10,
            "10": 10,
            "15": 10,
            "20": 10,
            "25": 10,
            "30": 10,
            "35": 10,
            "40": 10,
            "45": 10,
            "50": 10,
            "55": 10,
            "60": 10,
            "65": 10,
            "70": 10,
            "75": 60,  # This should contain all speeds >= 75
        }

        # Build chart
        chart_builder = HistogramChartBuilder()
        labels = sorted(histogram.keys(), key=lambda x: float(x))
        formatted = chart_builder._format_labels(labels, max_bucket=75.0)

        # Check labels
        self.assertIn("70-75", formatted)
        self.assertIn("75+", formatted)
        # Make sure 70+ is NOT in the labels
        self.assertNotIn("70+", [label for label in formatted if label != "70-75"])


class TestSharedHistogramFunctions(unittest.TestCase):
    """Tests for shared histogram formatting functions (DRY principle)."""

    def test_format_histogram_labels_standalone(self):
        """Test that the shared format_histogram_labels function works correctly."""
        from pdf_generator.core.stats_utils import format_histogram_labels

        labels = ["10", "20", "30", "40", "50", "60", "70", "75"]
        formatted = format_histogram_labels(labels, max_bucket=75.0)

        # Should cap the 70 bucket at 75 and show 75+
        self.assertEqual(formatted[-2], "70-75")
        self.assertEqual(formatted[-1], "75+")

    def test_compute_histogram_ranges_with_max_bucket(self):
        """Test that compute_histogram_ranges caps the last range at max_bucket."""
        from pdf_generator.core.stats_utils import compute_histogram_ranges

        # Bucket size inferred as 5 from the data
        numeric_buckets = {5: 1, 10: 1, 15: 1, 20: 1, 70: 1, 75: 3}
        ranges = compute_histogram_ranges(
            numeric_buckets, bucket_size=5.0, max_bucket=75.0
        )

        # The last range should end at 75, not 80
        last_range = ranges[-1]
        self.assertEqual(last_range[1], 75.0)
        # The range should be (70, 75) not (70, 80)
        self.assertEqual(last_range, (70.0, 75.0))

    def test_chart_and_table_use_same_logic(self):
        """Test that chart's _format_labels delegates to shared function."""
        from pdf_generator.core.stats_utils import format_histogram_labels

        chart_builder = HistogramChartBuilder()

        labels = ["10", "20", "30", "70", "75"]

        # Chart method should produce same result as shared function
        chart_result = chart_builder._format_labels(labels, max_bucket=75.0)
        shared_result = format_histogram_labels(labels, max_bucket=75.0)

        self.assertEqual(chart_result, shared_result)


class TestComparisonTableMaxBucket(unittest.TestCase):
    """Tests for max_bucket in comparison histogram table."""

    def test_comparison_table_shows_correct_buckets_with_max_bucket(self):
        """Test that comparison table shows N-M and M+ buckets, not N+."""
        from unittest.mock import MagicMock, patch

        from pdf_generator.core.table_builders import create_histogram_comparison_table

        with (
            patch("pdf_generator.core.table_builders.Tabular") as mock_tabular,
            patch("pdf_generator.core.table_builders.Center") as mock_center,
            patch("pdf_generator.core.table_builders.NoEscape") as mock_noescape,
            patch("pdf_generator.core.table_builders.escape_latex") as mock_escape,
        ):
            mock_escape.side_effect = lambda x: x
            mock_noescape.side_effect = lambda x: x

            mock_table = MagicMock()
            mock_tabular.return_value = mock_table
            mock_centered = MagicMock()
            mock_center.return_value = mock_centered

            # Create histograms with data up to 80
            primary = {str(i): 10 for i in range(5, 85, 5)}
            compare = {str(i): 10 for i in range(5, 85, 5)}

            # Generate table with max_bucket=75
            create_histogram_comparison_table(
                histogram=primary,
                compare_histogram=compare,
                units="mph",
                primary_label="t1",
                compare_label="t2",
                cutoff=5.0,
                bucket_size=5.0,
                max_bucket=75.0,
            )

            # Get all the row calls
            row_calls = [call[0][0] for call in mock_table.add_row.call_args_list]

            # Extract bucket labels (first column of each row)
            bucket_labels = []
            for row in row_calls:
                if len(row) > 0:
                    label = str(row[0])
                    # Skip header rows
                    if "Bucket" not in label and "sffamily" not in label:
                        bucket_labels.append(label)

            # Should have 70-75 bucket (not 70+)
            has_70_75 = any("70-75" in label for label in bucket_labels)
            # Should have 75+ bucket
            has_75_plus = any("75+" in label for label in bucket_labels)
            # Should NOT have 70+ (standalone)
            has_70_plus_only = any(
                label == "70+" or (label.endswith("70+") and "70-75" not in label)
                for label in bucket_labels
            )

            self.assertTrue(
                has_70_75,
                f"Should have 70-75 bucket in comparison table. Got: {bucket_labels}",
            )
            self.assertTrue(
                has_75_plus,
                f"Should have 75+ bucket in comparison table. Got: {bucket_labels}",
            )
            self.assertFalse(
                has_70_plus_only,
                f"Should NOT have standalone 70+ bucket. Got: {bucket_labels}",
            )


if __name__ == "__main__":
    unittest.main()
