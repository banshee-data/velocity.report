#!/usr/bin/env python3
"""Map utilities for radar sensor visualization.

This module handles map rendering with radar sensor markers, designed to be
forward-compatible with OpenStreetMap vector data retrieval. Currently works
with static SVG files but is structured to support future OSM integration with
GPS positioning, configurable zoom levels, and bounding boxes.

Future enhancements:
- OSM vector data download based on GPS coordinates and bounding box
- Dynamic map generation at configurable zoom levels
- Multiple sensor support with GPS positions and bearing angles
- Automatic viewBox calculation from GPS coordinates
"""

import os
import re
import math
import importlib.util
from typing import Dict, Any, Optional, Tuple


class RadarMarker:
    """Represents a radar sensor marker with GPS-compatible positioning.

    This class is designed for future GPS-based positioning while currently
    working with SVG fractional coordinates (0-1 range).

    Attributes:
        cx_frac: X-position as fraction of viewBox width (0-1)
        cy_frac: Y-position as fraction of viewBox height (0-1)
        bearing_deg: Bearing/heading angle in degrees (0=North, 90=East)
        coverage_length: Length of coverage triangle as fraction of viewBox height
        coverage_angle: Apex angle of coverage triangle in degrees
        color: Fill color for the marker
        opacity: Fill opacity (0-1)
        gps_lat: GPS latitude (reserved for future OSM integration)
        gps_lon: GPS longitude (reserved for future OSM integration)
    """

    def __init__(
        self,
        cx_frac: float,
        cy_frac: float,
        bearing_deg: float,
        coverage_length: float = 0.42,
        coverage_angle: float = 20.0,
        color: str = "#f25f5c",
        opacity: float = 0.9,
        gps_lat: Optional[float] = None,
        gps_lon: Optional[float] = None,
    ):
        """Initialize radar marker.

        Args:
            cx_frac: X-position as fraction of viewBox width (0-1)
            cy_frac: Y-position as fraction of viewBox height (0-1)
            bearing_deg: Bearing angle in degrees (0=North, clockwise)
            coverage_length: Triangle length as fraction of viewBox height
            coverage_angle: Triangle apex angle in degrees
            color: Marker fill color
            opacity: Marker fill opacity (0-1)
            gps_lat: GPS latitude (for future OSM integration)
            gps_lon: GPS longitude (for future OSM integration)
        """
        self.cx_frac = cx_frac
        self.cy_frac = cy_frac
        self.bearing_deg = bearing_deg
        self.coverage_length = coverage_length
        self.coverage_angle = coverage_angle
        self.color = color
        self.opacity = opacity
        self.gps_lat = gps_lat
        self.gps_lon = gps_lon

    def to_svg_coords(
        self, viewbox: Tuple[float, float, float, float]
    ) -> Tuple[float, float]:
        """Convert fractional position to SVG coordinates.

        Args:
            viewbox: SVG viewBox as (min_x, min_y, width, height)

        Returns:
            Tuple of (cx, cy) in SVG coordinate space
        """
        vb_min_x, vb_min_y, vb_w, vb_h = viewbox
        cx = vb_min_x + vb_w * self.cx_frac
        cy = vb_min_y + vb_h * self.cy_frac
        return cx, cy


class SVGMarkerInjector:
    """Injects radar markers into SVG map files.

    This class handles the SVG manipulation to add radar sensor visualization
    overlays. It's designed to work with static SVG files now, but structured
    to support future dynamic SVG generation from OSM vector data.
    """

    def __init__(
        self,
        circle_radius: float = 20.0,
        circle_fill: str = "#ffffff",
        circle_stroke: Optional[str] = None,
        circle_stroke_width: float = 2.0,
    ):
        """Initialize SVG marker injector.

        Args:
            circle_radius: Radius of position marker circle
            circle_fill: Fill color for position circle
            circle_stroke: Stroke color for position circle (None = use marker color)
            circle_stroke_width: Stroke width for position circle
        """
        self.circle_radius = circle_radius
        self.circle_fill = circle_fill
        self.circle_stroke = circle_stroke
        self.circle_stroke_width = circle_stroke_width

    def _extract_viewbox(self, svg_text: str) -> Tuple[float, float, float, float]:
        """Extract viewBox from SVG content.

        Args:
            svg_text: SVG file content as string

        Returns:
            Tuple of (min_x, min_y, width, height)

        Raises:
            RuntimeError: If viewBox cannot be determined
        """
        # Try viewBox attribute first
        vb_match = re.search(
            r"viewBox\s*=\s*[\"']\s*([0-9.+-eE]+)\s+([0-9.+-eE]+)\s+([0-9.+-eE]+)\s+([0-9.+-eE]+)\s*[\"']",
            svg_text,
        )
        if vb_match:
            return (
                float(vb_match.group(1)),
                float(vb_match.group(2)),
                float(vb_match.group(3)),
                float(vb_match.group(4)),
            )

        # Fallback to width/height attributes
        w_match = re.search(r"width\s*=\s*\"?([0-9.+-eE]+)", svg_text)
        h_match = re.search(r"height\s*=\s*\"?([0-9.+-eE]+)", svg_text)
        if w_match and h_match:
            return 0.0, 0.0, float(w_match.group(1)), float(h_match.group(1))

        raise RuntimeError("Unable to determine SVG viewBox/size for marker placement")

    def _compute_triangle_points(
        self,
        marker: RadarMarker,
        viewbox: Tuple[float, float, float, float],
    ) -> str:
        """Compute SVG points string for coverage triangle.

        Args:
            marker: RadarMarker instance with position and bearing
            viewbox: SVG viewBox as (min_x, min_y, width, height)

        Returns:
            SVG points string for polygon element
        """
        vb_min_x, vb_min_y, vb_w, vb_h = viewbox

        # Get position in SVG coordinates
        cx, cy = marker.to_svg_coords(viewbox)

        # Triangle length (from tip toward base)
        L = marker.coverage_length * vb_h

        # Compute base width from apex angle
        apex_deg = max(1.0, min(178.0, marker.coverage_angle))  # Clamp to valid range
        W = 2.0 * L * math.tan(math.radians(apex_deg / 2.0))

        # Convert bearing to radians (0=North, clockwise)
        theta = math.radians(marker.bearing_deg)

        # Forward direction (direction triangle points)
        fx = math.sin(theta)
        fy = -math.cos(theta)

        # Perpendicular direction (for base width)
        px = math.cos(theta)
        py = math.sin(theta)

        # Base center point
        bx = cx + fx * L
        by = cy + fy * L

        # Base left and right points
        blx = bx + px * (W / 2.0)
        bly = by + py * (W / 2.0)
        brx = bx - px * (W / 2.0)
        bry = by - py * (W / 2.0)

        # Return points string: tip, base-left, base-right
        return f"{cx:.2f},{cy:.2f} {blx:.2f},{bly:.2f} {brx:.2f},{bry:.2f}"

    def inject_marker(
        self,
        svg_text: str,
        marker: RadarMarker,
    ) -> str:
        """Inject radar marker into SVG content.

        Args:
            svg_text: Original SVG file content
            marker: RadarMarker instance to inject

        Returns:
            Modified SVG content with marker overlay

        Raises:
            RuntimeError: If SVG viewBox cannot be determined
        """
        # Extract viewBox for coordinate calculations
        viewbox = self._extract_viewbox(svg_text)

        # Compute triangle points
        points = self._compute_triangle_points(marker, viewbox)

        # Get position for circle marker
        cx, cy = marker.to_svg_coords(viewbox)

        # Use marker color for circle stroke if not explicitly set
        circle_stroke = self.circle_stroke or marker.color

        # Build SVG snippet for marker overlay
        insert_snippet = (
            f"\n  <!-- radar marker inserted by map_utils.py -->\n"
            f'  <g id="radar-marker" fill="{marker.color}" fill-opacity="{marker.opacity}" '
            f'stroke="#ffffff" stroke-width="1">\n'
            f'    <polygon points="{points}" />\n'
            f'    <circle cx="{cx:.2f}" cy="{cy:.2f}" r="{self.circle_radius}" '
            f'fill="{self.circle_fill}" stroke="{circle_stroke}" '
            f'stroke-width="{self.circle_stroke_width}" />\n'
            f"  </g>\n"
        )

        # Inject before closing </svg> tag
        if svg_text.strip().endswith("</svg>"):
            svg_text = svg_text.rstrip()[:-6] + insert_snippet + "</svg>"
        else:
            svg_text = svg_text + insert_snippet

        return svg_text


class SVGToPDFConverter:
    """Converts SVG files to PDF format using available tools.

    Tries multiple conversion methods in order of preference:
    1. cairosvg (Python library)
    2. inkscape (command-line tool)
    3. rsvg-convert (command-line tool)
    """

    @staticmethod
    def convert(
        svg_path: str,
        pdf_path: str,
    ) -> bool:
        """Convert SVG file to PDF.

        Args:
            svg_path: Path to input SVG file
            pdf_path: Path to output PDF file

        Returns:
            True if conversion succeeded, False otherwise
        """
        # Try cairosvg (Python-based, preferred)
        if SVGToPDFConverter._try_cairosvg(svg_path, pdf_path):
            return True

        # Try inkscape (command-line)
        if SVGToPDFConverter._try_inkscape(svg_path, pdf_path):
            return True

        # Try rsvg-convert (command-line)
        if SVGToPDFConverter._try_rsvg_convert(svg_path, pdf_path):
            return True

        return False

    @staticmethod
    def _try_cairosvg(svg_path: str, pdf_path: str) -> bool:
        """Try conversion using cairosvg Python library."""
        try:
            if importlib.util.find_spec("cairosvg") is not None:
                from cairosvg import svg2pdf

                with open(pdf_path, "wb") as out_f:
                    svg2pdf(url=svg_path, write_to=out_f)
                return True
        except Exception:
            pass
        return False

    @staticmethod
    def _try_inkscape(svg_path: str, pdf_path: str) -> bool:
        """Try conversion using inkscape command-line tool."""
        try:
            import subprocess

            # Check if inkscape is available
            subprocess.check_call(
                ["inkscape", "--version"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )

            # Convert using inkscape
            subprocess.check_call(
                [
                    "inkscape",
                    svg_path,
                    "--export-type=pdf",
                    "--export-filename",
                    pdf_path,
                ],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
            return True
        except Exception:
            pass
        return False

    @staticmethod
    def _try_rsvg_convert(svg_path: str, pdf_path: str) -> bool:
        """Try conversion using rsvg-convert command-line tool."""
        try:
            import subprocess

            # Check if rsvg-convert is available
            subprocess.check_call(
                ["rsvg-convert", "--version"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )

            # Convert using rsvg-convert
            with open(pdf_path, "wb") as out_f:
                subprocess.check_call(
                    ["rsvg-convert", "-f", "pdf", svg_path],
                    stdout=out_f,
                    stderr=subprocess.DEVNULL,
                )
            return True
        except Exception:
            pass
        return False


class MapProcessor:
    """High-level interface for map processing with radar markers.

    This class provides the main API for adding radar markers to maps and
    converting them to PDF format. It's designed to work with static SVG files
    now but structured for future OSM integration.

    Future enhancements will add:
    - OSM vector data download based on GPS coordinates
    - Dynamic bounding box calculation
    - Multi-sensor support
    - Zoom level configuration
    """

    def __init__(
        self,
        base_dir: Optional[str] = None,
        marker_config: Optional[Dict[str, Any]] = None,
    ):
        """Initialize map processor.

        Args:
            base_dir: Base directory for map files (defaults to module directory)
            marker_config: Configuration dict for marker appearance (circle properties)
        """
        self.base_dir = base_dir or os.path.dirname(__file__)

        # Initialize marker injector with config
        marker_config = marker_config or {}
        self.injector = SVGMarkerInjector(
            circle_radius=marker_config.get("circle_radius", 20.0),
            circle_fill=marker_config.get("circle_fill", "#ffffff"),
            circle_stroke=marker_config.get("circle_stroke", None),
            circle_stroke_width=marker_config.get("circle_stroke_width", 2.0),
        )

    def process_map(
        self,
        marker: Optional[RadarMarker] = None,
        force_convert: bool = False,
    ) -> Tuple[bool, Optional[str]]:
        """Process map SVG file, optionally adding marker and converting to PDF.

        Args:
            marker: RadarMarker to add (None = no marker overlay)
            force_convert: Force PDF conversion even if PDF exists and is current

        Returns:
            Tuple of (success, pdf_path or None)
        """
        map_svg = os.path.join(self.base_dir, "map.svg")
        map_pdf = os.path.join(self.base_dir, "map.pdf")

        # Check if source SVG exists
        if not os.path.exists(map_svg):
            return False, None

        # Determine if conversion is needed
        need_convert = force_convert or not os.path.exists(map_pdf)
        if not need_convert:
            try:
                need_convert = os.path.getmtime(map_svg) > os.path.getmtime(map_pdf)
            except Exception:
                need_convert = True

        # Determine source SVG for conversion
        source_svg = map_svg
        temp_svg = None

        # If marker is provided, create temporary SVG with marker overlay
        if marker is not None and marker.coverage_length > 0:
            try:
                with open(map_svg, "r", encoding="utf-8") as f:
                    svg_text = f.read()

                # Inject marker into SVG
                svg_with_marker = self.injector.inject_marker(svg_text, marker)

                # Write to temporary file
                temp_svg = os.path.join(self.base_dir, "map_with_marker.svg")
                with open(temp_svg, "w", encoding="utf-8") as f:
                    f.write(svg_with_marker)

                # Use temporary SVG as conversion source
                source_svg = temp_svg
                need_convert = True  # Force conversion when marker is added

            except Exception as e:
                print(f"Warning: failed to create map with marker overlay: {e}")
                source_svg = map_svg

        # Convert to PDF if needed
        if need_convert:
            if not SVGToPDFConverter.convert(source_svg, map_pdf):
                print(
                    "Warning: map.svg found but failed to convert to PDF; "
                    "skipping map inclusion"
                )
                return False, None

        # Return success if PDF exists
        if os.path.exists(map_pdf):
            return True, os.path.abspath(map_pdf)

        return False, None


def create_marker_from_config(config: Dict[str, Any]) -> RadarMarker:
    """Create RadarMarker instance from configuration dictionary.

    This is a convenience function for creating markers from the report_config
    MAP_CONFIG dictionary.

    Args:
        config: Configuration dict (typically MAP_CONFIG from report_config)

    Returns:
        RadarMarker instance
    """
    return RadarMarker(
        cx_frac=config.get("triangle_cx", 0.385),
        cy_frac=config.get("triangle_cy", 0.71),
        bearing_deg=config.get("triangle_angle", 32.0),
        coverage_length=config.get("triangle_len", 0.42),
        coverage_angle=config.get("triangle_apex_angle", 20.0),
        color=config.get("triangle_color", "#f25f5c"),
        opacity=config.get("triangle_opacity", 0.9),
    )


# =============================================================================
# Future OSM Integration (Placeholder)
# =============================================================================
# These functions are reserved for future OpenStreetMap integration


def download_osm_map(
    gps_lat: float,
    gps_lon: float,
    zoom_level: int = 16,
    bbox_margin_m: float = 500.0,
) -> Optional[str]:
    """Download OSM vector data for map generation.

    FUTURE IMPLEMENTATION: This will download OpenStreetMap vector data
    based on GPS coordinates and generate an SVG map.

    Args:
        gps_lat: Center latitude
        gps_lon: Center longitude
        zoom_level: OSM zoom level (1-19)
        bbox_margin_m: Bounding box margin in meters

    Returns:
        Path to generated SVG file, or None if download fails
    """
    raise NotImplementedError(
        "OSM vector data download not yet implemented. "
        "Use static map.svg files for now."
    )


def compute_viewbox_from_gps(
    center_lat: float,
    center_lon: float,
    margin_m: float = 500.0,
) -> Tuple[float, float, float, float]:
    """Compute SVG viewBox from GPS coordinates and margin.

    FUTURE IMPLEMENTATION: This will calculate appropriate SVG viewBox
    dimensions based on GPS coordinates and desired coverage area.

    Args:
        center_lat: Center latitude
        center_lon: Center longitude
        margin_m: Coverage margin in meters

    Returns:
        Tuple of (min_x, min_y, width, height) for SVG viewBox
    """
    raise NotImplementedError(
        "GPS-to-viewBox conversion not yet implemented. "
        "Use fractional coordinates (0-1) for now."
    )
