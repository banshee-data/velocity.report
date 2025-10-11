#!/usr/bin/env python3
"""Unit tests for report_config.py configuration module."""

import unittest

# Import all config sections
from report_config import (
    COLORS,
    FONTS,
    LAYOUT,
    SITE_INFO,
    PDF_CONFIG,
    MAP_CONFIG,
    HISTOGRAM_CONFIG,
    DEBUG,
    get_config,
    override_site_info,
)


class TestColorsConfig(unittest.TestCase):
    """Tests for COLORS configuration."""

    def test_has_all_required_colors(self):
        """Verify all required color keys exist."""
        required = ["p50", "p85", "p98", "max", "count_bar", "low_sample"]
        for key in required:
            self.assertIn(key, COLORS, f"Missing color key: {key}")

    def test_colors_are_hex_strings(self):
        """Verify all colors are valid hex color strings."""
        for key, color in COLORS.items():
            self.assertIsInstance(color, str, f"{key} should be a string")
            self.assertTrue(color.startswith("#"), f"{key} color should start with #")
            self.assertIn(
                len(color), [4, 7], f"{key} color should be #RGB or #RRGGBB format"
            )
            # Verify hex characters
            hex_chars = set("0123456789abcdefABCDEF")
            for char in color[1:]:
                self.assertIn(
                    char, hex_chars, f"{key} color has invalid hex character: {char}"
                )

    def test_percentile_colors_distinct(self):
        """Verify percentile colors are distinct from each other."""
        percentile_colors = [COLORS["p50"], COLORS["p85"], COLORS["p98"]]
        self.assertEqual(
            len(set(percentile_colors)), 3, "Percentile colors should be distinct"
        )


class TestFontsConfig(unittest.TestCase):
    """Tests for FONTS configuration."""

    def test_has_required_font_sizes(self):
        """Verify all required font size keys exist."""
        required = [
            "chart_title",
            "chart_label",
            "chart_tick",
            "chart_axis_label",
            "chart_axis_tick",
            "chart_legend",
            "histogram_title",
            "histogram_label",
            "histogram_tick",
        ]
        for key in required:
            self.assertIn(key, FONTS, f"Missing font key: {key}")

    def test_font_sizes_are_positive_integers(self):
        """Verify all font sizes are positive integers."""
        for key, size in FONTS.items():
            self.assertIsInstance(size, int, f"{key} should be an integer")
            self.assertGreater(size, 0, f"{key} should be positive")

    def test_font_sizes_reasonable_range(self):
        """Verify font sizes are in reasonable range (5-20pt)."""
        for key, size in FONTS.items():
            self.assertGreaterEqual(size, 5, f"{key} too small: {size}")
            self.assertLessEqual(size, 20, f"{key} too large: {size}")


class TestLayoutConfig(unittest.TestCase):
    """Tests for LAYOUT configuration."""

    def test_has_chart_dimensions(self):
        """Verify chart dimension configuration exists."""
        self.assertIn("chart_figsize", LAYOUT)
        self.assertIn("histogram_figsize", LAYOUT)

    def test_figsize_format(self):
        """Verify figsize values are tuples of two numbers."""
        for key in ["chart_figsize", "histogram_figsize"]:
            figsize = LAYOUT[key]
            self.assertIsInstance(figsize, tuple, f"{key} should be a tuple")
            self.assertEqual(len(figsize), 2, f"{key} should have 2 elements")
            self.assertTrue(
                all(isinstance(x, (int, float)) and x > 0 for x in figsize),
                f"{key} should contain positive numbers",
            )

    def test_thresholds_are_positive(self):
        """Verify threshold values are positive."""
        thresholds = ["low_sample_threshold", "count_missing_threshold"]
        for key in thresholds:
            self.assertIn(key, LAYOUT)
            self.assertGreater(LAYOUT[key], 0, f"{key} should be positive")

    def test_fractions_in_valid_range(self):
        """Verify fraction values are between 0 and 1."""
        fractions = ["bar_width_bg_fraction", "bar_width_fraction"]
        for key in fractions:
            self.assertIn(key, LAYOUT)
            value = LAYOUT[key]
            self.assertGreater(value, 0, f"{key} should be > 0")
            self.assertLessEqual(value, 1.0, f"{key} should be <= 1.0")

    def test_margins_in_valid_range(self):
        """Verify chart margins are between 0 and 1."""
        margins = ["chart_left", "chart_right", "chart_top", "chart_bottom"]
        for key in margins:
            self.assertIn(key, LAYOUT)
            value = LAYOUT[key]
            self.assertGreaterEqual(value, 0, f"{key} should be >= 0")
            self.assertLessEqual(value, 1.0, f"{key} should be <= 1.0")


class TestSiteInfoConfig(unittest.TestCase):
    """Tests for SITE_INFO configuration."""

    def test_has_required_fields(self):
        """Verify all required site info fields exist."""
        required = [
            "location",
            "surveyor",
            "contact",
            "speed_limit",
            "site_description",
            "speed_limit_note",
        ]
        for key in required:
            self.assertIn(key, SITE_INFO, f"Missing site_info key: {key}")

    def test_contact_looks_like_email(self):
        """Verify contact contains @ character (basic email check)."""
        contact = SITE_INFO["contact"]
        self.assertIsInstance(contact, str)
        self.assertIn("@", contact, "Contact should contain @ for email")

    def test_speed_limit_is_positive_int(self):
        """Verify speed limit is a positive integer."""
        speed_limit = SITE_INFO["speed_limit"]
        self.assertIsInstance(speed_limit, int)
        self.assertGreater(speed_limit, 0)

    def test_text_fields_are_strings(self):
        """Verify text fields are non-empty strings."""
        text_fields = ["location", "surveyor", "site_description", "speed_limit_note"]
        for key in text_fields:
            value = SITE_INFO[key]
            self.assertIsInstance(value, str, f"{key} should be a string")
            self.assertTrue(len(value) > 0, f"{key} should not be empty")


class TestPDFConfig(unittest.TestCase):
    """Tests for PDF_CONFIG configuration."""

    def test_has_required_keys(self):
        """Verify all required PDF config keys exist."""
        required = ["geometry", "columnsep", "headheight", "headsep", "fonts_dir"]
        for key in required:
            self.assertIn(key, PDF_CONFIG, f"Missing PDF_CONFIG key: {key}")

    def test_geometry_is_dict(self):
        """Verify geometry is a dictionary with margin keys."""
        geometry = PDF_CONFIG["geometry"]
        self.assertIsInstance(geometry, dict)
        required_margins = ["top", "bottom", "left", "right"]
        for margin in required_margins:
            self.assertIn(margin, geometry, f"Missing geometry key: {margin}")
            # Should be string with 'cm' or 'pt' unit
            self.assertIsInstance(geometry[margin], str)

    def test_spacing_values_are_strings(self):
        """Verify spacing values are strings with units."""
        spacing_keys = ["columnsep", "headheight", "headsep"]
        for key in spacing_keys:
            value = PDF_CONFIG[key]
            self.assertIsInstance(value, str)

    def test_fonts_dir_is_string(self):
        """Verify fonts_dir is a string."""
        self.assertIsInstance(PDF_CONFIG["fonts_dir"], str)
        self.assertTrue(len(PDF_CONFIG["fonts_dir"]) > 0)


class TestMapConfig(unittest.TestCase):
    """Tests for MAP_CONFIG configuration."""

    def test_has_triangle_config(self):
        """Verify triangle marker configuration exists."""
        triangle_keys = [
            "triangle_len",
            "triangle_cx",
            "triangle_cy",
            "triangle_apex_angle",
            "triangle_angle",
            "triangle_color",
            "triangle_opacity",
        ]
        for key in triangle_keys:
            self.assertIn(key, MAP_CONFIG, f"Missing MAP_CONFIG key: {key}")

    def test_has_circle_config(self):
        """Verify circle marker configuration exists."""
        circle_keys = [
            "circle_radius",
            "circle_fill",
            "circle_stroke",
            "circle_stroke_width",
        ]
        for key in circle_keys:
            self.assertIn(key, MAP_CONFIG, f"Missing MAP_CONFIG key: {key}")

    def test_numeric_values_positive(self):
        """Verify numeric map config values are positive."""
        numeric_keys = [
            "triangle_len",
            "triangle_cx",
            "triangle_cy",
            "triangle_apex_angle",
            "triangle_angle",
            "circle_radius",
        ]
        for key in numeric_keys:
            value = MAP_CONFIG[key]
            self.assertIsInstance(value, (int, float), f"{key} should be numeric")
            self.assertGreater(value, 0, f"{key} should be positive")

    def test_opacity_in_valid_range(self):
        """Verify triangle opacity is between 0 and 1."""
        opacity = MAP_CONFIG["triangle_opacity"]
        self.assertGreaterEqual(opacity, 0.0)
        self.assertLessEqual(opacity, 1.0)

    def test_colors_are_hex(self):
        """Verify color values are hex strings."""
        color_keys = ["triangle_color", "circle_fill", "circle_stroke"]
        for key in color_keys:
            color = MAP_CONFIG[key]
            if color is not None:  # circle_stroke can be None initially
                self.assertIsInstance(color, str)
                self.assertTrue(color.startswith("#"))

    def test_circle_stroke_defaults_to_triangle_color(self):
        """Verify circle_stroke is set to triangle_color if not explicitly set."""
        # After module initialization, circle_stroke should match triangle_color
        self.assertEqual(MAP_CONFIG["circle_stroke"], MAP_CONFIG["triangle_color"])


class TestHistogramConfig(unittest.TestCase):
    """Tests for HISTOGRAM_CONFIG configuration."""

    def test_has_required_keys(self):
        """Verify all required histogram config keys exist."""
        required = ["default_cutoff", "default_bucket_size", "default_max_bucket"]
        for key in required:
            self.assertIn(key, HISTOGRAM_CONFIG, f"Missing HISTOGRAM_CONFIG key: {key}")

    def test_values_are_positive(self):
        """Verify histogram config values are positive numbers."""
        for key, value in HISTOGRAM_CONFIG.items():
            self.assertIsInstance(value, (int, float), f"{key} should be numeric")
            self.assertGreater(value, 0, f"{key} should be positive")


class TestDebugConfig(unittest.TestCase):
    """Tests for DEBUG configuration."""

    def test_has_plot_debug(self):
        """Verify plot_debug key exists."""
        self.assertIn("plot_debug", DEBUG)

    def test_plot_debug_is_bool(self):
        """Verify plot_debug is a boolean."""
        self.assertIsInstance(DEBUG["plot_debug"], bool)


class TestHelperFunctions(unittest.TestCase):
    """Tests for helper functions."""

    def test_get_config_returns_all_sections(self):
        """Verify get_config() returns all configuration sections."""
        config = get_config()
        self.assertIsInstance(config, dict)
        required_sections = [
            "COLORS",
            "FONTS",
            "LAYOUT",
            "SITE_INFO",
            "PDF_CONFIG",
            "MAP_CONFIG",
            "HISTOGRAM_CONFIG",
            "DEBUG",
        ]
        for section in required_sections:
            self.assertIn(section, config, f"Missing config section: {section}")

    def test_get_config_returns_actual_dicts(self):
        """Verify get_config() returns copies/references to config dicts."""
        config = get_config()
        # Verify it contains the COLORS data (equality, not identity)
        self.assertEqual(config["COLORS"], COLORS)
        self.assertEqual(config["FONTS"], FONTS)

    def test_override_site_info_updates_values(self):
        """Test override_site_info() updates SITE_INFO values."""
        # Store original values
        original_values = {
            "location": SITE_INFO["location"],
            "speed_limit": SITE_INFO["speed_limit"],
        }
        try:
            # Override values
            override_site_info(location="Test Location", speed_limit=30)
            # Check they were updated (access via dict reference)
            import report_config

            self.assertEqual(report_config.SITE_INFO["location"], "Test Location")
            self.assertEqual(report_config.SITE_INFO["speed_limit"], 30)
        finally:
            # Restore original values
            override_site_info(**original_values)

    def test_override_site_info_rejects_invalid_keys(self):
        """Test override_site_info() raises error for invalid keys."""
        with self.assertRaises(ValueError) as context:
            override_site_info(invalid_key="value")
        self.assertIn("Unknown site_info key", str(context.exception))

    def test_get_map_config_with_overrides_basic(self):
        """Test get_map_config_with_overrides() with simple override."""
        from report_config import get_map_config_with_overrides

        config = get_map_config_with_overrides(triangle_color="#0000ff")

        # Verify override applied
        self.assertEqual(config["triangle_color"], "#0000ff")
        # Verify circle_stroke follows triangle_color
        self.assertEqual(config["circle_stroke"], "#0000ff")

    def test_get_map_config_with_overrides_explicit_stroke(self):
        """Test get_map_config_with_overrides() with explicit circle_stroke."""
        from report_config import get_map_config_with_overrides

        config = get_map_config_with_overrides(
            triangle_color="#ff0000", circle_stroke="#00ff00"
        )

        # Verify both overrides applied independently
        self.assertEqual(config["triangle_color"], "#ff0000")
        self.assertEqual(config["circle_stroke"], "#00ff00")

    def test_get_map_config_with_overrides_invalid_key(self):
        """Test get_map_config_with_overrides() rejects invalid keys."""
        from report_config import get_map_config_with_overrides

        with self.assertRaises(ValueError) as context:
            get_map_config_with_overrides(invalid_key="value")
        self.assertIn("Unknown map_config key", str(context.exception))


if __name__ == "__main__":
    unittest.main()
