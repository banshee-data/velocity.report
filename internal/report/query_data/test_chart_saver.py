#!/usr/bin/env python3
"""Unit tests for chart_saver module.

Tests cover:
- PDF saving with tight bounds
- Size constraint application
- Figure cleanup
- Error handling
"""

import os
import unittest
import tempfile
import shutil
from unittest.mock import MagicMock, patch

from chart_saver import ChartSaver, save_chart_as_pdf


class TestChartSaver(unittest.TestCase):
    """Test ChartSaver class."""

    def setUp(self):
        """Create temporary directory for test files."""
        self.temp_dir = tempfile.mkdtemp()
        self.output_path = os.path.join(self.temp_dir, "test_chart.pdf")

    def tearDown(self):
        """Clean up temporary directory."""
        shutil.rmtree(self.temp_dir)

    def test_initialization_defaults(self):
        """Test ChartSaver initialization with default settings."""
        saver = ChartSaver()

        self.assertEqual(saver.min_width_in, 6.0)  # From LAYOUT config
        self.assertEqual(saver.max_width_in, 11.0)  # From LAYOUT config

    def test_initialization_custom(self):
        """Test ChartSaver initialization with custom constraints."""
        saver = ChartSaver(min_width_in=4.0, max_width_in=8.0)

        self.assertEqual(saver.min_width_in, 4.0)
        self.assertEqual(saver.max_width_in, 8.0)

    def test_apply_size_constraints_no_change(self):
        """Test size constraints when dimensions are within limits."""
        saver = ChartSaver(min_width_in=4.0, max_width_in=10.0)

        width, height = saver._apply_size_constraints(7.0, 5.0)

        self.assertAlmostEqual(width, 7.0)
        self.assertAlmostEqual(height, 5.0)

    def test_apply_size_constraints_scale_up(self):
        """Test size constraints when width is too small."""
        saver = ChartSaver(min_width_in=6.0, max_width_in=10.0)

        width, height = saver._apply_size_constraints(3.0, 2.0)

        # Should scale up proportionally
        self.assertAlmostEqual(width, 6.0)
        self.assertAlmostEqual(height, 4.0)  # 2.0 * (6.0/3.0)

    def test_apply_size_constraints_scale_down(self):
        """Test size constraints when width is too large."""
        saver = ChartSaver(min_width_in=4.0, max_width_in=8.0)

        width, height = saver._apply_size_constraints(12.0, 9.0)

        # Should scale down proportionally
        self.assertAlmostEqual(width, 8.0)
        self.assertAlmostEqual(height, 6.0)  # 9.0 * (8.0/12.0)

    def test_get_dpi_from_figure(self):
        """Test DPI retrieval from figure."""
        saver = ChartSaver()

        # Mock figure with dpi attribute
        fig = MagicMock()
        fig.dpi = 100

        dpi = saver._get_dpi(fig)
        self.assertEqual(dpi, 100)

    def test_get_dpi_fallback(self):
        """Test DPI fallback when figure doesn't have dpi."""
        saver = ChartSaver()

        # Mock figure without dpi attribute
        fig = MagicMock(spec=[])

        dpi = saver._get_dpi(fig)
        # Should get matplotlib's default (typically 100 or 72)
        self.assertIn(dpi, [72, 100.0])  # Accept either common default

    @patch("chart_saver.plt")
    def test_close_figure_success(self, mock_plt):
        """Test successful figure closing."""
        saver = ChartSaver()
        fig = MagicMock()

        saver._close_figure(fig)

        mock_plt.close.assert_called_once_with(fig)

    @patch("chart_saver.plt", None)
    def test_close_figure_no_matplotlib(self):
        """Test figure closing when matplotlib not available."""
        saver = ChartSaver()
        fig = MagicMock()

        # Should not raise exception
        saver._close_figure(fig)

    def test_save_as_pdf_simple(self):
        """Test basic PDF saving (without tight bounds optimization)."""
        saver = ChartSaver()

        # Create mock figure with basic savefig
        fig = MagicMock()
        fig.savefig = MagicMock()

        # Mock the tight bbox calculation to raise exception (triggers fallback)
        fig.canvas.draw.side_effect = Exception("No canvas")

        result = saver.save_as_pdf(fig, self.output_path, close_fig=False)

        # Should fall back to simple savefig
        self.assertTrue(result)
        self.assertTrue(fig.savefig.called)

    def test_save_as_pdf_with_close(self):
        """Test PDF saving with figure cleanup."""
        saver = ChartSaver()

        fig = MagicMock()
        fig.savefig = MagicMock()
        fig.canvas.draw.side_effect = Exception("No canvas")

        with patch.object(saver, "_close_figure") as mock_close:
            result = saver.save_as_pdf(fig, self.output_path, close_fig=True)

            self.assertTrue(result)
            mock_close.assert_called_once_with(fig)

    def test_save_as_pdf_without_close(self):
        """Test PDF saving without figure cleanup."""
        saver = ChartSaver()

        fig = MagicMock()
        fig.savefig = MagicMock()
        fig.canvas.draw.side_effect = Exception("No canvas")

        with patch.object(saver, "_close_figure") as mock_close:
            result = saver.save_as_pdf(fig, self.output_path, close_fig=False)

            self.assertTrue(result)
            mock_close.assert_not_called()

    def test_save_as_pdf_failure(self):
        """Test PDF saving failure handling."""
        saver = ChartSaver()

        # Mock figure that raises exception on savefig
        fig = MagicMock()
        fig.savefig.side_effect = Exception("Save failed")
        fig.canvas.draw.side_effect = Exception("No canvas")

        result = saver.save_as_pdf(fig, self.output_path, close_fig=False)

        self.assertFalse(result)


class TestConvenienceFunction(unittest.TestCase):
    """Test save_chart_as_pdf convenience function."""

    def setUp(self):
        """Create temporary directory for test files."""
        self.temp_dir = tempfile.mkdtemp()
        self.output_path = os.path.join(self.temp_dir, "test_chart.pdf")

    def tearDown(self):
        """Clean up temporary directory."""
        shutil.rmtree(self.temp_dir)

    @patch("chart_saver.ChartSaver")
    def test_save_chart_as_pdf(self, mock_saver_class):
        """Test convenience function delegates to ChartSaver."""
        mock_saver = MagicMock()
        mock_saver.save_as_pdf.return_value = True
        mock_saver_class.return_value = mock_saver

        fig = MagicMock()
        result = save_chart_as_pdf(fig, self.output_path, close_fig=True)

        self.assertTrue(result)
        mock_saver_class.assert_called_once()
        mock_saver.save_as_pdf.assert_called_once_with(fig, self.output_path, True)

    @patch("chart_saver.ChartSaver")
    def test_save_chart_as_pdf_exception(self, mock_saver_class):
        """Test convenience function error handling."""
        mock_saver_class.side_effect = Exception("Initialization failed")

        fig = MagicMock()
        result = save_chart_as_pdf(fig, self.output_path)

        self.assertFalse(result)


if __name__ == "__main__":
    unittest.main()
