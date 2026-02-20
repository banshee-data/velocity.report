"""Coverage boost tests for chart_builder.py.

Targets specific uncovered branches: non-numeric sort fallback,
zero-total percentage branches in build_comparison, tight_layout
exception handling, and HAVE_MATPLOTLIB guard.
"""

import unittest
from unittest.mock import patch

import matplotlib.pyplot as plt

from pdf_generator.core.chart_builder import HistogramChartBuilder


class TestHistogramSortFallback(unittest.TestCase):
    """Test sort fallback when histogram keys are non-numeric."""

    def setUp(self):
        self.builder = HistogramChartBuilder()

    def tearDown(self):
        plt.close("all")

    def test_build_non_numeric_keys_uses_string_sort(self):
        """Non-numeric keys trigger the 'except' branch that sorts by str."""
        histogram = {"low": 10, "medium": 20, "high": 30}
        fig = self.builder.build(histogram, "String Keys", "mph")
        self.assertIsNotNone(fig)
        ax = fig.axes[0]
        self.assertEqual(len(ax.containers), 1)


class TestComparisonZeroTotalBranches(unittest.TestCase):
    """Test build_comparison when primary or compare total is zero."""

    def setUp(self):
        self.builder = HistogramChartBuilder()

    def tearDown(self):
        plt.close("all")

    def test_build_comparison_primary_total_zero(self):
        """When primary counts sum to zero, uses the [0.0]*len fallback."""
        fig = self.builder.build_comparison(
            histogram={"5": 0, "10": 0},
            compare_histogram={"5": 10, "10": 20},
            title="Zero Primary",
            units="mph",
            primary_label="Primary",
            compare_label="Compare",
        )
        self.assertIsNotNone(fig)

    def test_build_comparison_compare_total_zero(self):
        """When compare counts sum to zero, uses the [0.0]*len fallback."""
        fig = self.builder.build_comparison(
            histogram={"5": 10, "10": 20},
            compare_histogram={"5": 0, "10": 0},
            title="Zero Compare",
            units="mph",
            primary_label="Primary",
            compare_label="Compare",
        )
        self.assertIsNotNone(fig)


class TestHistogramTightLayoutException(unittest.TestCase):
    """Test exception handling in tight_layout and subplots_adjust."""

    def setUp(self):
        self.builder = HistogramChartBuilder()

    def tearDown(self):
        plt.close("all")

    def test_build_tight_layout_exception(self):
        """When tight_layout raises, build still returns a figure."""
        histogram = {"5": 10, "10": 20}
        with patch.object(
            plt.Figure, "tight_layout", side_effect=RuntimeError("layout error")
        ):
            fig = self.builder.build(histogram, "Layout Error", "mph")
        self.assertIsNotNone(fig)

    def test_build_subplots_adjust_exception(self):
        """When subplots_adjust raises, build still returns a figure."""
        histogram = {"5": 10, "10": 20}
        with patch.object(
            plt.Figure, "subplots_adjust", side_effect=RuntimeError("adjust error")
        ):
            fig = self.builder.build(histogram, "Adjust Error", "mph")
        self.assertIsNotNone(fig)

    def test_build_comparison_tight_layout_exception(self):
        """When tight_layout raises in comparison mode, still returns a figure."""
        with patch.object(
            plt.Figure, "tight_layout", side_effect=RuntimeError("layout error")
        ):
            fig = self.builder.build_comparison(
                {"5": 10},
                {"5": 20},
                title="Layout Exception",
                units="mph",
                primary_label="A",
                compare_label="B",
            )
        self.assertIsNotNone(fig)

    def test_build_comparison_subplots_adjust_exception(self):
        """When subplots_adjust raises in comparison mode, still returns a figure."""
        with patch.object(
            plt.Figure, "subplots_adjust", side_effect=RuntimeError("adjust error")
        ):
            fig = self.builder.build_comparison(
                {"5": 10},
                {"5": 20},
                title="Adjust Exception",
                units="mph",
                primary_label="A",
                compare_label="B",
            )
        self.assertIsNotNone(fig)


class TestHistogramBuilderNoMatplotlib(unittest.TestCase):
    """Test HAVE_MATPLOTLIB guard in HistogramChartBuilder."""

    def test_init_raises_when_no_matplotlib(self):
        """ImportError raised when HAVE_MATPLOTLIB is False."""
        import pdf_generator.core.chart_builder as cb

        original = cb.HAVE_MATPLOTLIB
        cb.HAVE_MATPLOTLIB = False
        try:
            with self.assertRaises(ImportError):
                HistogramChartBuilder()
        finally:
            cb.HAVE_MATPLOTLIB = original
