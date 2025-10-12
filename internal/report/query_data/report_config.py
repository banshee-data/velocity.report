#!/usr/bin/env python3
"""Configuration constants for PDF report generation.

⚠️  DEPRECATED: This module is deprecated. Use config_manager.ReportConfig instead.

This module provides backward compatibility for legacy code that imports
configuration dictionaries. All new code should use the JSON-based configuration
system in config_manager.py.

Migration Guide:
    See docs/REFACTOR_CONFIG_CONSOLIDATION.md for migration instructions.

    Old (Deprecated):
        from report_config import COLORS, FONTS, SITE_INFO
        color = COLORS["p50"]

    New (Recommended):
        from config_manager import ReportConfig
        config = ReportConfig.from_json("config.json")
        color = config.colors.p50

Legacy Behavior:
- Configuration dictionaries use hardcoded defaults (no environment variables)
- Use helper functions (get_config, override_site_info, get_map_config_with_overrides)
  for runtime customization
"""

import warnings
from typing import Dict, Any
from config_manager import (
    DEFAULT_COLOR_CONFIG,
    DEFAULT_FONT_CONFIG,
    DEFAULT_LAYOUT_CONFIG,
    DEFAULT_PDF_CONFIG,
    DEFAULT_MAP_CONFIG,
    DEFAULT_HISTOGRAM_PROCESSING_CONFIG,
    DEFAULT_SITE_CONFIG,
    DEFAULT_DEBUG_CONFIG,
    _colors_to_dict,
    _fonts_to_dict,
    _layout_to_dict,
    _pdf_to_dict,
    _map_to_dict,
    _histogram_processing_to_dict,
    _debug_to_dict,
    _site_to_dict,
)

# Issue deprecation warning when module is imported
warnings.warn(
    "report_config module is deprecated. Use config_manager.ReportConfig instead. "
    "See docs/REFACTOR_CONFIG_CONSOLIDATION.md for migration guide.",
    DeprecationWarning,
    stacklevel=2,
)


# =============================================================================
# Color Palette
# =============================================================================

# Use config_manager as single source of truth
COLORS: Dict[str, str] = _colors_to_dict(DEFAULT_COLOR_CONFIG)


# =============================================================================
# Font Sizes
# =============================================================================

# Use config_manager as single source of truth
FONTS: Dict[str, int] = _fonts_to_dict(DEFAULT_FONT_CONFIG)


# =============================================================================
# Layout Constants
# =============================================================================

# Use config_manager as single source of truth
LAYOUT: Dict[str, Any] = _layout_to_dict(DEFAULT_LAYOUT_CONFIG)


# =============================================================================
# Site Information
# =============================================================================
# These are defaults and can be overridden via CLI arguments or JSON configuration

# Use config_manager as single source of truth, but with example values for backward compat
from config_manager import SiteConfig

_EXAMPLE_SITE_CONFIG = SiteConfig(
    location="Clarendon Avenue, San Francisco",
    surveyor="Banshee, INC.",
    contact="david@banshee-data.com",
    speed_limit=25,
    site_description=(
        "This survey was conducted from the southbound parking lane outside "
        "500 Clarendon Avenue, directly in front of an elementary school. "
        "The site is located on a downhill grade, which may influence vehicle "
        "speed and braking behavior. Data was collected from a fixed position "
        "over three consecutive days."
    ),
    speed_limit_note=(
        "The posted speed limit at this location is 35 mph, reduced to 25 mph "
        "when school children are present."
    ),
)
SITE_INFO: Dict[str, Any] = _site_to_dict(_EXAMPLE_SITE_CONFIG)


# =============================================================================
# PDF/LaTeX Document Settings
# =============================================================================

# Use config_manager as single source of truth
PDF_CONFIG: Dict[str, Any] = _pdf_to_dict(DEFAULT_PDF_CONFIG)


# =============================================================================
# Map/SVG Marker Configuration
# =============================================================================

# Use config_manager as single source of truth
MAP_CONFIG = _map_to_dict(DEFAULT_MAP_CONFIG)


# =============================================================================
# Histogram Processing
# =============================================================================

# Use config_manager as single source of truth
HISTOGRAM_CONFIG: Dict[str, Any] = _histogram_processing_to_dict(
    DEFAULT_HISTOGRAM_PROCESSING_CONFIG
)


# =============================================================================
# Debug Settings
# =============================================================================

# Use config_manager as single source of truth
DEBUG: Dict[str, bool] = _debug_to_dict(DEFAULT_DEBUG_CONFIG)


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
    # Start with defaults from config_manager
    config = _map_to_dict(DEFAULT_MAP_CONFIG)

    # Apply overrides
    for key, value in kwargs.items():
        if key in config:
            config[key] = value
        else:
            raise ValueError(f"Unknown map_config key: {key}")

    # If triangle_color was changed but circle_stroke wasn't explicitly set,
    # update circle_stroke to match the new triangle_color
    if "triangle_color" in kwargs and "circle_stroke" not in kwargs:
        config["circle_stroke"] = config["triangle_color"]

    return config
