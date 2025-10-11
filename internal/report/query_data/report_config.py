#!/usr/bin/env python3
"""Configuration constants for PDF report generation.

This module centralizes all configuration values used across the report generation
workflow, including colors, font sizes, layout dimensions, and site-specific content.
Environment variables can override some values for flexibility.

Note on Configuration Patterns:
- Most config dictionaries are immutable after definition
- MAP_CONFIG uses computed defaults to avoid post-definition mutation
- Use helper functions (get_config, override_site_info, get_map_config_with_overrides)
  for runtime customization
"""

import os
from typing import Dict, Any


# =============================================================================
# Color Palette
# =============================================================================

COLORS: Dict[str, str] = {
    # Speed percentile line colors
    "p50": "#fbd92f",  # Yellow
    "p85": "#f7b32b",  # Orange
    "p98": "#f25f5c",  # Red/Pink
    "max": "#2d1e2f",  # Dark purple/black
    # Chart elements
    "count_bar": "#2d1e2f",  # Gray bars for counts
    "low_sample": "#f7b32b",  # Orange highlight for low-sample periods
}


# =============================================================================
# Font Sizes
# =============================================================================

FONTS: Dict[str, int] = {
    # Chart title and labels (matplotlib)
    "chart_title": 14,
    "chart_label": 13,
    "chart_tick": 11,
    "chart_axis_label": 8,
    "chart_axis_tick": 7,
    "chart_legend": 7,
    # Histogram-specific
    "histogram_title": 14,
    "histogram_label": 13,
    "histogram_tick": 11,
}


# =============================================================================
# Layout Constants
# =============================================================================

LAYOUT: Dict[str, Any] = {
    # Figure dimensions (inches)
    "chart_figsize": (24, 8),
    "histogram_figsize": (3, 2),
    # Thresholds
    "low_sample_threshold": 50,  # Counts below this trigger orange highlight
    "count_missing_threshold": 5,  # Counts below this are treated as missing data
    # Bar chart widths (as fractions of bucket spacing)
    "bar_width_bg_fraction": 0.95,  # Background highlight bars
    "bar_width_fraction": 0.7,  # Primary count bars
    # Chart sizing constraints
    "min_chart_width_in": 6.0,  # Minimum PDF width (inches)
    "max_chart_width_in": 11.0,  # Maximum PDF width (inches)
    # Chart margins and spacing (for fig.subplots_adjust)
    "chart_left": 0.02,
    "chart_right": 0.96,
    "chart_top": 0.995,
    "chart_bottom": 0.16,
    # Y-axis scaling
    "count_axis_scale": 1.6,  # Scale count axis by this factor for headroom
    # Line and marker styling
    "line_width": 1.0,
    "marker_size": 4,
    "marker_edge_width": 0.4,
}


# =============================================================================
# Site Information
# =============================================================================
# These are defaults and can be overridden via CLI arguments or environment variables

SITE_INFO: Dict[str, Any] = {
    "location": os.getenv("REPORT_LOCATION", "Clarendon Avenue, San Francisco"),
    "surveyor": os.getenv("REPORT_SURVEYOR", "Banshee, INC."),
    "contact": os.getenv("REPORT_CONTACT", "david@banshee-data.com"),
    "speed_limit": int(os.getenv("REPORT_SPEED_LIMIT", "25")),
    # Site-specific narrative content
    "site_description": (
        "This survey was conducted from the southbound parking lane outside "
        "500 Clarendon Avenue, directly in front of an elementary school. "
        "The site is located on a downhill grade, which may influence vehicle "
        "speed and braking behavior. Data was collected from a fixed position "
        "over three consecutive days."
    ),
    "speed_limit_note": (
        "The posted speed limit at this location is 35 mph, reduced to 25 mph "
        "when school children are present."
    ),
}


# =============================================================================
# PDF/LaTeX Document Settings
# =============================================================================

PDF_CONFIG: Dict[str, Any] = {
    # Page geometry (margins in cm)
    "geometry": {
        "top": "1.8cm",
        "bottom": "1.0cm",
        "left": "1.0cm",
        "right": "1.0cm",
    },
    # Column separation
    "columnsep": os.getenv("REPORT_COLUMNSEP_PT", "14"),  # Points
    # Header/footer spacing
    "headheight": "12pt",
    "headsep": "10pt",
    # Fonts directory (relative to pdf_generator.py)
    "fonts_dir": "fonts",
}


# =============================================================================
# Map/SVG Marker Configuration
# =============================================================================

# Base map configuration values (immutable)
_MAP_CONFIG_BASE: Dict[str, Any] = {
    # Triangle marker properties
    "triangle_len": float(os.getenv("MAP_TRIANGLE_LEN", "0.42")),
    "triangle_cx": float(os.getenv("MAP_TRIANGLE_CX", "0.385")),
    "triangle_cy": float(os.getenv("MAP_TRIANGLE_CY", "0.71")),
    "triangle_apex_angle": float(os.getenv("MAP_TRIANGLE_APEX_ANGLE", "20")),
    "triangle_angle": float(os.getenv("MAP_TRIANGLE_ANGLE", "32")),
    "triangle_color": os.getenv("MAP_TRIANGLE_COLOR", "#f25f5c"),
    "triangle_opacity": float(os.getenv("MAP_TRIANGLE_OPACITY", "0.9")),
    # Circle marker at triangle apex
    "circle_radius": float(os.getenv("MAP_TRIANGLE_CIRCLE_RADIUS", "20")),
    "circle_fill": os.getenv("MAP_TRIANGLE_CIRCLE_FILL", "#ffffff"),
    "circle_stroke": os.getenv("MAP_TRIANGLE_CIRCLE_STROKE", None),
    "circle_stroke_width": os.getenv("MAP_TRIANGLE_CIRCLE_STROKE_WIDTH", "2"),
}


def _get_map_config() -> Dict[str, Any]:
    """Get map configuration with computed defaults.

    Dynamically computes circle_stroke default to match triangle_color if not
    explicitly set via environment variable. This avoids modifying the config
    dictionary after definition, which could lead to bugs.

    Returns:
        Map configuration dictionary with all defaults resolved
    """
    config = _MAP_CONFIG_BASE.copy()

    # Compute circle_stroke default: use triangle_color if not explicitly set
    if config["circle_stroke"] is None:
        config["circle_stroke"] = config["triangle_color"]

    return config


# Public map configuration (computed on first access)
MAP_CONFIG = _get_map_config()


# =============================================================================
# Histogram Processing
# =============================================================================

HISTOGRAM_CONFIG: Dict[str, Any] = {
    "default_cutoff": 5.0,
    "default_bucket_size": 5.0,
    "default_max_bucket": 50.0,
}


# =============================================================================
# Debug Settings
# =============================================================================

DEBUG: Dict[str, bool] = {
    "plot_debug": os.getenv("VELOCITY_PLOT_DEBUG", "0") == "1",
}


# =============================================================================
# Helper Functions
# =============================================================================


def get_config() -> Dict[str, Any]:
    """Get complete configuration as a single dictionary.

    This is useful for passing config to classes/functions that need
    multiple configuration sections.
    """
    return {
        "COLORS": COLORS,
        "FONTS": FONTS,
        "LAYOUT": LAYOUT,
        "SITE_INFO": SITE_INFO,
        "PDF_CONFIG": PDF_CONFIG,
        "MAP_CONFIG": MAP_CONFIG,
        "HISTOGRAM_CONFIG": HISTOGRAM_CONFIG,
        "DEBUG": DEBUG,
    }


def override_site_info(**kwargs) -> None:
    """Override site information at runtime.

    Usage:
        override_site_info(location='Main Street', speed_limit=30)
    """
    for key, value in kwargs.items():
        if key in SITE_INFO:
            SITE_INFO[key] = value
        else:
            raise ValueError(f"Unknown site_info key: {key}")


def get_map_config_with_overrides(**kwargs) -> Dict[str, Any]:
    """Get map configuration with custom overrides.

    This creates a fresh map configuration dictionary with any specified
    overrides applied. The circle_stroke default is still computed dynamically
    if not explicitly provided.

    Args:
        **kwargs: Key-value pairs to override in the map configuration

    Returns:
        Map configuration dictionary with overrides and computed defaults

    Raises:
        ValueError: If an unknown configuration key is provided

    Usage:
        config = get_map_config_with_overrides(
            triangle_color='#0000ff',
            triangle_angle=45.0
        )
    """
    # Start with base config
    config = _MAP_CONFIG_BASE.copy()

    # Apply overrides
    for key, value in kwargs.items():
        if key in config:
            config[key] = value
        else:
            raise ValueError(f"Unknown map_config key: {key}")

    # Compute circle_stroke default if not explicitly set
    if config["circle_stroke"] is None:
        config["circle_stroke"] = config["triangle_color"]

    return config
