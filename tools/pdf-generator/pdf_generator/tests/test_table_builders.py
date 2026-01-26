#!/usr/bin/env python3
"""Unit tests for table_builders module."""

import unittest
from unittest.mock import MagicMock, patch

# Import builders
from pdf_generator.core.table_builders import (
    StatsTableBuilder,
    ParameterTableBuilder,
    HistogramTableBuilder,
    create_stats_table,
    create_param_table,
    create_comparison_summary_table,
    create_histogram_table,
    create_histogram_comparison_table,
    create_twocolumn_stats_table,
)


class TestStatsTableBuilder(unittest.TestCase):
    """Tests for StatsTableBuilder class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = StatsTableBuilder()
        self.sample_stats = [
            {
                "start_time": "2024-01-01T00:00:00",
                "count": 100,
                "p50": 25.5,
                "p85": 32.0,
                "p98": 38.5,
                "max_speed": 42.0,
            },
            {
                "start_time": "2024-01-01T01:00:00",
                "count": 150,
                "p50": 28.0,
                "p85": 35.0,
                "p98": 40.0,
                "max_speed": 45.0,
            },
        ]

    def test_initialization(self):
        """Test builder initialization."""
        self.assertIsNotNone(self.builder.normalizer)

    def test_build_header_with_start_time(self):
        """Test header building with start time column."""
        header = self.builder.build_header(include_start_time=True)
        self.assertEqual(len(header), 6)
        self.assertIn("Start Time", str(header[0]))
        self.assertIn("Count", str(header[1]))
        self.assertIn("p50", str(header[2]))
        self.assertIn("p85", str(header[3]))
        self.assertIn("p98", str(header[4]))
        self.assertIn("Max", str(header[5]))

    def test_build_header_without_start_time(self):
        """Test header building without start time column."""
        header = self.builder.build_header(include_start_time=False)
        self.assertEqual(len(header), 5)
        self.assertNotIn("Start Time", str(header[0]))
        self.assertIn("Count", str(header[0]))

    def test_build_rows_with_start_time(self):
        """Test building data rows with start time."""
        rows = self.builder.build_rows(
            self.sample_stats, include_start_time=True, tz_name="America/New_York"
        )
        self.assertEqual(len(rows), 2)
        # Each row should have 6 columns (start time + 5 metrics)
        self.assertEqual(len(rows[0]), 6)
        self.assertEqual(len(rows[1]), 6)

    def test_build_rows_without_start_time(self):
        """Test building data rows without start time."""
        rows = self.builder.build_rows(
            self.sample_stats, include_start_time=False, tz_name=None
        )
        self.assertEqual(len(rows), 2)
        # Each row should have 5 columns (just metrics)
        self.assertEqual(len(rows[0]), 5)
        self.assertEqual(len(rows[1]), 5)

    @patch("pdf_generator.core.table_builders.Tabular")
    @patch("pdf_generator.core.table_builders.Center")
    def test_build_centered(self, mock_center, mock_tabular):
        """Test building centered table."""
        mock_table = MagicMock()
        mock_tabular.return_value = mock_table
        mock_centered = MagicMock()
        mock_center.return_value = mock_centered

        _ = self.builder.build(
            self.sample_stats,
            tz_name="UTC",
            units="mph",
            caption="Test Caption",
            include_start_time=True,
            center_table=True,
        )

        # Should create Tabular
        mock_tabular.assert_called_once()
        # Should wrap in Center
        mock_center.assert_called_once()
        # Should add table to center
        mock_centered.append.assert_called()

    @patch("pdf_generator.core.table_builders.Tabular")
    def test_build_not_centered(self, mock_tabular):
        """Test building non-centered table."""
        mock_table = MagicMock()
        mock_tabular.return_value = mock_table

        result = self.builder.build(
            self.sample_stats,
            tz_name="UTC",
            units="mph",
            caption="Test Caption",
            include_start_time=True,
            center_table=False,
        )

        # Should create Tabular
        mock_tabular.assert_called_once()
        # Result should be the table itself, not wrapped
        self.assertEqual(result, mock_table)

    def test_build_twocolumn(self):
        """Test building two-column table."""
        mock_doc = MagicMock()

        self.builder.build_twocolumn(
            mock_doc,
            self.sample_stats,
            tz_name="UTC",
            units="mph",
            caption="Two Column Caption",
        )

        # Should append supertabular environment
        mock_doc.append.assert_called()
        # Check that multiple append calls were made (header, rows, hline, caption, etc.)
        self.assertGreater(
            mock_doc.append.call_count, 5, "Should append multiple table components"
        )


class TestParameterTableBuilder(unittest.TestCase):
    """Tests for ParameterTableBuilder class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = ParameterTableBuilder()
        self.sample_entries = [
            {"key": "Speed Limit", "value": "25 mph"},
            {"key": "Total Vehicles", "value": "1,234"},
            {"key": "Study Period", "value": "7 days"},
        ]

    def test_initialization(self):
        """Test builder initialization."""
        self.assertIsNotNone(self.builder)

    @patch("pdf_generator.core.table_builders.Tabular")
    def test_build(self, mock_tabular):
        """Test building parameter table."""
        mock_table = MagicMock()
        mock_tabular.return_value = mock_table

        _ = self.builder.build(self.sample_entries)

        # Should create table with "ll" spec (two left-aligned columns)
        mock_tabular.assert_called_once_with("ll")
        # Should add 3 rows
        self.assertEqual(mock_table.add_row.call_count, 3)

    @patch("pdf_generator.core.table_builders.Tabular")
    def test_build_empty_entries(self, mock_tabular):
        """Test building table with empty entries."""
        mock_table = MagicMock()
        mock_tabular.return_value = mock_table

        _ = self.builder.build([])

        # Should create table
        mock_tabular.assert_called_once()
        # Should not add any rows
        mock_table.add_row.assert_not_called()

    @patch("pdf_generator.core.table_builders.Tabular")
    @patch("pdf_generator.core.table_builders.NoEscape")
    def test_build_formatting(self, mock_noescape, mock_tabular):
        """Test that keys are bold and values are monospace."""
        mock_table = MagicMock()
        mock_tabular.return_value = mock_table
        mock_noescape.side_effect = lambda x: f"NoEscape({x})"

        _ = self.builder.build(self.sample_entries)

        # Check that add_row was called with formatted cells
        for call_args in mock_table.add_row.call_args_list:
            row = call_args[0][0]
            # Key should be bold
            self.assertIn("textbf", str(row[0]))
            # Value should be monospace
            self.assertIn("AtkinsonMono", str(row[1]))


class TestComparisonSummaryTableBuilder(unittest.TestCase):
    """Tests for comparison summary table builder."""

    @patch("pdf_generator.core.table_builders.Tabular")
    def test_create_comparison_summary_table(self, mock_tabular):
        """Test comparison summary table creation."""
        mock_table = MagicMock()
        mock_tabular.return_value = mock_table

        entries = [
            {
                "label": "Maximum Velocity",
                "primary": "30.00 mph",
                "compare": "35.00 mph",
                "change": "+16.7%",
            }
        ]

        _ = create_comparison_summary_table(
            entries, "2025-06-01 to 2025-06-07", "2025-05-01 to 2025-05-07"
        )

        mock_tabular.assert_called_once()
        self.assertGreaterEqual(mock_table.add_row.call_count, 2)


class TestHistogramTableBuilder(unittest.TestCase):
    """Tests for HistogramTableBuilder class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = HistogramTableBuilder()
        self.sample_histogram = {
            "0": 10,
            "5": 50,
            "10": 100,
            "15": 80,
            "20": 60,
            "25": 40,
            "30": 20,
            "35": 10,
        }

    def test_initialization(self):
        """Test builder initialization."""
        self.assertIsNotNone(self.builder)

    @patch("pdf_generator.core.table_builders.process_histogram")
    @patch("pdf_generator.core.table_builders.Center")
    @patch("pdf_generator.core.table_builders.Tabular")
    def test_build_basic(self, mock_tabular, mock_center, mock_process):
        """Test basic histogram table building."""
        # Mock process_histogram to return test data
        mock_process.return_value = (
            {
                0.0: 10,
                5.0: 50,
                10.0: 100,
                15.0: 80,
                20.0: 60,
                25.0: 40,
                30.0: 20,
                35.0: 10,
            },
            370,
            [(5.0, 10.0), (10.0, 15.0), (15.0, 20.0), (20.0, 25.0), (25.0, 30.0)],
        )

        mock_table = MagicMock()
        mock_tabular.return_value = mock_table
        mock_centered = MagicMock()
        mock_center.return_value = mock_centered

        _ = self.builder.build(
            self.sample_histogram,
            units="mph",
            cutoff=5.0,
            bucket_size=5.0,
            max_bucket=35.0,
        )

        # Should create centered container
        mock_center.assert_called()
        # Should create table
        mock_tabular.assert_called()
        # Should add header, data rows, and hline
        self.assertGreater(mock_table.add_row.call_count, 0)
        self.assertGreater(mock_table.add_hline.call_count, 0)

    @patch("pdf_generator.core.table_builders.process_histogram")
    @patch("pdf_generator.core.table_builders.count_in_histogram_range")
    @patch("pdf_generator.core.table_builders.count_histogram_ge")
    @patch("pdf_generator.core.table_builders.Center")
    @patch("pdf_generator.core.table_builders.Tabular")
    def test_build_with_max_bucket(
        self, mock_tabular, mock_center, mock_ge, mock_range, mock_process
    ):
        """Test building with explicit max bucket."""
        mock_process.return_value = (
            {5.0: 50, 10.0: 100, 15.0: 80, 20.0: 60, 25.0: 40, 30.0: 20, 35.0: 10},
            360,
            [(5.0, 10.0), (10.0, 15.0), (15.0, 20.0), (20.0, 25.0), (25.0, 30.0)],
        )
        mock_range.return_value = 50
        mock_ge.return_value = 30

        mock_table = MagicMock()
        mock_tabular.return_value = mock_table
        mock_centered = MagicMock()
        mock_center.return_value = mock_centered

        _ = self.builder.build(
            self.sample_histogram,
            units="mph",
            cutoff=5.0,
            bucket_size=5.0,
            max_bucket=35.0,
        )

        # Should call count_histogram_ge for the last bucket
        mock_ge.assert_called()

    @patch("pdf_generator.core.table_builders.process_histogram")
    @patch("pdf_generator.core.table_builders.Center")
    @patch("pdf_generator.core.table_builders.Tabular")
    def test_compute_ranges(self, mock_tabular, mock_center, mock_process):
        """Test range computation from histogram data."""
        numeric_buckets = {5.0: 50, 10.0: 100, 15.0: 80, 20.0: 60, 25.0: 40}
        fallback_ranges = [(5.0, 10.0), (10.0, 15.0)]

        ranges = self.builder._compute_ranges(
            numeric_buckets,
            bucket_size=5.0,
            max_bucket=25.0,
            fallback_ranges=fallback_ranges,
        )

        # Should compute ranges from data
        self.assertIsInstance(ranges, list)
        if ranges and ranges != fallback_ranges:
            # Should have ranges covering the data
            self.assertGreater(len(ranges), 0)
            # First range should start at minimum key
            self.assertAlmostEqual(ranges[0][0], 5.0)

    def test_compute_ranges_empty_data(self):
        """Test range computation with empty data."""
        fallback_ranges = [(5.0, 10.0), (10.0, 15.0)]

        ranges = self.builder._compute_ranges(
            {}, bucket_size=5.0, max_bucket=None, fallback_ranges=fallback_ranges
        )

        # Should return fallback ranges
        self.assertEqual(ranges, fallback_ranges)

    @patch("pdf_generator.core.table_builders.process_histogram")
    @patch("pdf_generator.core.table_builders.Center")
    @patch("pdf_generator.core.table_builders.Tabular")
    def test_no_below_cutoff_row_when_zero_count(
        self, mock_tabular, mock_center, mock_process
    ):
        """Test that <cutoff bucket is not shown when count is 0."""
        # Mock process_histogram to return data with NO values below cutoff
        mock_process.return_value = (
            {
                5.0: 50,
                10.0: 100,
                15.0: 80,
                20.0: 60,
            },
            290,
            [(5.0, 10.0), (10.0, 15.0), (15.0, 20.0), (20.0, 25.0)],
        )

        mock_table = MagicMock()
        mock_tabular.return_value = mock_table
        mock_centered = MagicMock()
        mock_center.return_value = mock_centered

        _ = self.builder.build(
            {"5": 50, "10": 100, "15": 80, "20": 60},
            units="mph",
            cutoff=5.0,
            bucket_size=5.0,
            max_bucket=25.0,
        )

        # Check that no row was added with "<5" label
        added_rows = [call[0][0] for call in mock_table.add_row.call_args_list]
        # Convert to strings to check
        row_strings = []
        for row in added_rows:
            if hasattr(row, "__iter__"):
                row_strings.extend([str(cell) for cell in row])
            else:
                row_strings.append(str(row))

        # Should NOT contain "<5" in any row
        has_below_cutoff = any("<5" in str(item) for item in row_strings)
        self.assertFalse(
            has_below_cutoff, "Table should not contain <5 bucket when count is 0"
        )


class TestHistogramComparisonTableBuilder(unittest.TestCase):
    """Tests for histogram comparison table generation."""

    @patch("pdf_generator.core.table_builders.escape_latex")
    @patch("pdf_generator.core.table_builders.NoEscape")
    @patch("pdf_generator.core.table_builders.Center")
    @patch("pdf_generator.core.table_builders.Tabular")
    @patch("pdf_generator.core.table_builders.process_histogram")
    def test_create_histogram_comparison_table(
        self,
        mock_process,
        mock_tabular,
        mock_center_class,
        mock_noescape,
        mock_escape,
    ):
        """Test comparison histogram table creation with mixed buckets."""
        mock_escape.side_effect = lambda x: x
        mock_noescape.side_effect = lambda x: x

        primary_buckets = {0.0: 2, 5.0: 10}
        compare_buckets = {0.0: 1, 5.0: 5, 10.0: 10}
        ranges = [(5.0, 10.0), (10.0, 15.0)]
        mock_process.side_effect = [
            (primary_buckets, 12, ranges),
            (compare_buckets, 16, ranges),
        ]

        mock_table = MagicMock()
        mock_tabular.return_value = mock_table
        mock_center = MagicMock()
        mock_center_class.return_value = mock_center

        result = create_histogram_comparison_table(
            {"0": 2, "5": 10},
            {"0": 1, "5": 5, "10": 10},
            units="mph",
            primary_label="Primary",
            compare_label="Compare",
            cutoff=5.0,
            bucket_size=5.0,
            max_bucket=15.0,
        )

        self.assertEqual(result, mock_center)

        added_rows = [call.args[0] for call in mock_table.add_row.call_args_list]
        row_cells = [str(cell) for row in added_rows for cell in row]
        self.assertTrue(any("0-5" in cell for cell in row_cells))
        # With max_bucket=15, ranges are (5-10), (10-15), then "15+"
        self.assertTrue(any("10-15" in cell for cell in row_cells))
        # Delta column now shows percentage point differences, not "--" for zero counts
        self.assertTrue(any("+" in cell or "-" in cell for cell in row_cells))


class TestConvenienceFunctions(unittest.TestCase):
    """Tests for convenience wrapper functions."""

    @patch("pdf_generator.core.table_builders.StatsTableBuilder")
    def test_create_stats_table(self, mock_builder_class):
        """Test create_stats_table convenience function."""
        mock_builder = MagicMock()
        mock_builder_class.return_value = mock_builder
        mock_builder.build.return_value = "table"

        stats = [{"count": 100, "p50": 25.0}]
        _ = create_stats_table(stats, "UTC", "mph", "Caption")

        # Should create builder and call build
        mock_builder_class.assert_called_once()
        mock_builder.build.assert_called_once_with(
            stats, "UTC", "mph", "Caption", True, True
        )

    @patch("pdf_generator.core.table_builders.ParameterTableBuilder")
    def test_create_param_table(self, mock_builder_class):
        """Test create_param_table convenience function."""
        mock_builder = MagicMock()
        mock_builder_class.return_value = mock_builder
        mock_builder.build.return_value = "table"

        entries = [{"key": "K", "value": "V"}]
        _ = create_param_table(entries)

        # Should create builder and call build
        mock_builder_class.assert_called_once()
        mock_builder.build.assert_called_once_with(entries)

    @patch("pdf_generator.core.table_builders.HistogramTableBuilder")
    def test_create_histogram_table(self, mock_builder_class):
        """Test create_histogram_table convenience function."""
        mock_builder = MagicMock()
        mock_builder_class.return_value = mock_builder
        mock_builder.build.return_value = "table"

        histogram = {"5": 100, "10": 200}
        _ = create_histogram_table(histogram, "mph", cutoff=5.0)

        # Should create builder and call build
        mock_builder_class.assert_called_once()
        mock_builder.build.assert_called_once_with(histogram, "mph", 5.0, 5.0, None)

    @patch("pdf_generator.core.table_builders.StatsTableBuilder")
    def test_create_twocolumn_stats_table(self, mock_builder_class):
        """Test create_twocolumn_stats_table convenience function."""
        mock_builder = MagicMock()
        mock_builder_class.return_value = mock_builder

        mock_doc = MagicMock()
        stats = [{"count": 100, "p50": 25.0}]

        create_twocolumn_stats_table(mock_doc, stats, "UTC", "mph", "Caption")

        # Should create builder and call build_twocolumn
        mock_builder_class.assert_called_once()
        mock_builder.build_twocolumn.assert_called_once_with(
            mock_doc, stats, "UTC", "mph", "Caption"
        )


class TestImportFallbacks(unittest.TestCase):
    """Tests for import error handling."""

    def test_pylatex_available(self):
        """Test that PyLaTeX is available in normal environment."""
        from pdf_generator.core.table_builders import HAVE_PYLATEX

        # In test environment, should be True
        self.assertTrue(HAVE_PYLATEX)

    def test_builder_requires_pylatex(self):
        """Test that builders require PyLaTeX."""
        # This test validates the import check exists
        # In a real no-pylatex environment, initialization would raise ImportError
        # but we can't test that without mocking imports
        builder = StatsTableBuilder()
        self.assertIsNotNone(builder)


# Phase 2: Edge Case Tests


class TestHistogramEdgeCases(unittest.TestCase):
    """Phase 2 tests for histogram table edge cases.

    Tests exercise histogram table behavior for edge conditions such as
    empty histograms, single buckets, and cutoff boundary conditions.
    """

    def setUp(self):
        """Set up test fixtures."""
        self.mock_doc = MagicMock()
        self.builder = HistogramTableBuilder()

    def test_histogram_with_all_values_below_cutoff(self):
        """Test histogram when all values are below the cutoff speed."""
        histogram = {
            "0.0": 5,
            "5.0": 10,
            "10.0": 15,
            "15.0": 20,
            # All below cutoff of 25
        }

        # Call builder with cutoff=25, bucket_size=5, max_bucket=20
        table = self.builder.build(
            histogram,
            units="mph",
            cutoff=25.0,
            bucket_size=5.0,
            max_bucket=20.0,
        )

        # Should handle this case gracefully
        self.assertIsNotNone(table)

    def test_histogram_with_zero_total_count(self):
        """Test histogram table with zero total count."""
        histogram = {}  # Empty histogram = zero total

        # Should handle empty histogram gracefully
        table = self.builder.build(
            histogram,
            units="mph",
            cutoff=25.0,
            bucket_size=5.0,
            max_bucket=None,
        )

        self.assertIsNotNone(table)

    def test_histogram_with_single_bucket(self):
        """Test histogram with only one bucket."""
        histogram = {
            "20.0": 100,  # Single bucket
        }

        table = self.builder.build(
            histogram,
            units="mph",
            cutoff=25.0,
            bucket_size=5.0,
            max_bucket=None,
        )

        self.assertIsNotNone(table)

    def test_histogram_with_max_bucket_equal_to_cutoff(self):
        """Test edge case where max bucket value equals cutoff."""
        histogram = {
            "10.0": 20,
            "15.0": 30,
            "20.0": 40,
            "25.0": 50,  # Exactly at cutoff
        }

        table = self.builder.build(
            histogram,
            units="mph",
            cutoff=25.0,
            bucket_size=5.0,
            max_bucket=25.0,  # Max equals cutoff
        )

        self.assertIsNotNone(table)

    def test_histogram_with_no_below_cutoff_values(self):
        """Test histogram where no values are below cutoff."""
        histogram = {
            "30.0": 20,
            "35.0": 30,
            "40.0": 40,
            # All above cutoff of 25
        }

        table = self.builder.build(
            histogram,
            units="mph",
            cutoff=25.0,
            bucket_size=5.0,
            max_bucket=None,
        )

        # Should skip the below-cutoff row
        self.assertIsNotNone(table)


class TestPyLaTeXImportErrors(unittest.TestCase):
    """Test that ImportError is raised when PyLaTeX is not available."""

    def test_stats_table_builder_import_error(self):
        """Test that StatsTableBuilder raises ImportError without PyLaTeX."""
        import pdf_generator.core.table_builders as tb_module

        original_have_pylatex = tb_module.HAVE_PYLATEX

        try:
            tb_module.HAVE_PYLATEX = False

            with self.assertRaises(ImportError) as context:
                StatsTableBuilder()

            self.assertIn("PyLaTeX is required", str(context.exception))
            self.assertIn("pip install pylatex", str(context.exception))
        finally:
            tb_module.HAVE_PYLATEX = original_have_pylatex

    def test_parameter_table_builder_import_error(self):
        """Test that ParameterTableBuilder raises ImportError without PyLaTeX."""
        import pdf_generator.core.table_builders as tb_module

        original_have_pylatex = tb_module.HAVE_PYLATEX

        try:
            tb_module.HAVE_PYLATEX = False

            with self.assertRaises(ImportError) as context:
                ParameterTableBuilder()

            self.assertIn("PyLaTeX is required", str(context.exception))
            self.assertIn("pip install pylatex", str(context.exception))
        finally:
            tb_module.HAVE_PYLATEX = original_have_pylatex


class TestHistogramTableFallbackMethod(unittest.TestCase):
    """Test the histogram fallback method that adds rows."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = HistogramTableBuilder()

    def test_add_histogram_rows_fallback_with_below_cutoff(self):
        """Test fallback method when there are values below cutoff."""
        from pylatex import Tabular

        table = Tabular("lrr")
        numeric_buckets = {
            5: 10,  # Below cutoff
            10: 20,  # Below cutoff
            15: 30,  # Below cutoff
            20: 40,  # Below cutoff
            25: 50,
            30: 60,
            35: 70,
            40: 80,
        }
        ranges = [(25, 30), (30, 35), (35, 40)]
        cutoff = 25.0
        proc_max = 40.0
        total = sum(numeric_buckets.values())

        # Call the fallback method directly (table, numeric_buckets, total, cutoff, ranges, proc_max)
        self.builder._add_histogram_rows_fallback(
            table, numeric_buckets, total, cutoff, ranges, proc_max
        )

        # Table should have rows
        # Verify it was built without errors
        self.assertIsNotNone(table)

    def test_add_histogram_rows_fallback_without_below_cutoff(self):
        """Test fallback method when there are NO values below cutoff."""
        from pylatex import Tabular

        table = Tabular("lrr")
        numeric_buckets = {
            30: 50,
            35: 60,
            40: 70,
            45: 80,
        }
        ranges = [(30, 35), (35, 40), (40, 45)]
        cutoff = 25.0
        proc_max = 45.0
        total = sum(numeric_buckets.values())

        # Call the fallback method directly (table, numeric_buckets, total, cutoff, ranges, proc_max)
        self.builder._add_histogram_rows_fallback(
            table, numeric_buckets, total, cutoff, ranges, proc_max
        )

        # Table should have rows (no below-cutoff row)
        self.assertIsNotNone(table)

    def test_add_histogram_rows_fallback_edge_case_zero_total(self):
        """Test fallback method with zero total count."""
        from pylatex import Tabular

        table = Tabular("lrr")
        numeric_buckets = {}
        ranges = [(25, 30), (30, 35)]
        cutoff = 25.0
        proc_max = 35.0
        total = 0

        # Should handle zero total without errors (table, numeric_buckets, total, cutoff, ranges, proc_max)
        self.builder._add_histogram_rows_fallback(
            table, numeric_buckets, total, cutoff, ranges, proc_max
        )

        self.assertIsNotNone(table)


if __name__ == "__main__":
    unittest.main()
