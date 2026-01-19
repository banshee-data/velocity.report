#!/usr/bin/env python3
"""Unit tests for report_sections module."""

import unittest
from unittest.mock import MagicMock, patch

# Import section builders
from pdf_generator.core.report_sections import (
    VelocityOverviewSection,
    SiteInformationSection,
    ScienceMethodologySection,
    SurveyParametersSection,
    add_metric_data_intro,
    add_site_specifics,
    add_science,
    add_survey_parameters,
)


class TestVelocityOverviewSection(unittest.TestCase):
    """Tests for VelocityOverviewSection class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = VelocityOverviewSection()
        self.mock_doc = MagicMock()

    def test_initialization(self):
        """Test builder initialization."""
        self.assertIsNotNone(self.builder)

    @patch("pdf_generator.core.report_sections.NoEscape")
    @patch("pdf_generator.core.report_sections.create_param_table")
    def test_build(self, mock_create_table, mock_noescape):
        """Test building velocity overview section."""
        mock_noescape.side_effect = lambda x: f"NoEscape({x})"
        mock_create_table.return_value = "param_table"

        self.builder.build(
            self.mock_doc,
            start_date="2025-01-01",
            end_date="2025-01-07",
            location="Main Street",
            speed_limit=25,
            total_vehicles=1000,
            p50=22.5,
            p85=28.0,
            p98=32.5,
            max_speed=38.0,
        )

        # Should append multiple elements (section header, paragraph, subsection, table, par)
        self.assertGreater(self.mock_doc.append.call_count, 3)

        # Should create parameter table
        mock_create_table.assert_called_once()
        call_args = mock_create_table.call_args[0][0]
        self.assertEqual(len(call_args), 4)  # 4 metrics

    @patch("pdf_generator.core.report_sections.create_comparison_summary_table")
    @patch("pdf_generator.core.report_sections.NoEscape")
    def test_build_with_comparison(self, mock_noescape, mock_create_table):
        """Test building velocity overview with comparison metrics."""
        mock_noescape.side_effect = lambda x: x
        mock_create_table.return_value = "comparison_table"

        self.builder.build(
            self.mock_doc,
            start_date="2025-01-01",
            end_date="2025-01-07",
            location="Main Street",
            speed_limit=25,
            total_vehicles=100,
            p50=20.0,
            p85=30.0,
            p98=40.0,
            max_speed=0.0,
            compare_start_date="2024-12-01",
            compare_end_date="2024-12-07",
            compare_total_vehicles=110,
            compare_p50=25.0,
            compare_p85=27.0,
            compare_p98=44.0,
            compare_max_speed=10.0,
        )

        mock_create_table.assert_called_once()
        entries, primary_label, compare_label = mock_create_table.call_args[0]
        self.assertEqual(primary_label, "t1")
        self.assertEqual(compare_label, "t2")

        entry_map = {entry["label"]: entry for entry in entries}
        self.assertEqual(entry_map["Max Velocity"]["change"], "--")
        self.assertEqual(entry_map["Total Vehicles"]["change"], "+10.0%")

    @patch("pdf_generator.core.report_sections.NoEscape")
    @patch("pdf_generator.core.report_sections.escape_latex")
    def test_build_formats_vehicle_count(self, mock_escape, mock_noescape):
        """Test that total vehicles is formatted with thousands separator."""
        mock_escape.side_effect = lambda x: x
        mock_noescape.side_effect = lambda x: f"NoEscape({x})"

        self.builder.build(
            self.mock_doc,
            start_date="2025-01-01",
            end_date="2025-01-07",
            location="Main Street",
            speed_limit=25,
            total_vehicles=12345,
            p50=22.5,
            p85=28.0,
            p98=32.5,
            max_speed=38.0,
        )

        # Check that escape_latex was called with formatted number
        calls = [str(call) for call in mock_escape.call_args_list]
        found_formatted = any("12,345" in str(c) for c in calls)
        self.assertTrue(
            found_formatted, "Should format vehicle count with thousands separator"
        )


class TestSiteInformationSection(unittest.TestCase):
    """Tests for SiteInformationSection class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = SiteInformationSection()
        self.mock_doc = MagicMock()

    def test_initialization(self):
        """Test builder initialization."""
        self.assertIsNotNone(self.builder)

    @patch("pdf_generator.core.report_sections.NoEscape")
    @patch("pdf_generator.core.report_sections.escape_latex")
    def test_build(self, mock_escape, mock_noescape):
        """Test building site information section."""
        mock_escape.side_effect = lambda x: x
        mock_noescape.side_effect = lambda x: f"NoEscape({x})"

        self.builder.build(
            self.mock_doc,
            site_description="Test site description",
            speed_limit_note="Speed limit is 25 mph",
        )

        # Should append subsection header, description, par, speed limit note
        self.assertGreaterEqual(self.mock_doc.append.call_count, 4)

        # Check that site_description and speed_limit_note were used
        calls = [str(call) for call in mock_escape.call_args_list]
        self.assertTrue(any("Test site description" in str(c) for c in calls))
        self.assertTrue(any("Speed limit is 25 mph" in str(c) for c in calls))


class TestScienceMethodologySection(unittest.TestCase):
    """Tests for ScienceMethodologySection class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = ScienceMethodologySection()
        self.mock_doc = MagicMock()

    def test_initialization(self):
        """Test builder initialization."""
        self.assertIsNotNone(self.builder)

    @patch("pdf_generator.core.report_sections.Center")
    @patch("pdf_generator.core.report_sections.NoEscape")
    def test_build(self, mock_noescape, mock_center):
        """Test building science section."""
        mock_noescape.side_effect = lambda x: f"NoEscape({x})"
        mock_center_inst = MagicMock()
        mock_center.return_value.__enter__.return_value = mock_center_inst

        self.builder.build(self.mock_doc)

        # Should append many elements (subsections, paragraphs, formula, etc.)
        self.assertGreater(self.mock_doc.append.call_count, 10)

        # Should create Center environment for formula explanation
        mock_center.assert_called()

    @patch("pdf_generator.core.report_sections.NoEscape")
    def test_add_citizen_radar_intro(self, mock_noescape):
        """Test adding citizen radar introduction."""
        mock_noescape.side_effect = lambda x: f"NoEscape({x})"

        self.builder._add_citizen_radar_intro(self.mock_doc)

        # Should append subsection, intro text, formula, centered explanation, safety text
        self.assertGreater(self.mock_doc.append.call_count, 5)

        # Check for kinetic energy formula
        calls = [str(call) for call in self.mock_doc.append.call_args_list]
        found_formula = any("K_E" in str(c) and "tfrac" in str(c) for c in calls)
        self.assertTrue(found_formula, "Should include kinetic energy formula")

    @patch("pdf_generator.core.report_sections.NoEscape")
    def test_add_aggregation_percentiles(self, mock_noescape):
        """Test adding aggregation and percentiles explanation."""
        mock_noescape.side_effect = lambda x: f"NoEscape({x})"

        self.builder._add_aggregation_percentiles(self.mock_doc)

        # Should append subsection, Doppler explanation, clustering, bias, percentiles, reliability
        self.assertGreater(self.mock_doc.append.call_count, 8)

        # Check for key concepts
        calls = [str(call) for call in self.mock_doc.append.call_args_list]
        found_doppler = any("Doppler" in str(c) for c in calls)
        found_p85 = any("p85" in str(c) for c in calls)
        found_p98 = any("p98" in str(c) for c in calls)

        self.assertTrue(found_doppler, "Should mention Doppler effect")
        self.assertTrue(found_p85, "Should mention p85 percentile")
        self.assertTrue(found_p98, "Should mention p98 percentile")


class TestSurveyParametersSection(unittest.TestCase):
    """Tests for SurveyParametersSection class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = SurveyParametersSection()
        self.mock_doc = MagicMock()

    def test_initialization(self):
        """Test builder initialization."""
        self.assertIsNotNone(self.builder)

    @patch("pdf_generator.core.report_sections.create_param_table")
    @patch("pdf_generator.core.report_sections.NoEscape")
    def test_build(self, mock_noescape, mock_create_table):
        """Test building survey parameters section."""
        mock_noescape.side_effect = lambda x: f"NoEscape({x})"
        mock_create_table.return_value = "param_table"

        self.builder.build(
            self.mock_doc,
            start_iso="2025-01-01T00:00:00",
            end_iso="2025-01-07T23:59:59",
            timezone_display="US/Pacific",
            group="1h",
            units="mph",
            min_speed_str="5 mph cutoff",
        )

        # Should append subsection, parameter table, par
        self.assertGreaterEqual(self.mock_doc.append.call_count, 3)

        # Should create parameter table with 15 entries
        mock_create_table.assert_called_once()
        call_args = mock_create_table.call_args[0][0]
        self.assertEqual(len(call_args), 15)  # 15 survey parameters

        # Check that key parameters are included
        keys = [entry["key"] for entry in call_args]
        self.assertIn("Start time", keys)
        self.assertIn("End time", keys)
        self.assertIn("Timezone", keys)
        self.assertIn("Roll-up Period", keys)
        self.assertIn("Units", keys)
        self.assertIn("Radar Sensor", keys)


class TestConvenienceFunctions(unittest.TestCase):
    """Tests for convenience wrapper functions."""

    @patch("pdf_generator.core.report_sections.VelocityOverviewSection")
    def test_add_metric_data_intro(self, mock_builder_class):
        """Test add_metric_data_intro convenience function."""
        mock_builder = MagicMock()
        mock_builder_class.return_value = mock_builder
        mock_doc = MagicMock()

        add_metric_data_intro(
            mock_doc,
            start_date="2025-01-01",
            end_date="2025-01-07",
            location="Main Street",
            speed_limit=25,
            total_vehicles=1000,
            p50=22.5,
            p85=28.0,
            p98=32.5,
            max_speed=38.0,
        )

        # Should create builder and call build
        mock_builder_class.assert_called_once()
        mock_builder.build.assert_called_once()

    @patch("pdf_generator.core.report_sections.SiteInformationSection")
    def test_add_site_specifics(self, mock_builder_class):
        """Test add_site_specifics convenience function."""
        mock_builder = MagicMock()
        mock_builder_class.return_value = mock_builder
        mock_doc = MagicMock()

        add_site_specifics(mock_doc, "site desc", "speed limit")

        # Should create builder and call build with all parameters
        mock_builder_class.assert_called_once()
        mock_builder.build.assert_called_once_with(mock_doc, "site desc", "speed limit")

    @patch("pdf_generator.core.report_sections.ScienceMethodologySection")
    def test_add_science(self, mock_builder_class):
        """Test add_science convenience function."""
        mock_builder = MagicMock()
        mock_builder_class.return_value = mock_builder
        mock_doc = MagicMock()

        add_science(mock_doc)

        # Should create builder and call build
        mock_builder_class.assert_called_once()
        mock_builder.build.assert_called_once_with(mock_doc)

    @patch("pdf_generator.core.report_sections.SurveyParametersSection")
    def test_add_survey_parameters(self, mock_builder_class):
        """Test add_survey_parameters convenience function."""
        mock_builder = MagicMock()
        mock_builder_class.return_value = mock_builder
        mock_doc = MagicMock()

        add_survey_parameters(
            mock_doc,
            start_iso="2025-01-01T00:00:00",
            end_iso="2025-01-07T23:59:59",
            timezone_display="US/Pacific",
            group="1h",
            units="mph",
            min_speed_str="5 mph cutoff",
        )

        # Should create builder and call build
        mock_builder_class.assert_called_once()
        mock_builder.build.assert_called_once()


class TestImportFallbacks(unittest.TestCase):
    """Tests for import error handling."""

    def test_pylatex_available(self):
        """Test that PyLaTeX is available in normal environment."""
        from pdf_generator.core.report_sections import HAVE_PYLATEX

        # In test environment, should be True
        self.assertTrue(HAVE_PYLATEX)

    def test_builder_requires_pylatex(self):
        """Test that builders require PyLaTeX."""
        # This test validates the import check exists
        builder = VelocityOverviewSection()
        self.assertIsNotNone(builder)


# Phase 3: Empty State and Error Handling Tests


class TestReportSectionsEmptyStates(unittest.TestCase):
    """Phase 3 tests for report_sections.py empty state handling."""

    def setUp(self):
        """Set up test fixtures."""
        self.mock_doc = MagicMock()

    def test_site_information_both_fields_empty(self):
        """Verify builder skips content when both description and note are empty.

        When both site description and speed limit note are empty the builder
        should return early and not append content to the document.
        """
        builder = SiteInformationSection()

        # Call with both fields empty - should return early and not append anything
        builder.build(self.mock_doc, site_description="", speed_limit_note="")

        # Should NOT append anything when both are empty (early return expected)
        self.mock_doc.append.assert_not_called()

    def test_site_information_only_description(self):
        """Test site section with only description, no speed limit note."""
        builder = SiteInformationSection()

        # Call with only description
        builder.build(
            self.mock_doc,
            site_description="This is a residential area",
            speed_limit_note="",
        )

        # Should append description
        self.mock_doc.append.assert_called()

    def test_site_information_only_speed_limit_note(self):
        """Test site section with only speed limit note, no description."""
        builder = SiteInformationSection()

        # Call with only speed limit note
        builder.build(
            self.mock_doc, site_description="", speed_limit_note="25 mph posted"
        )

        # Should append speed limit note
        self.mock_doc.append.assert_called()

    def test_velocity_overview_total_vehicles_format_error(self):
        """Test velocity overview when total_vehicles formatting fails.

        Verifies the section builder handles non-numeric vehicle counts without
        raising an exception and still produces output.
        """
        builder = VelocityOverviewSection()

        # Pass a value that might cause formatting issues
        # The try-except should handle this gracefully
        builder.build(
            self.mock_doc,
            start_date="2025-01-01",
            end_date="2025-01-07",
            location="Test Location",
            speed_limit=25,
            total_vehicles="invalid",  # Non-numeric
            p50=30.5,
            p85=35.0,
            p98=40.0,
            max_speed=45.0,
        )

        # Should handle error and still create section
        self.mock_doc.append.assert_called()

    def test_science_section_import_check(self):
        """Confirm ScienceMethodologySection can be constructed and used.

        Validates that the builder is importable in the test environment and
        that calling build() appends expected content to the document.
        """
        # This validates the import check exists
        builder = ScienceMethodologySection()
        self.assertIsNotNone(builder)

        # Verify it can build section (takes only doc parameter)
        builder.build(self.mock_doc)

        self.mock_doc.append.assert_called()

    def test_parameters_section_import_check(self):
        """Confirm SurveyParametersSection can be constructed and used."""
        # This validates the import check exists
        builder = SurveyParametersSection()
        self.assertIsNotNone(builder)

        # Verify it can build section
        builder.build(
            self.mock_doc,
            start_iso="2025-01-01T00:00:00",
            end_iso="2025-01-07T23:59:59",
            timezone_display="UTC",
            group="1h",
            units="mph",
            min_speed_str="5 mph cutoff",
        )

        self.mock_doc.append.assert_called()


if __name__ == "__main__":
    unittest.main()
