#!/usr/bin/env python3
"""LaTeX document builder for PDF reports.

This module handles all document initialization, package management, and preamble
setup, providing a clean API for creating properly configured PyLaTeX documents.
"""

import os
from typing import Dict, Optional, Any

try:
    from pylatex import Document, Package, NoEscape

    HAVE_PYLATEX = True
except Exception:  # pragma: no cover
    HAVE_PYLATEX = False

    class Document:  # type: ignore
        def __init__(self, *args, **kwargs):
            pass

    class Package:  # type: ignore
        def __init__(self, *args, **kwargs):
            pass

    class NoEscape(str):  # type: ignore
        pass


from report_config import PDF_CONFIG, SITE_INFO


class DocumentBuilder:
    """Builds PyLaTeX Document with custom configuration.

    Responsibilities:
    - Create Document with geometry settings
    - Add required packages
    - Configure preamble (fonts, headers, spacing)
    - Set up two-column layout with title
    - Provide clean API for document initialization
    """

    def __init__(self, config: Optional[Dict[str, Any]] = None):
        """Initialize with optional config override.

        Args:
            config: Optional dict to override PDF_CONFIG defaults
        """
        self.config = config or PDF_CONFIG

    def create_document(self, page_numbers: bool = False) -> Document:
        """Create base Document with geometry.

        Args:
            page_numbers: Whether to show page numbers

        Returns:
            Document instance with geometry configured
        """
        geometry_options = self.config.get("geometry", {})
        return Document(geometry_options=geometry_options, page_numbers=page_numbers)

    def add_packages(self, doc: Document) -> None:
        """Add all required LaTeX packages.

        Args:
            doc: Document instance to add packages to
        """
        # Package list in order of dependency
        packages = [
            ("fancyhdr", None),  # Headers and footers
            ("graphicx", None),  # Graphics inclusion
            ("amsmath", None),  # Math macros like \tfrac
            ("titlesec", None),  # Title formatting
            ("hyperref", None),  # Hyperlinks
            ("fontspec", None),  # Font selection for XeLaTeX/LuaLaTeX
            ("caption", "font=sf"),  # Sans-serif captions
            ("supertabular", None),  # Tables that can break across columns
            ("float", None),  # H position for floats
            ("array", None),  # Column spec modifiers (>{...}, <{...})
        ]

        for package_name, options in packages:
            if options:
                doc.packages.append(Package(package_name, options=options))
            else:
                doc.packages.append(Package(package_name))

    def setup_preamble(self, doc: Document) -> None:
        """Configure document preamble (fonts, formatting).

        Args:
            doc: Document instance to configure
        """
        # Make caption labels and text bold
        doc.preamble.append(NoEscape("\\captionsetup{labelfont=bf,textfont=bf}"))

        # Title formatting: make section headings sans-serif
        doc.preamble.append(
            NoEscape("\\titleformat{\\section}{\\bfseries\\Large}{}{0em}{}")
        )

        # Column gap configuration
        colsep_str = self.config.get("columnsep", "10")
        if colsep_str and not colsep_str.endswith("pt"):
            colsep_str = f"{colsep_str}pt"
        doc.preamble.append(NoEscape(f"\\setlength{{\\columnsep}}{{{colsep_str}}}"))

        # Header spacing
        headheight = self.config.get("headheight", "12pt")
        headsep = self.config.get("headsep", "10pt")
        doc.append(NoEscape(f"\\setlength{{\\headheight}}{{{headheight}}}"))
        doc.append(NoEscape(f"\\setlength{{\\headsep}}{{{headsep}}}"))

    def setup_fonts(self, doc: Document, fonts_path: str) -> None:
        """Register Atkinson Hyperlegible fonts.

        Args:
            doc: Document instance to configure
            fonts_path: Path to fonts directory
        """
        # Set the document's default family to sans-serif globally
        # Point fontspec at the bundled Atkinson Hyperlegible fonts
        if not os.path.exists(fonts_path):
            print(f"Warning: Fonts directory not found at {fonts_path}")
            return

        # Use Path= with trailing os.sep so fontspec can find the files
        sans_font_options = [
            f"Path={fonts_path + os.sep}",
            "Extension=.ttf",
            "Scale=MatchLowercase",
            "UprightFont=AtkinsonHyperlegible-Regular",
            "ItalicFont=AtkinsonHyperlegible-Italic",
            "BoldFont=AtkinsonHyperlegible-Bold",
            "BoldItalicFont=AtkinsonHyperlegible-BoldItalic",
        ]
        sans_font_options_str = ",\n    ".join(sans_font_options)
        doc.preamble.append(
            NoEscape(
                rf"\setsansfont[{sans_font_options_str}]{{AtkinsonHyperlegible-Regular}}"
            )
        )
        # Use sans family as the default
        doc.preamble.append(NoEscape("\\renewcommand{\\familydefault}{\\sfdefault}"))

        # Register Atkinson Hyperlegible Mono (if bundled)
        mono_regular = os.path.join(
            fonts_path, "AtkinsonHyperlegibleMono-VariableFont_wght.ttf"
        )
        mono_italic = os.path.join(
            fonts_path, "AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf"
        )

        if os.path.exists(mono_regular):
            # Register new font family command using fontspec
            # Prefer a heavier weight for the monospace family by requesting
            # a higher wght axis value from the variable font via fontspec.
            doc.preamble.append(
                NoEscape(
                    rf"\newfontfamily\AtkinsonMono[Path={fonts_path + os.sep},Extension=.ttf,ItalicFont=AtkinsonHyperlegibleMono-Italic-VariableFont_wght,Scale=MatchLowercase,RawFeature={{+wght=800}}]{{AtkinsonHyperlegibleMono-VariableFont_wght}}"
                )
            )
        else:
            # Fallback: define \AtkinsonMono as a declarative font switch
            # so it can be used both in column-specs (>{\AtkinsonMono}) and
            # around braced content (\AtkinsonMono{...}). Using \ttfamily
            # keeps behavior acceptable when the bundled mono isn't present.
            doc.preamble.append(NoEscape(r"\newcommand{\AtkinsonMono}{\ttfamily}"))

    def setup_header(
        self, doc: Document, start_iso: str, end_iso: str, location: str
    ) -> None:
        """Configure page header with fancyhdr.

        Args:
            doc: Document instance to configure
            start_iso: Start date in ISO format
            end_iso: End date in ISO format
            location: Location name for header
        """
        doc.append(NoEscape("\\pagestyle{fancy}"))
        doc.append(NoEscape("\\fancyhf{}"))
        doc.append(
            NoEscape(
                "\\fancyhead[L]{\\textbf{\\protect\\href{https://velocity.report}{velocity.report}}}"
            )
        )
        doc.append(NoEscape(f"\\fancyhead[C]{{ {start_iso[:10]} to {end_iso[:10]} }}"))
        doc.append(NoEscape(f"\\fancyhead[R]{{ \\textit{{{location}}}}}"))
        doc.append(NoEscape("\\renewcommand{\\headrulewidth}{0.8pt}"))

    def begin_twocolumn_layout(
        self, doc: Document, location: str, surveyor: str, contact: str
    ) -> None:
        """Start two-column layout with spanning title.

        Args:
            doc: Document instance to configure
            location: Location name for title
            surveyor: Surveyor name
            contact: Contact email
        """
        # Begin two-column layout using \twocolumn with header in optional argument
        # This allows supertabular to work properly and keeps header on same page
        # The optional argument to \twocolumn creates a spanning header above the columns
        doc.append(NoEscape("\\twocolumn["))
        doc.append(NoEscape("\\vspace{-8pt}"))
        doc.append(NoEscape("\\begin{center}"))
        doc.append(NoEscape(f"{{\\huge \\sffamily\\textbf{{ {location}}}}}"))
        doc.append(NoEscape("\\par"))
        doc.append(NoEscape("\\vspace{0.1cm}"))
        doc.append(
            NoEscape(
                f"{{\\large \\sffamily Surveyor: \\textit{{{surveyor}}} \\ \\textbullet \\ \\ Contact: \\href{{mailto:{contact}}}{{{contact}}}}}"
            )
        )
        doc.append(NoEscape("\\end{center}"))
        doc.append(NoEscape("]"))

    def build(
        self,
        start_iso: str,
        end_iso: str,
        location: str,
        surveyor: Optional[str] = None,
        contact: Optional[str] = None,
    ) -> Document:
        """Build complete configured document (convenience method).

        This is the main entry point that orchestrates all setup steps.

        Args:
            start_iso: Start date in ISO format
            end_iso: End date in ISO format
            location: Location name
            surveyor: Surveyor name (defaults to SITE_INFO)
            contact: Contact email (defaults to SITE_INFO)

        Returns:
            Fully configured Document ready for content addition
        """
        # Use defaults from SITE_INFO if not provided
        surveyor = surveyor or SITE_INFO["surveyor"]
        contact = contact or SITE_INFO["contact"]

        # Create base document
        doc = self.create_document(page_numbers=False)

        # Add all packages
        self.add_packages(doc)

        # Setup preamble
        self.setup_preamble(doc)

        # Setup fonts
        fonts_dir = self.config.get("fonts_dir", "fonts")
        fonts_path = os.path.join(os.path.dirname(__file__), fonts_dir)
        self.setup_fonts(doc, fonts_path)

        # Setup header
        self.setup_header(doc, start_iso, end_iso, location)

        # Begin two-column layout
        self.begin_twocolumn_layout(doc, location, surveyor, contact)

        return doc
