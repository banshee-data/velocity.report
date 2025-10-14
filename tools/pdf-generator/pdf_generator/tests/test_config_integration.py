#!/usr/bin/env python3
"""Integration tests for config value propagation.

This module tests that all configuration values from config files
properly propagate through the system to the generated reports.
"""

import unittest
import os
import tempfile
from unittest.mock import patch, MagicMock

try:
    import pylatex  # noqa: F401

    HAVE_PYLATEX = True
except ImportError:
    HAVE_PYLATEX = False

from pdf_generator.core.config_manager import (
    ReportConfig,
    RadarConfig,
    SiteConfig,
    QueryConfig,
    OutputConfig,
)
from pdf_generator.core.pdf_generator import generate_pdf_report


class TestConfigIntegration(unittest.TestCase):
    """Test that all config values properly propagate to generated reports."""

    def setUp(self):
        """Set up test fixtures."""
        if not HAVE_PYLATEX:
            self.skipTest("PyLaTeX not available")

        self.temp_dir = tempfile.mkdtemp()
        self.config = ReportConfig(
            site=SiteConfig(
                location="Test Location Alpha",
                surveyor="Test Surveyor Beta",
                contact="test@example.com",
                speed_limit=35,
                site_description="Test site description gamma",
                speed_limit_note="Test speed limit note delta",
            ),
            query=QueryConfig(
                start_date="2025-01-01",
                end_date="2025-01-07",
                group="1h",
                units="mph",
                timezone="US/Pacific",
            ),
            radar=RadarConfig(
                cosine_error_angle=15.0,
                sensor_model="Test Sensor Model Epsilon",
                firmware_version="v9.8.7",
                transmit_frequency="99.999 GHz",
                sample_rate="99 kSPS",
                velocity_resolution="9.999 mph",
                azimuth_fov="99°",
                elevation_fov="88°",
            ),
            output=OutputConfig(
                file_prefix="test-output",
                output_dir=self.temp_dir,
            ),
        )

    def tearDown(self):
        """Clean up test files."""
        import shutil

        if os.path.exists(self.temp_dir):
            shutil.rmtree(self.temp_dir)

    @patch("pdf_generator.core.pdf_generator.DocumentBuilder")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    def test_radar_config_values_in_report(self, mock_map, mock_chart, mock_builder):
        """Test that all radar config values appear in the generated report."""
        # Setup mocks
        mock_chart.return_value = False  # No charts
        mock_doc = MagicMock()
        mock_builder_inst = mock_builder.return_value
        mock_builder_inst.build.return_value = mock_doc

        # Track all param_entries that get created
        original_append = mock_doc.append

        def track_append(content):
            # Capture param_entries when create_param_table is called
            if hasattr(content, "__class__") and "Tabular" in str(content.__class__):
                # This is a workaround - in real test we'd inspect the actual calls
                pass
            return original_append(content)

        mock_doc.append = track_append

        # Generate report with our test config
        output_path = os.path.join(self.temp_dir, "test_report.pdf")

        with patch(
            "pdf_generator.core.pdf_generator.create_param_table"
        ) as mock_param_table:
            mock_param_table.return_value = MagicMock()

            generate_pdf_report(
                output_path=output_path,
                start_iso="2025-01-01T00:00:00-08:00",
                end_iso="2025-01-07T23:59:59-08:00",
                group=self.config.query.group,
                units=self.config.query.units,
                timezone_display="US/Pacific",
                min_speed_str="5.0 mph",
                location=self.config.site.location,
                overall_metrics=[],
                daily_metrics=None,
                granular_metrics=[],
                histogram=None,
                tz_name=self.config.query.timezone,
                surveyor=self.config.site.surveyor,
                contact=self.config.site.contact,
                cosine_error_angle=self.config.radar.cosine_error_angle,
                sensor_model=self.config.radar.sensor_model,
                firmware_version=self.config.radar.firmware_version,
                transmit_frequency=self.config.radar.transmit_frequency,
                sample_rate=self.config.radar.sample_rate,
                velocity_resolution=self.config.radar.velocity_resolution,
                azimuth_fov=self.config.radar.azimuth_fov,
                elevation_fov=self.config.radar.elevation_fov,
                include_map=False,
            )

            # Verify create_param_table was called
            self.assertTrue(mock_param_table.called)

            # Get the param_entries that were passed to create_param_table
            call_args = mock_param_table.call_args
            param_entries = call_args[0][0]

            # Convert to dict for easier assertion
            params_dict = {entry["key"]: entry["value"] for entry in param_entries}

            # Verify all radar config values appear
            self.assertEqual(params_dict["Radar Sensor"], "Test Sensor Model Epsilon")
            self.assertEqual(params_dict["Radar Firmware version"], "v9.8.7")
            self.assertEqual(params_dict["Radar Transmit Frequency"], "99.999 GHz")
            self.assertEqual(params_dict["Radar Sample Rate"], "99 kSPS")
            self.assertEqual(params_dict["Radar Velocity Resolution"], "9.999 mph")
            self.assertEqual(params_dict["Azimuth Field of View"], "99°")
            self.assertEqual(params_dict["Elevation Field of View"], "88°")
            self.assertEqual(params_dict["Cosine Error Angle"], "15.0°")
            # Cosine error factor is calculated: 1/cos(15°) ≈ 1.0353
            self.assertIn("Cosine Error Factor", params_dict)
            self.assertTrue(params_dict["Cosine Error Factor"].startswith("1.03"))

    @patch("pdf_generator.core.pdf_generator.DocumentBuilder")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    def test_site_config_values_in_report(self, mock_map, mock_chart, mock_builder):
        """Test that site config values appear in the generated report."""
        mock_chart.return_value = False
        mock_doc = MagicMock()
        mock_builder_inst = mock_builder.return_value
        mock_builder_inst.build.return_value = mock_doc

        output_path = os.path.join(self.temp_dir, "test_report.pdf")

        generate_pdf_report(
            output_path=output_path,
            start_iso="2025-01-01T00:00:00-08:00",
            end_iso="2025-01-07T23:59:59-08:00",
            group="1h",
            units="mph",
            timezone_display="US/Pacific",
            min_speed_str="5.0 mph",
            location=self.config.site.location,
            overall_metrics=[],
            daily_metrics=None,
            granular_metrics=[],
            histogram=None,
            tz_name="US/Pacific",
            surveyor=self.config.site.surveyor,
            contact=self.config.site.contact,
            speed_limit=self.config.site.speed_limit,
            site_description=self.config.site.site_description,
            speed_limit_note=self.config.site.speed_limit_note,
            include_map=False,
        )

        # Verify builder.build was called with correct surveyor and contact
        mock_builder_inst.build.assert_called_once()
        call_args = mock_builder_inst.build.call_args

        # Check positional or keyword args depending on how it was called
        if call_args[1]:  # keyword args
            self.assertEqual(call_args[1]["surveyor"], "Test Surveyor Beta")
            self.assertEqual(call_args[1]["contact"], "test@example.com")
            self.assertEqual(call_args[1]["location"], "Test Location Alpha")
        else:  # positional args
            # builder.build(start_iso, end_iso, location, surveyor, contact)
            self.assertEqual(call_args[0][2], "Test Location Alpha")
            self.assertEqual(call_args[0][3], "Test Surveyor Beta")
            self.assertEqual(call_args[0][4], "test@example.com")

    @patch("pdf_generator.core.pdf_generator.DocumentBuilder")
    @patch("pdf_generator.core.pdf_generator.chart_exists")
    @patch("pdf_generator.core.pdf_generator.MapProcessor")
    @patch("pdf_generator.core.pdf_generator.add_site_specifics")
    def test_site_description_propagation(
        self, mock_site_spec, mock_map, mock_chart, mock_builder
    ):
        """Test that site description and speed limit note propagate correctly."""
        mock_chart.return_value = False
        mock_doc = MagicMock()
        mock_builder_inst = mock_builder.return_value
        mock_builder_inst.build.return_value = mock_doc

        output_path = os.path.join(self.temp_dir, "test_report.pdf")

        generate_pdf_report(
            output_path=output_path,
            start_iso="2025-01-01T00:00:00-08:00",
            end_iso="2025-01-07T23:59:59-08:00",
            group="1h",
            units="mph",
            timezone_display="US/Pacific",
            min_speed_str="5.0 mph",
            location="Test Location",
            overall_metrics=[],
            daily_metrics=None,
            granular_metrics=[],
            histogram=None,
            tz_name="US/Pacific",
            site_description="Test site description gamma",
            speed_limit_note="Test speed limit note delta",
            include_map=False,
        )

        # Verify add_site_specifics was called with correct values
        mock_site_spec.assert_called_once()
        call_args = mock_site_spec.call_args[0]
        self.assertEqual(call_args[1], "Test site description gamma")
        self.assertEqual(call_args[2], "Test speed limit note delta")

    def test_cosine_error_factor_calculation(self):
        """Test that cosine error factor is calculated correctly from angle."""
        import math

        # Test various angles
        test_cases = [
            (0.0, 1.0),  # 0 degrees -> factor of 1.0
            (21.0, 1.0 / math.cos(math.radians(21.0))),  # Example from config
            (15.0, 1.0 / math.cos(math.radians(15.0))),  # Our test angle
            (45.0, 1.0 / math.cos(math.radians(45.0))),  # 45 degrees
        ]

        for angle, expected_factor in test_cases:
            config = ReportConfig(
                site=SiteConfig(
                    location="Test",
                    surveyor="Test",
                    contact="test@test.com",
                ),
                query=QueryConfig(
                    start_date="2025-01-01",
                    end_date="2025-01-02",
                    timezone="UTC",
                ),
                radar=RadarConfig(cosine_error_angle=angle),
                output=OutputConfig(file_prefix="test"),
            )

            # The factor should be calculated as property
            self.assertAlmostEqual(
                config.radar.cosine_error_factor,
                expected_factor,
                places=4,
                msg=f"Cosine error factor mismatch for angle {angle}°",
            )


class TestConfigValueDefaults(unittest.TestCase):
    """Test that default values are properly set for radar config."""

    def test_radar_config_defaults(self):
        """Test that RadarConfig has sensible defaults."""
        radar = RadarConfig(cosine_error_angle=21.0)

        # Verify defaults match what's in the schema
        self.assertEqual(radar.sensor_model, "OmniPreSense OPS243-A")
        self.assertEqual(radar.firmware_version, "v1.2.3")
        self.assertEqual(radar.transmit_frequency, "24.125 GHz")
        self.assertEqual(radar.sample_rate, "20 kSPS")
        self.assertEqual(radar.velocity_resolution, "0.272 mph")
        self.assertEqual(radar.azimuth_fov, "20°")
        self.assertEqual(radar.elevation_fov, "24°")

    def test_config_from_minimal_json(self):
        """Test that config can be loaded from minimal JSON with proper defaults."""
        minimal_config = {
            "site": {
                "location": "Test Location",
                "surveyor": "Test Surveyor",
                "contact": "test@example.com",
            },
            "query": {
                "start_date": "2025-01-01",
                "end_date": "2025-01-07",
                "timezone": "UTC",
            },
            "radar": {"cosine_error_angle": 21.0},
        }

        config = ReportConfig.from_dict(minimal_config)

        # Verify required fields are set
        self.assertEqual(config.site.location, "Test Location")
        self.assertEqual(config.radar.cosine_error_angle, 21.0)

        # Verify defaults are applied
        self.assertEqual(config.radar.sensor_model, "OmniPreSense OPS243-A")
        self.assertEqual(config.query.group, "1h")
        self.assertEqual(config.query.units, "mph")


if __name__ == "__main__":
    unittest.main()
