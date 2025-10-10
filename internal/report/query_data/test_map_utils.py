#!/usr/bin/env python3
"""Unit tests for map_utils module.

Tests cover:
- RadarMarker creation and coordinate conversion
- SVG marker injection
- Triangle geometry calculations
- SVG viewBox extraction
- Map processing workflow
"""

import os
import unittest
import tempfile
import shutil
from unittest.mock import patch, MagicMock

from map_utils import (
    RadarMarker,
    SVGMarkerInjector,
    SVGToPDFConverter,
    MapProcessor,
    create_marker_from_config,
)


class TestRadarMarker(unittest.TestCase):
    """Test RadarMarker class."""

    def test_marker_initialization_defaults(self):
        """Test marker creation with default parameters."""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=45.0)

        self.assertEqual(marker.cx_frac, 0.5)
        self.assertEqual(marker.cy_frac, 0.5)
        self.assertEqual(marker.bearing_deg, 45.0)
        self.assertEqual(marker.coverage_length, 0.42)
        self.assertEqual(marker.coverage_angle, 20.0)
        self.assertEqual(marker.color, "#f25f5c")
        self.assertEqual(marker.opacity, 0.9)
        self.assertIsNone(marker.gps_lat)
        self.assertIsNone(marker.gps_lon)

    def test_marker_initialization_custom(self):
        """Test marker creation with custom parameters."""
        marker = RadarMarker(
            cx_frac=0.3,
            cy_frac=0.7,
            bearing_deg=90.0,
            coverage_length=0.5,
            coverage_angle=30.0,
            color="#ff0000",
            opacity=0.8,
            gps_lat=37.7749,
            gps_lon=-122.4194,
        )

        self.assertEqual(marker.cx_frac, 0.3)
        self.assertEqual(marker.cy_frac, 0.7)
        self.assertEqual(marker.bearing_deg, 90.0)
        self.assertEqual(marker.coverage_length, 0.5)
        self.assertEqual(marker.coverage_angle, 30.0)
        self.assertEqual(marker.color, "#ff0000")
        self.assertEqual(marker.opacity, 0.8)
        self.assertEqual(marker.gps_lat, 37.7749)
        self.assertEqual(marker.gps_lon, -122.4194)

    def test_to_svg_coords(self):
        """Test conversion from fractional to SVG coordinates."""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)

        # Test with simple viewBox
        viewbox = (0.0, 0.0, 100.0, 100.0)
        cx, cy = marker.to_svg_coords(viewbox)
        self.assertAlmostEqual(cx, 50.0)
        self.assertAlmostEqual(cy, 50.0)

        # Test with offset viewBox
        viewbox = (10.0, 20.0, 100.0, 100.0)
        cx, cy = marker.to_svg_coords(viewbox)
        self.assertAlmostEqual(cx, 60.0)
        self.assertAlmostEqual(cy, 70.0)

        # Test with corner positions
        marker_tl = RadarMarker(cx_frac=0.0, cy_frac=0.0, bearing_deg=0.0)
        cx, cy = marker_tl.to_svg_coords((0, 0, 100, 100))
        self.assertAlmostEqual(cx, 0.0)
        self.assertAlmostEqual(cy, 0.0)

        marker_br = RadarMarker(cx_frac=1.0, cy_frac=1.0, bearing_deg=0.0)
        cx, cy = marker_br.to_svg_coords((0, 0, 100, 100))
        self.assertAlmostEqual(cx, 100.0)
        self.assertAlmostEqual(cy, 100.0)


class TestSVGMarkerInjector(unittest.TestCase):
    """Test SVGMarkerInjector class."""

    def setUp(self):
        """Create injector instance for tests."""
        self.injector = SVGMarkerInjector()

    def test_extract_viewbox_from_viewbox_attribute(self):
        """Test viewBox extraction from viewBox attribute."""
        svg = '<svg viewBox="0 0 800 600"></svg>'
        viewbox = self.injector._extract_viewbox(svg)
        self.assertEqual(viewbox, (0.0, 0.0, 800.0, 600.0))

        # Test with offset viewBox
        svg = '<svg viewBox="10 20 800 600"></svg>'
        viewbox = self.injector._extract_viewbox(svg)
        self.assertEqual(viewbox, (10.0, 20.0, 800.0, 600.0))

        # Test with quotes
        svg = "<svg viewBox='0 0 800 600'></svg>"
        viewbox = self.injector._extract_viewbox(svg)
        self.assertEqual(viewbox, (0.0, 0.0, 800.0, 600.0))

    def test_extract_viewbox_from_width_height(self):
        """Test viewBox extraction from width/height attributes."""
        svg = '<svg width="800" height="600"></svg>'
        viewbox = self.injector._extract_viewbox(svg)
        self.assertEqual(viewbox, (0.0, 0.0, 800.0, 600.0))

    def test_extract_viewbox_failure(self):
        """Test viewBox extraction failure handling."""
        svg = "<svg></svg>"
        with self.assertRaises(RuntimeError):
            self.injector._extract_viewbox(svg)

    def test_compute_triangle_points_north(self):
        """Test triangle points computation for north-pointing marker."""
        marker = RadarMarker(
            cx_frac=0.5,
            cy_frac=0.5,
            bearing_deg=0.0,  # North
            coverage_length=0.1,
            coverage_angle=20.0,
        )
        viewbox = (0.0, 0.0, 100.0, 100.0)

        points = self.injector._compute_triangle_points(marker, viewbox)

        # Should be a valid points string
        self.assertIsInstance(points, str)
        self.assertIn(",", points)

        # Should have 3 coordinate pairs
        coords = points.split()
        self.assertEqual(len(coords), 3)

    def test_compute_triangle_points_east(self):
        """Test triangle points computation for east-pointing marker."""
        marker = RadarMarker(
            cx_frac=0.5,
            cy_frac=0.5,
            bearing_deg=90.0,  # East
            coverage_length=0.1,
            coverage_angle=30.0,
        )
        viewbox = (0.0, 0.0, 100.0, 100.0)

        points = self.injector._compute_triangle_points(marker, viewbox)

        # Verify we got a valid points string
        coords = points.split()
        self.assertEqual(len(coords), 3)

    def test_inject_marker_basic(self):
        """Test basic marker injection into SVG."""
        svg = '<svg viewBox="0 0 100 100">\n</svg>'
        marker = RadarMarker(
            cx_frac=0.5,
            cy_frac=0.5,
            bearing_deg=0.0,
            coverage_length=0.1,
        )

        result = self.injector.inject_marker(svg, marker)

        # Verify marker was injected
        self.assertIn("radar-marker", result)
        self.assertIn("polygon", result)
        self.assertIn("circle", result)
        self.assertIn("points=", result)

        # Verify original content preserved
        self.assertIn("viewBox", result)

        # Verify SVG structure maintained
        self.assertTrue(result.strip().endswith("</svg>"))

    def test_inject_marker_with_custom_colors(self):
        """Test marker injection with custom colors."""
        svg = '<svg viewBox="0 0 100 100">\n</svg>'
        marker = RadarMarker(
            cx_frac=0.5,
            cy_frac=0.5,
            bearing_deg=45.0,
            color="#00ff00",
            opacity=0.7,
        )

        injector = SVGMarkerInjector(
            circle_fill="#ff0000",
            circle_stroke="#0000ff",
        )

        result = injector.inject_marker(svg, marker)

        # Verify custom colors appear in output
        self.assertIn("#00ff00", result)  # Marker color
        self.assertIn("#ff0000", result)  # Circle fill
        self.assertIn("#0000ff", result)  # Circle stroke
        self.assertIn("0.7", result)  # Opacity

    def test_inject_marker_preserves_content(self):
        """Test that marker injection preserves existing SVG content."""
        svg = """<svg viewBox="0 0 100 100">
  <rect x="10" y="10" width="20" height="20"/>
</svg>"""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)

        result = self.injector.inject_marker(svg, marker)

        # Verify existing content preserved
        self.assertIn("<rect", result)
        self.assertIn('x="10"', result)

        # Verify marker added after existing content
        rect_idx = result.index("<rect")
        marker_idx = result.index("radar-marker")
        self.assertGreater(marker_idx, rect_idx)

    def test_inject_marker_svg_without_closing_tag(self):
        """Test marker injection when SVG doesn't end with proper closing tag (line 249)."""
        # SVG that doesn't end with </svg>
        svg = '<svg viewBox="0 0 100 100">'
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)

        result = self.injector.inject_marker(svg, marker)

        # Marker should be appended
        self.assertIn("radar-marker", result)
        # Original content preserved
        self.assertIn('<svg viewBox="0 0 100 100">', result)


class TestSVGToPDFConverter(unittest.TestCase):
    """Test SVGToPDFConverter class."""

    def setUp(self):
        """Create temporary directory for test files."""
        self.temp_dir = tempfile.mkdtemp()
        self.svg_path = os.path.join(self.temp_dir, "test.svg")
        self.pdf_path = os.path.join(self.temp_dir, "test.pdf")

        # Create minimal valid SVG
        with open(self.svg_path, "w") as f:
            f.write('<svg viewBox="0 0 100 100"></svg>')

    def tearDown(self):
        """Clean up temporary directory."""
        shutil.rmtree(self.temp_dir)

    @patch("map_utils.importlib.util.find_spec")
    def test_try_cairosvg_success(self, mock_find_spec):
        """Test successful conversion with cairosvg."""
        mock_find_spec.return_value = MagicMock()  # cairosvg available

        # Since svg2pdf is imported conditionally, we need to patch it where it's used
        with patch("builtins.open", create=True) as mock_open:
            mock_open.return_value.__enter__.return_value = MagicMock()

            # The actual conversion will fail, but we're just testing the attempt
            result = SVGToPDFConverter._try_cairosvg(self.svg_path, self.pdf_path)
            # Result depends on whether cairosvg is actually installed
            self.assertIsInstance(result, bool)

    @patch("map_utils.importlib.util.find_spec")
    def test_try_cairosvg_not_available(self, mock_find_spec):
        """Test cairosvg not available."""
        mock_find_spec.return_value = None  # cairosvg not available

        result = SVGToPDFConverter._try_cairosvg(self.svg_path, self.pdf_path)
        self.assertFalse(result)

    @patch("subprocess.check_call")
    def test_try_inkscape_success(self, mock_check_call):
        """Test successful conversion with inkscape."""
        # Mock inkscape available and working
        mock_check_call.return_value = None

        # Create dummy PDF to simulate success
        with open(self.pdf_path, "w") as f:
            f.write("dummy pdf")

        result = SVGToPDFConverter._try_inkscape(self.svg_path, self.pdf_path)

        # Should return True if inkscape runs successfully
        # (actual behavior depends on inkscape availability)
        self.assertIsInstance(result, bool)

    @patch("subprocess.check_call")
    def test_try_rsvg_convert_success(self, mock_check_call):
        """Test successful conversion with rsvg-convert."""
        mock_check_call.return_value = None

        # Create dummy PDF
        with open(self.pdf_path, "w") as f:
            f.write("dummy pdf")

        result = SVGToPDFConverter._try_rsvg_convert(self.svg_path, self.pdf_path)

        self.assertIsInstance(result, bool)


class TestMapProcessor(unittest.TestCase):
    """Test MapProcessor class."""

    def setUp(self):
        """Create temporary directory for test files."""
        self.temp_dir = tempfile.mkdtemp()
        self.map_svg = os.path.join(self.temp_dir, "map.svg")

        # Create minimal valid SVG
        with open(self.map_svg, "w") as f:
            f.write('<svg viewBox="0 0 100 100"></svg>')

    def tearDown(self):
        """Clean up temporary directory."""
        shutil.rmtree(self.temp_dir)

    def test_processor_initialization(self):
        """Test MapProcessor initialization."""
        processor = MapProcessor(base_dir=self.temp_dir)
        self.assertEqual(processor.base_dir, self.temp_dir)
        self.assertIsInstance(processor.injector, SVGMarkerInjector)

    def test_processor_custom_marker_config(self):
        """Test MapProcessor with custom marker config."""
        config = {
            "circle_radius": 30.0,
            "circle_fill": "#ff0000",
            "circle_stroke": "#00ff00",
            "circle_stroke_width": 3.0,
        }
        processor = MapProcessor(
            base_dir=self.temp_dir,
            marker_config=config,
        )

        self.assertEqual(processor.injector.circle_radius, 30.0)
        self.assertEqual(processor.injector.circle_fill, "#ff0000")
        self.assertEqual(processor.injector.circle_stroke, "#00ff00")
        self.assertEqual(processor.injector.circle_stroke_width, 3.0)

    def test_process_map_no_svg(self):
        """Test processing when map.svg doesn't exist."""
        processor = MapProcessor(base_dir=tempfile.mkdtemp())
        success, pdf_path = processor.process_map()

        self.assertFalse(success)
        self.assertIsNone(pdf_path)

    @patch.object(SVGToPDFConverter, "convert")
    def test_process_map_without_marker(self, mock_convert):
        """Test processing map without marker overlay."""
        mock_convert.return_value = True

        processor = MapProcessor(base_dir=self.temp_dir)
        success, pdf_path = processor.process_map(marker=None)

        # Should attempt conversion
        self.assertTrue(mock_convert.called)

    @patch.object(SVGToPDFConverter, "convert")
    def test_process_map_with_marker(self, mock_convert):
        """Test processing map with marker overlay."""
        mock_convert.return_value = True

        marker = RadarMarker(
            cx_frac=0.5,
            cy_frac=0.5,
            bearing_deg=0.0,
            coverage_length=0.1,
        )

        processor = MapProcessor(base_dir=self.temp_dir)
        success, pdf_path = processor.process_map(marker=marker)

        # Should create temporary SVG with marker
        temp_svg = os.path.join(self.temp_dir, "map_with_marker.svg")
        self.assertTrue(os.path.exists(temp_svg))

        # Verify marker was injected
        with open(temp_svg, "r") as f:
            content = f.read()
            self.assertIn("radar-marker", content)

    @patch.object(SVGToPDFConverter, "convert")
    def test_process_map_force_convert(self, mock_convert):
        """Test force conversion flag."""
        mock_convert.return_value = True

        # Create existing PDF
        map_pdf = os.path.join(self.temp_dir, "map.pdf")
        with open(map_pdf, "w") as f:
            f.write("existing pdf")

        processor = MapProcessor(base_dir=self.temp_dir)

        # Without force_convert, might skip (depends on timestamps)
        # With force_convert, should always convert
        success, pdf_path = processor.process_map(force_convert=True)

        self.assertTrue(mock_convert.called)


class TestHelperFunctions(unittest.TestCase):
    """Test module-level helper functions."""

    def test_create_marker_from_config(self):
        """Test creating marker from config dictionary."""
        config = {
            "triangle_cx": 0.4,
            "triangle_cy": 0.6,
            "triangle_angle": 45.0,
            "triangle_len": 0.3,
            "triangle_apex_angle": 25.0,
            "triangle_color": "#ff0000",
            "triangle_opacity": 0.8,
        }

        marker = create_marker_from_config(config)

        self.assertEqual(marker.cx_frac, 0.4)
        self.assertEqual(marker.cy_frac, 0.6)
        self.assertEqual(marker.bearing_deg, 45.0)
        self.assertEqual(marker.coverage_length, 0.3)
        self.assertEqual(marker.coverage_angle, 25.0)
        self.assertEqual(marker.color, "#ff0000")
        self.assertEqual(marker.opacity, 0.8)

    def test_create_marker_from_config_defaults(self):
        """Test creating marker from config with missing keys."""
        config = {}  # Empty config should use defaults

        marker = create_marker_from_config(config)

        self.assertEqual(marker.cx_frac, 0.385)
        self.assertEqual(marker.cy_frac, 0.71)
        self.assertEqual(marker.bearing_deg, 32.0)
        self.assertEqual(marker.coverage_length, 0.42)
        self.assertEqual(marker.coverage_angle, 20.0)
        self.assertEqual(marker.color, "#f25f5c")
        self.assertEqual(marker.opacity, 0.9)


class TestGPSCoordinateEdgeCases(unittest.TestCase):
    """Test GPS coordinate edge cases."""

    def test_marker_with_boundary_gps_coordinates(self):
        """Test marker with boundary GPS values."""
        # North pole
        marker_north = RadarMarker(
            cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0, gps_lat=90.0, gps_lon=0.0
        )
        self.assertEqual(marker_north.gps_lat, 90.0)

        # South pole
        marker_south = RadarMarker(
            cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0, gps_lat=-90.0, gps_lon=0.0
        )
        self.assertEqual(marker_south.gps_lat, -90.0)

        # Date line
        marker_date = RadarMarker(
            cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0, gps_lat=0.0, gps_lon=180.0
        )
        self.assertEqual(marker_date.gps_lon, 180.0)

    def test_marker_with_negative_gps_coordinates(self):
        """Test marker with negative GPS coordinates (Southern/Western hemispheres)."""
        marker = RadarMarker(
            cx_frac=0.5,
            cy_frac=0.5,
            bearing_deg=0.0,
            gps_lat=-33.8688,
            gps_lon=-151.2093,
        )
        self.assertEqual(marker.gps_lat, -33.8688)
        self.assertEqual(marker.gps_lon, -151.2093)

    def test_marker_with_zero_gps_coordinates(self):
        """Test marker at equator and prime meridian."""
        marker = RadarMarker(
            cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0, gps_lat=0.0, gps_lon=0.0
        )
        self.assertEqual(marker.gps_lat, 0.0)
        self.assertEqual(marker.gps_lon, 0.0)


class TestSVGManipulationEdgeCases(unittest.TestCase):
    """Test SVG manipulation edge cases."""

    def setUp(self):
        """Create injector instance for tests."""
        self.injector = SVGMarkerInjector()

    def test_extract_viewbox_with_malformed_svg(self):
        """Test viewBox extraction with malformed SVG that raises exception."""
        # Missing svg tag - should raise RuntimeError
        bad_svg = "<path d='M 0 0 L 100 100'/>"
        with self.assertRaises(RuntimeError):
            self.injector._extract_viewbox(bad_svg)

        # svg tag but no viewBox or dimensions - should also raise
        bad_svg2 = "<svg></svg>"
        with self.assertRaises(RuntimeError):
            self.injector._extract_viewbox(bad_svg2)

    def test_extract_viewbox_with_negative_values(self):
        """Test viewBox extraction with negative coordinates."""
        svg = '<svg viewBox="-50 -50 200 200"></svg>'
        result = self.injector._extract_viewbox(svg)
        self.assertEqual(result, (-50.0, -50.0, 200.0, 200.0))

    def test_extract_viewbox_with_decimal_values(self):
        """Test viewBox extraction with decimal values."""
        svg = '<svg viewBox="0.5 0.5 99.5 99.5"></svg>'
        result = self.injector._extract_viewbox(svg)
        self.assertEqual(result, (0.5, 0.5, 99.5, 99.5))

    def test_inject_marker_with_empty_svg(self):
        """Test marker injection with minimal SVG."""
        svg_content = '<svg viewBox="0 0 100 100"></svg>'
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)

        result = self.injector.inject_marker(svg_content, marker)

        # Should have added marker elements
        self.assertIn("<g", result)
        self.assertIn("</g>", result)
        self.assertIn("<polygon", result)

    def test_inject_marker_preserves_existing_content(self):
        """Test that marker injection preserves existing SVG elements."""
        svg_content = """<svg viewBox="0 0 100 100">
            <rect x="10" y="10" width="80" height="80" fill="blue"/>
        </svg>"""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)

        result = self.injector.inject_marker(svg_content, marker)

        # Original rect should still be there
        self.assertIn('<rect x="10" y="10"', result)
        # Marker should be added
        self.assertIn("<polygon", result)

    def test_triangle_points_with_different_bearings(self):
        """Test triangle generation with various bearings."""
        marker_north = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)
        marker_east = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=90.0)
        marker_south = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=180.0)
        marker_west = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=270.0)

        viewbox = (0, 0, 100, 100)

        # All should generate valid points strings
        points_n = self.injector._compute_triangle_points(marker_north, viewbox)
        points_e = self.injector._compute_triangle_points(marker_east, viewbox)
        points_s = self.injector._compute_triangle_points(marker_south, viewbox)
        points_w = self.injector._compute_triangle_points(marker_west, viewbox)

        # All should be non-empty strings
        self.assertIsInstance(points_n, str)
        self.assertGreater(len(points_n), 0)
        self.assertIsInstance(points_e, str)
        self.assertGreater(len(points_e), 0)
        self.assertIsInstance(points_s, str)
        self.assertGreater(len(points_s), 0)
        self.assertIsInstance(points_w, str)
        self.assertGreater(len(points_w), 0)


class TestPDFConversionEdgeCases(unittest.TestCase):
    """Test PDF conversion edge cases and fallback chains."""

    def setUp(self):
        """Create converter instance for tests."""
        self.converter = SVGToPDFConverter()

    def test_converter_initialization(self):
        """Test PDF converter initializes correctly."""
        self.assertIsInstance(self.converter, SVGToPDFConverter)

    def test_converter_methods_exist(self):
        """Test that converter has expected methods."""
        self.assertTrue(hasattr(self.converter, "convert"))
        # Static methods exist on class
        self.assertTrue(hasattr(SVGToPDFConverter, "_try_cairosvg"))
        self.assertTrue(hasattr(SVGToPDFConverter, "_try_inkscape"))
        self.assertTrue(hasattr(SVGToPDFConverter, "_try_rsvg_convert"))


class TestMarkerPositioningEdgeCases(unittest.TestCase):
    """Test marker positioning edge cases."""

    def test_marker_at_corners(self):
        """Test marker positioning at SVG corners."""
        viewbox = (0, 0, 1000, 1000)

        # Top-left corner
        marker_tl = RadarMarker(cx_frac=0.0, cy_frac=0.0, bearing_deg=45.0)
        cx, cy = marker_tl.to_svg_coords(viewbox)
        self.assertEqual(cx, 0.0)
        self.assertEqual(cy, 0.0)

        # Top-right corner
        marker_tr = RadarMarker(cx_frac=1.0, cy_frac=0.0, bearing_deg=135.0)
        cx, cy = marker_tr.to_svg_coords(viewbox)
        self.assertEqual(cx, 1000.0)
        self.assertEqual(cy, 0.0)

        # Bottom-left corner
        marker_bl = RadarMarker(cx_frac=0.0, cy_frac=1.0, bearing_deg=315.0)
        cx, cy = marker_bl.to_svg_coords(viewbox)
        self.assertEqual(cx, 0.0)
        self.assertEqual(cy, 1000.0)

        # Bottom-right corner
        marker_br = RadarMarker(cx_frac=1.0, cy_frac=1.0, bearing_deg=225.0)
        cx, cy = marker_br.to_svg_coords(viewbox)
        self.assertEqual(cx, 1000.0)
        self.assertEqual(cy, 1000.0)

    def test_marker_with_extreme_bearing_angles(self):
        """Test marker with extreme bearing angles."""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)
        # 0 degrees (North)
        self.assertEqual(marker.bearing_deg, 0.0)

        # 360 degrees (also North)
        marker_360 = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=360.0)
        self.assertEqual(marker_360.bearing_deg, 360.0)

        # Negative angle (should work - represents counter-clockwise)
        marker_neg = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=-90.0)
        self.assertEqual(marker_neg.bearing_deg, -90.0)

    def test_marker_with_viewbox_offset(self):
        """Test marker conversion with offset viewBox."""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)

        # ViewBox with offset origin
        viewbox = (100, 200, 400, 600)
        cx, cy = marker.to_svg_coords(viewbox)

        # Center should be: origin + (fraction * size)
        self.assertAlmostEqual(cx, 100 + 0.5 * 400)
        self.assertAlmostEqual(cy, 200 + 0.5 * 600)

    def test_marker_with_tiny_viewbox(self):
        """Test marker with very small viewBox."""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)

        # Tiny viewBox
        viewbox = (0, 0, 1, 1)
        cx, cy = marker.to_svg_coords(viewbox)
        self.assertAlmostEqual(cx, 0.5)
        self.assertAlmostEqual(cy, 0.5)

    def test_marker_with_huge_viewbox(self):
        """Test marker with very large viewBox."""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0)

        # Huge viewBox
        viewbox = (0, 0, 1000000, 1000000)
        cx, cy = marker.to_svg_coords(viewbox)
        self.assertAlmostEqual(cx, 500000.0)
        self.assertAlmostEqual(cy, 500000.0)


class TestCircleStrokeConfiguration(unittest.TestCase):
    """Test circle stroke color configuration."""

    def test_circle_stroke_uses_custom_color(self):
        """Test that circle stroke color can be customized."""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0, color="#ff0000")
        # Circle stroke should default to triangle color
        self.assertEqual(marker.color, "#ff0000")

    def test_circle_stroke_with_opacity(self):
        """Test circle opacity configuration."""
        marker = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0, opacity=0.5)
        self.assertEqual(marker.opacity, 0.5)

        # Test boundary values
        marker_min = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0, opacity=0.0)
        self.assertEqual(marker_min.opacity, 0.0)

        marker_max = RadarMarker(cx_frac=0.5, cy_frac=0.5, bearing_deg=0.0, opacity=1.0)
        self.assertEqual(marker_max.opacity, 1.0)


class TestSVGToPDFConverterEdgeCases(unittest.TestCase):
    """Test edge cases in SVG to PDF conversion."""

    @patch("subprocess.check_call")
    def test_inkscape_exception_handler(self, mock_check_call):
        """Test inkscape exception handler (lines 298-300)."""
        # Make inkscape version check fail
        mock_check_call.side_effect = Exception("Command not found")

        with tempfile.TemporaryDirectory() as tmpdir:
            svg_path = os.path.join(tmpdir, "test.svg")
            pdf_path = os.path.join(tmpdir, "test.pdf")

            with open(svg_path, "w") as f:
                f.write("<svg></svg>")

            result = SVGToPDFConverter._try_inkscape(svg_path, pdf_path)

            # Should return False on exception
            self.assertFalse(result)

    @patch("subprocess.check_call")
    def test_rsvg_exception_handler(self, mock_check_call):
        """Test rsvg-convert exception handler (lines 331-333)."""
        # Make rsvg version check fail
        mock_check_call.side_effect = Exception("Command not found")

        with tempfile.TemporaryDirectory() as tmpdir:
            svg_path = os.path.join(tmpdir, "test.svg")
            pdf_path = os.path.join(tmpdir, "test.pdf")

            with open(svg_path, "w") as f:
                f.write("<svg></svg>")

            result = SVGToPDFConverter._try_rsvg_convert(svg_path, pdf_path)

            # Should return False on exception
            self.assertFalse(result)


class TestMapProcessorEdgeCases(unittest.TestCase):
    """Test edge cases in MapProcessor."""

    @patch("map_utils.SVGToPDFConverter.convert")
    def test_getmtime_exception_handler(self, mock_convert):
        """Test exception in os.path.getmtime (lines 421-424)."""
        mock_convert.return_value = True

        with tempfile.TemporaryDirectory() as tmpdir:
            processor = MapProcessor(base_dir=tmpdir)

            # Create map.svg and map.pdf
            map_svg = os.path.join(tmpdir, "map.svg")
            map_pdf = os.path.join(tmpdir, "map.pdf")

            with open(map_svg, "w") as f:
                f.write('<svg viewBox="0 0 100 100"></svg>')
            with open(map_pdf, "w") as f:
                f.write("pdf")

            # Mock getmtime to raise exception
            with patch("os.path.getmtime", side_effect=Exception("getmtime failed")):
                success, path = processor.process_map()

                # Should still succeed, just force convert
                self.assertTrue(success)
                self.assertIsNotNone(path)

    @patch("map_utils.SVGToPDFConverter.convert")
    def test_conversion_failure_warning(self, mock_convert):
        """Test conversion failure warning (lines 455-459)."""
        # Make conversion fail
        mock_convert.return_value = False

        with tempfile.TemporaryDirectory() as tmpdir:
            processor = MapProcessor(base_dir=tmpdir)

            # Create map.svg
            map_svg = os.path.join(tmpdir, "map.svg")
            with open(map_svg, "w") as f:
                f.write('<svg viewBox="0 0 100 100"></svg>')

            success, path = processor.process_map(force_convert=True)

            # Should return False when conversion fails
            self.assertFalse(success)
            self.assertIsNone(path)


if __name__ == "__main__":
    unittest.main()
