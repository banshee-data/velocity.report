#!/usr/bin/env python3
"""LaTeX document builder for PDF reports.

This module handles all document initialization, package management, and preamble
setup, providing a clean API for creating properly configured PyLaTeX documents.
"""

import os
from typing import TYPE_CHECKING, Dict, Optional, Any

try:
    from pylatex import Document, Package, NoEscape
    from pylatex.utils import escape_latex

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

    def escape_latex(s: str) -> str:  # type: ignore
        """Fallback escape_latex when pylatex is not available."""
        # Basic LaTeX escaping for special characters
        latex_special_chars = {
            "\\": r"\textbackslash{}",
            "{": r"\{",
            "}": r"\}",
            "$": r"\$",
            "&": r"\&",
            "#": r"\#",
            "_": r"\_",
            "%": r"\%",
            "~": r"\textasciitilde{}",
            "^": r"\textasciicircum{}",
        }
        return "".join(latex_special_chars.get(c, c) for c in s)


from pdf_generator.core.config_manager import (
    DEFAULT_PDF_CONFIG,
    DEFAULT_SITE_CONFIG,
    _pdf_to_dict,
    _site_to_dict,
)

if TYPE_CHECKING:
    from pdf_generator.core.tex_environment import TexEnvironment


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
        """Initialise with optional config override.

        Args:
            config: Optional dict to override default PDF configuration
        """
        self.config = config or _pdf_to_dict(DEFAULT_PDF_CONFIG)

    def create_document(
        self, page_numbers: bool = False, use_geometry_options: bool = True
    ) -> Document:
        """Create base Document with geometry.

        Args:
            page_numbers: Whether to show page numbers
            use_geometry_options: Let PyLaTeX emit \\usepackage[...]{geometry}

        Returns:
            Document instance with geometry configured
        """
        geometry_options = self.config.get("geometry", {})
        kwargs = {
            "page_numbers": page_numbers,
            "fontenc": None,
            "inputenc": None,
            "lmodern": False,
            "textcomp": False,
        }
        if use_geometry_options:
            kwargs["geometry_options"] = geometry_options

        # XeLaTeX handles Unicode/font loading via fontspec; disable PyLaTeX's
        # legacy font packages to keep the TeX dependency surface minimal.
        return Document(**kwargs)

    def apply_geometry_options(self, doc: Document) -> None:
        """Apply geometry settings in preamble when geometry is preloaded."""
        geometry_options = self.config.get("geometry", {})
        if not geometry_options:
            return

        rendered = ",".join(
            f"{key}={value}" if value is not None else str(key)
            for key, value in geometry_options.items()
        )
        doc.preamble.append(NoEscape(f"\\geometry{{{rendered}}}"))

    def add_packages(self, doc: Document, skip_preloaded: bool = False) -> None:
        """Add all required LaTeX packages.

        Args:
            doc: Document instance to add packages to
            skip_preloaded: Inject only runtime-only packages in precompiled mode
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
        if skip_preloaded:
            # velocity-report.fmt preloads all package macros except fontspec.
            packages = [pkg for pkg in packages if pkg[0] == "fontspec"]

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
        # mono_italic = os.path.join(
        #     fonts_path, "AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf"
        # )

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
        self,
        doc: Document,
        start_iso: str,
        end_iso: str,
        location: str,
        compare_start_iso: Optional[str] = None,
        compare_end_iso: Optional[str] = None,
        start_date: Optional[str] = None,
        end_date: Optional[str] = None,
        compare_start_date: Optional[str] = None,
        compare_end_date: Optional[str] = None,
    ) -> None:
        """Configure page header with fancyhdr.

        Args:
            doc: Document instance to configure
            start_iso: Start date in ISO format
            end_iso: End date in ISO format
            location: Location name for header
            compare_start_iso: Optional comparison start date in ISO format
            compare_end_iso: Optional comparison end date in ISO format
            start_date: Original start date string for display (if None, uses start_iso[:10])
            end_date: Original end date string for display (if None, uses end_iso[:10])
            compare_start_date: Original comparison start date for display
            compare_end_date: Original comparison end date for display
        """
        doc.append(NoEscape("\\pagestyle{fancy}"))
        doc.append(NoEscape("\\fancyhf{}"))
        doc.append(
            NoEscape(
                "\\fancyhead[L]{\\textbf{\\protect\\href{https://velocity.report}{velocity.report}}}"
            )
        )
        # Date range moved to footer, header center is empty
        # Escape user-controlled location to prevent LaTeX injection
        escaped_location = escape_latex(location)
        doc.append(NoEscape(f"\\fancyhead[R]{{ \\textit{{{escaped_location}}}}}"))
        doc.append(NoEscape("\\renewcommand{\\headrulewidth}{0.8pt}"))
        # Add footer with date range on left and page number on right
        # Use original date strings (single source of truth from datepicker - no fallbacks)
        if compare_start_date and compare_end_date:
            footer_range = (
                f"{start_date} to {end_date} vs "
                f"{compare_start_date} to {compare_end_date}"
            )
        else:
            footer_range = f"{start_date} to {end_date}"
        doc.append(NoEscape(f"\\fancyfoot[L]{{\\small {footer_range}}}"))
        doc.append(NoEscape("\\fancyfoot[R]{\\small Page \\thepage}"))
        doc.append(NoEscape("\\renewcommand{\\footrulewidth}{0.8pt}"))

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
        # Escape user-controlled strings to prevent LaTeX injection
        escaped_location = escape_latex(location)
        escaped_surveyor = escape_latex(surveyor)
        escaped_contact = escape_latex(contact)
        doc.append(NoEscape(f"{{\\huge \\sffamily\\textbf{{ {escaped_location}}}}}"))
        doc.append(NoEscape("\\par"))
        doc.append(NoEscape("\\vspace{0.1cm}"))
        surveyor_line = f"{{\\large \\sffamily Surveyor: \\textit{{{escaped_surveyor}}} \\ \\textbullet \\ \\ Contact: \\href{{mailto:{escaped_contact}}}{{{escaped_contact}}}}}"
        doc.append(NoEscape(surveyor_line))
        doc.append(NoEscape("\\end{center}"))
        doc.append(NoEscape("]"))

    def build(
        self,
        start_iso: str,
        end_iso: str,
        location: str,
        surveyor: Optional[str] = None,
        contact: Optional[str] = None,
        compare_start_iso: Optional[str] = None,
        compare_end_iso: Optional[str] = None,
        start_date: Optional[str] = None,
        end_date: Optional[str] = None,
        compare_start_date: Optional[str] = None,
        compare_end_date: Optional[str] = None,
        tex_environment: Optional["TexEnvironment"] = None,
    ) -> Document:
        """Build complete configured document (convenience method).

        This is the main entry point that orchestrates all setup steps.

        Args:
            start_iso: Start date in ISO format
            end_iso: End date in ISO format
            location: Location name
            surveyor: Surveyor name (uses default if not provided)
            contact: Contact email (uses default if not provided)
            start_date: Original start date string for display (if None, uses start_iso[:10])
            end_date: Original end date string for display (if None, uses end_iso[:10])
            compare_start_date: Original comparison start date string for display
            compare_end_date: Original comparison end date string for display
            tex_environment: Optional TeX environment for package-loading control

        Returns:
            Fully configured Document ready for content addition
        """
        # Use defaults only if None (not if empty string)
        if surveyor is None:
            surveyor = _site_to_dict(DEFAULT_SITE_CONFIG)["surveyor"]
        if contact is None:
            contact = _site_to_dict(DEFAULT_SITE_CONFIG)["contact"]

        skip_preloaded = bool(tex_environment and tex_environment.fmt_name)
        # Create base document — when using a preloaded format the geometry
        # package is already loaded so we must not pass geometry_options to the
        # Document constructor (which would emit \usepackage[...]{geometry}
        # again).  Instead we call apply_geometry_options() afterwards to emit
        # a bare \geometry{...} command that reconfigures the already-loaded
        # package.
        doc = self.create_document(
            page_numbers=False, use_geometry_options=not skip_preloaded
        )
        if skip_preloaded:
            self.apply_geometry_options(doc)

        # Add all packages
        self.add_packages(doc, skip_preloaded=skip_preloaded)

        # Setup preamble
        self.setup_preamble(doc)

        # Setup fonts
        fonts_dir = self.config.get("fonts_dir", "fonts")
        fonts_path = os.path.join(os.path.dirname(__file__), fonts_dir)
        self.setup_fonts(doc, fonts_path)

        # Setup header
        self.setup_header(
            doc,
            start_iso,
            end_iso,
            location,
            compare_start_iso,
            compare_end_iso,
            start_date,
            end_date,
            compare_start_date,
            compare_end_date,
        )

        # Begin two-column layout
        self.begin_twocolumn_layout(doc, location, surveyor, contact)

        return doc
