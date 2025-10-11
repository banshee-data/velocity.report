#!/usr/bin/env python3
"""Unit tests for document_builder.py"""

import os
import unittest
from unittest.mock import Mock, MagicMock, patch, call

from document_builder import DocumentBuilder


class TestDocumentBuilder(unittest.TestCase):
    """Tests for DocumentBuilder class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = DocumentBuilder()

    def test_initialization_default_config(self):
        """Test builder initialization with default config."""
        builder = DocumentBuilder()
        self.assertIsNotNone(builder.config)
        self.assertIn("geometry", builder.config)

    def test_initialization_custom_config(self):
        """Test builder initialization with custom config."""
        custom_config = {"geometry": {"top": "2cm"}, "fonts_dir": "custom_fonts"}
        builder = DocumentBuilder(config=custom_config)
        self.assertEqual(builder.config, custom_config)
        self.assertEqual(builder.config["fonts_dir"], "custom_fonts")

    @patch("document_builder.Document")
    def test_create_document_no_page_numbers(self, mock_doc_class):
        """Test document creation without page numbers."""
        mock_doc = MagicMock()
        mock_doc_class.return_value = mock_doc

        doc = self.builder.create_document(page_numbers=False)

        mock_doc_class.assert_called_once()
        call_kwargs = mock_doc_class.call_args[1]
        self.assertFalse(call_kwargs["page_numbers"])
        self.assertIn("geometry_options", call_kwargs)

    @patch("document_builder.Document")
    def test_create_document_with_page_numbers(self, mock_doc_class):
        """Test document creation with page numbers."""
        mock_doc = MagicMock()
        mock_doc_class.return_value = mock_doc

        doc = self.builder.create_document(page_numbers=True)

        call_kwargs = mock_doc_class.call_args[1]
        self.assertTrue(call_kwargs["page_numbers"])

    @patch("document_builder.Package")
    def test_add_packages(self, mock_package):
        """Test all required packages are added."""
        mock_doc = MagicMock()
        mock_doc.packages = []

        self.builder.add_packages(mock_doc)

        # Should have called Package constructor multiple times
        self.assertGreater(mock_package.call_count, 5)

        # Check that specific packages are included
        package_names = [call_args[0][0] for call_args in mock_package.call_args_list]
        expected_packages = [
            "fancyhdr",
            "graphicx",
            "amsmath",
            "titlesec",
            "hyperref",
            "fontspec",
            "caption",
            "supertabular",
            "float",
            "array",
        ]

        for pkg in expected_packages:
            self.assertIn(pkg, package_names, f"Package {pkg} not added")

    @patch("document_builder.Package")
    def test_add_packages_with_options(self, mock_package):
        """Test packages with options are added correctly."""
        mock_doc = MagicMock()
        mock_doc.packages = []

        self.builder.add_packages(mock_doc)

        # Check that caption package was called with options
        caption_calls = [
            call_args
            for call_args in mock_package.call_args_list
            if call_args[0][0] == "caption"
        ]
        self.assertGreater(len(caption_calls), 0)
        # Verify options were passed
        self.assertIn("options", caption_calls[0][1])

    @patch("document_builder.NoEscape")
    def test_setup_preamble(self, mock_noescape):
        """Test preamble configuration."""
        mock_doc = MagicMock()
        mock_doc.preamble = MagicMock()

        self.builder.setup_preamble(mock_doc)

        # Should have called NoEscape multiple times for preamble additions
        self.assertGreater(mock_noescape.call_count, 2)

        # Check that key preamble elements are set
        noescape_calls = [
            str(call_args[0][0]) for call_args in mock_noescape.call_args_list
        ]

        # Check for caption setup
        self.assertTrue(any("captionsetup" in call for call in noescape_calls))

        # Check for title format
        self.assertTrue(any("titleformat" in call for call in noescape_calls))

        # Check for columnsep
        self.assertTrue(any("columnsep" in call for call in noescape_calls))

    def test_setup_preamble_custom_columnsep(self):
        """Test preamble with custom column separation."""
        custom_config = {"columnsep": "20", "headheight": "12pt", "headsep": "10pt"}
        builder = DocumentBuilder(config=custom_config)
        mock_doc = MagicMock()
        mock_doc.preamble = MagicMock()

        builder.setup_preamble(mock_doc)

        # Should append columnsep with pt suffix
        appended_items = [
            call_args[0][0] for call_args in mock_doc.preamble.append.call_args_list
        ]
        self.assertTrue(any("20pt" in str(item) for item in appended_items))

    @patch("os.path.exists")
    @patch("document_builder.NoEscape")
    def test_setup_fonts_with_mono(self, mock_noescape, mock_exists):
        """Test font setup when mono font exists."""
        # Fonts directory and mono font both exist
        mock_exists.side_effect = lambda path: True

        mock_doc = MagicMock()
        mock_doc.preamble = MagicMock()
        fonts_path = "/path/to/fonts"

        self.builder.setup_fonts(mock_doc, fonts_path)

        # Should have set up both sans and mono fonts
        noescape_calls = [
            str(call_args[0][0]) for call_args in mock_noescape.call_args_list
        ]

        # Check for sans font setup
        self.assertTrue(any("setsansfont" in call for call in noescape_calls))

        # Check for mono font setup
        self.assertTrue(
            any(
                "newfontfamily" in call and "AtkinsonMono" in call
                for call in noescape_calls
            )
        )

        # Check for default family
        self.assertTrue(any("familydefault" in call for call in noescape_calls))

    @patch("os.path.exists")
    @patch("document_builder.NoEscape")
    def test_setup_fonts_without_mono(self, mock_noescape, mock_exists):
        """Test font setup fallback without mono font."""

        # Fonts directory exists, but mono font doesn't
        def exists_side_effect(path):
            return not path.endswith("Mono-VariableFont_wght.ttf")

        mock_exists.side_effect = exists_side_effect

        mock_doc = MagicMock()
        mock_doc.preamble = MagicMock()
        fonts_path = "/path/to/fonts"

        self.builder.setup_fonts(mock_doc, fonts_path)

        noescape_calls = [
            str(call_args[0][0]) for call_args in mock_noescape.call_args_list
        ]

        # Should use ttfamily fallback instead of newfontfamily
        self.assertTrue(any("ttfamily" in call for call in noescape_calls))

    @patch("os.path.exists")
    def test_setup_fonts_missing_directory(self, mock_exists):
        """Test font setup when fonts directory is missing."""
        mock_exists.return_value = False

        mock_doc = MagicMock()
        mock_doc.preamble = MagicMock()
        fonts_path = "/nonexistent/path"

        # Should not raise, just print warning
        self.builder.setup_fonts(mock_doc, fonts_path)

        # Should not have added any font configuration
        self.assertEqual(mock_doc.preamble.append.call_count, 0)

    @patch("document_builder.NoEscape")
    def test_setup_header(self, mock_noescape):
        """Test header/footer configuration."""
        mock_doc = MagicMock()
        start_iso = "2025-01-13T00:00:00Z"
        end_iso = "2025-01-19T23:59:59Z"
        location = "Test Location"

        self.builder.setup_header(mock_doc, start_iso, end_iso, location)

        noescape_calls = [
            str(call_args[0][0]) for call_args in mock_noescape.call_args_list
        ]

        # Check for fancyhdr setup
        self.assertTrue(any("pagestyle{fancy}" in call for call in noescape_calls))

        # Check for header with dates
        self.assertTrue(
            any(
                "2025-01-13" in call and "2025-01-19" in call for call in noescape_calls
            )
        )

        # Check for location in header
        self.assertTrue(any("Test Location" in call for call in noescape_calls))

        # Check for velocity.report link
        self.assertTrue(any("velocity.report" in call for call in noescape_calls))

    @patch("document_builder.NoEscape")
    def test_begin_twocolumn_layout(self, mock_noescape):
        """Test two-column layout initialization."""
        mock_doc = MagicMock()
        location = "Main Street"
        surveyor = "John Doe"
        contact = "john@example.com"

        self.builder.begin_twocolumn_layout(mock_doc, location, surveyor, contact)

        noescape_calls = [
            str(call_args[0][0]) for call_args in mock_noescape.call_args_list
        ]

        # Check for twocolumn command
        self.assertTrue(any("twocolumn" in call for call in noescape_calls))

        # Check for location in title
        self.assertTrue(any("Main Street" in call for call in noescape_calls))

        # Check for surveyor
        self.assertTrue(any("John Doe" in call for call in noescape_calls))

        # Check for contact email
        self.assertTrue(any("john@example.com" in call for call in noescape_calls))

    @patch("document_builder.DocumentBuilder.create_document")
    @patch("document_builder.DocumentBuilder.add_packages")
    @patch("document_builder.DocumentBuilder.setup_preamble")
    @patch("document_builder.DocumentBuilder.setup_fonts")
    @patch("document_builder.DocumentBuilder.setup_header")
    @patch("document_builder.DocumentBuilder.begin_twocolumn_layout")
    def test_build_orchestrates_all_steps(
        self,
        mock_twocolumn,
        mock_header,
        mock_fonts,
        mock_preamble,
        mock_packages,
        mock_create,
    ):
        """Test build() orchestrates all steps."""
        mock_doc = MagicMock()
        mock_create.return_value = mock_doc

        start_iso = "2025-01-13T00:00:00Z"
        end_iso = "2025-01-19T23:59:59Z"
        location = "Test St"
        surveyor = "Jane Smith"
        contact = "jane@test.com"

        result = self.builder.build(start_iso, end_iso, location, surveyor, contact)

        # Verify all steps were called
        mock_create.assert_called_once_with(page_numbers=False)
        mock_packages.assert_called_once_with(mock_doc)
        mock_preamble.assert_called_once_with(mock_doc)
        mock_fonts.assert_called_once()
        mock_header.assert_called_once_with(mock_doc, start_iso, end_iso, location)
        mock_twocolumn.assert_called_once_with(mock_doc, location, surveyor, contact)

        # Should return the document
        self.assertEqual(result, mock_doc)

    @patch("document_builder.DocumentBuilder.create_document")
    @patch("document_builder.DocumentBuilder.add_packages")
    @patch("document_builder.DocumentBuilder.setup_preamble")
    @patch("document_builder.DocumentBuilder.setup_fonts")
    @patch("document_builder.DocumentBuilder.setup_header")
    @patch("document_builder.DocumentBuilder.begin_twocolumn_layout")
    @patch(
        "document_builder.DEFAULT_SITE_INFO",
        {"surveyor": "Default Surveyor", "contact": "default@example.com"},
    )
    def test_build_uses_site_info_defaults(
        self,
        mock_twocolumn,
        mock_header,
        mock_fonts,
        mock_preamble,
        mock_packages,
        mock_create,
    ):
        """Test build() uses DEFAULT_SITE_INFO defaults when surveyor/contact not provided."""
        mock_doc = MagicMock()
        mock_create.return_value = mock_doc

        start_iso = "2025-01-13T00:00:00Z"
        end_iso = "2025-01-19T23:59:59Z"
        location = "Test St"

        result = self.builder.build(start_iso, end_iso, location)

        # Should use DEFAULT_SITE_INFO defaults
        mock_twocolumn.assert_called_once()
        call_args = mock_twocolumn.call_args[0]
        self.assertEqual(call_args[2], "Default Surveyor")
        self.assertEqual(call_args[3], "default@example.com")

    def test_font_path_resolution(self):
        """Test fonts directory path resolution."""
        custom_config = {"fonts_dir": "custom_fonts"}
        builder = DocumentBuilder(config=custom_config)

        self.assertEqual(builder.config["fonts_dir"], "custom_fonts")


if __name__ == "__main__":
    unittest.main()
