#!/usr/bin/env python3

"""Utilities for creating ZIP archives with LaTeX sources and charts.

This module provides functions to create portable and fonts-based versions of
TEX files, and package them into ZIP archives for distribution.
"""

import os
import glob
import zipfile
import tempfile
from typing import Optional
from pathlib import Path


def create_portable_tex(original_tex_path: str, portable_tex_path: str) -> None:
    """Create a portable version of the TEX file that uses standard LaTeX fonts.

    This replaces custom font paths with standard Computer Modern/Latin Modern fonts
    that are available in all LaTeX distributions. The portable version can be compiled
    with pdflatex instead of xelatex.

    Args:
        original_tex_path: Path to the original .tex file with custom fonts
        portable_tex_path: Path where the portable .tex file should be written
    """
    with open(original_tex_path, "r", encoding="utf-8") as f:
        content = f.read()

    # Remove fontspec package (requires XeTeX/LuaTeX)
    content = content.replace(
        r"\usepackage{fontspec}%", r"% \usepackage{fontspec}% (removed for portability)"
    )

    # Process line by line to handle multi-line font declarations
    lines = content.split("\n")
    filtered_lines = []
    in_multiline_font_decl = False

    for line in lines:
        # Detect start of multi-line font declarations
        if any(
            cmd in line
            for cmd in [
                r"\setsansfont[",
                r"\newfontfamily\AtkinsonMono[",
            ]
        ):
            filtered_lines.append(f"% {line}  % (removed for portability)")
            # Check if this declaration continues on next lines (no closing brace)
            if not (line.rstrip().endswith("}%") or line.rstrip().endswith("}")):
                in_multiline_font_decl = True
            continue

        # Skip lines that are part of multi-line font declarations
        if in_multiline_font_decl:
            filtered_lines.append(f"% {line}  % (removed for portability)")
            # Check if this line ends the declaration
            if line.rstrip().endswith("}%") or line.rstrip().endswith("}"):
                in_multiline_font_decl = False
            continue

        # Comment out single-line font commands
        if r"\renewcommand{\familydefault}{\sfdefault}" in line:
            filtered_lines.append(f"% {line}  % (removed for portability)")
            continue

        # Replace \AtkinsonMono with \texttt (monospace font)
        if r"\AtkinsonMono{" in line:
            line = line.replace(r"\AtkinsonMono{", r"\texttt{")

        # Also handle >\AtkinsonMono in table column specifications (with braces)
        if r">{\AtkinsonMono}" in line:
            line = line.replace(r">{\AtkinsonMono}", r">{\ttfamily}")

        filtered_lines.append(line)

    portable_content = "\n".join(filtered_lines)

    # Add a note at the top of the document
    portable_content = portable_content.replace(
        r"\begin{document}%",
        r"""% NOTE: This is a portable version of the original TEX file.
% Custom fonts have been replaced with standard LaTeX fonts for compatibility.
% You can compile this with pdflatex instead of xelatex.
% To use the original custom fonts, see the full report generation system at:
% https://github.com/banshee-data/velocity.report
%
\begin{document}%""",
    )

    with open(portable_tex_path, "w", encoding="utf-8") as f:
        f.write(portable_content)


def create_fonts_tex(original_tex_path: str, fonts_tex_path: str) -> None:
    """Create a version of the TEX file that uses fonts from ./fonts/ directory.

    This modifies the font paths to reference a local ./fonts/ directory instead of
    the absolute paths used during report generation. Users can download the fonts
    and place them in this directory to compile with the original custom fonts.

    Args:
        original_tex_path: Path to the original .tex file with custom fonts
        fonts_tex_path: Path where the fonts version .tex file should be written
    """
    with open(original_tex_path, "r", encoding="utf-8") as f:
        content = f.read()

    # Replace absolute font paths with relative ./fonts/ path
    # The Path= parameter in font declarations points to the absolute path
    lines = content.split("\n")
    modified_lines = []

    for line in lines:
        # Replace absolute paths in font declarations with ./fonts/
        if "Path=/Users/" in line or "Path=/home/" in line:
            # Extract everything after Path= up to the next comma
            import re

            line = re.sub(r"Path=[^,]+,", r"Path=./fonts/,", line)
        modified_lines.append(line)

    fonts_content = "\n".join(modified_lines)

    # Add a note at the top of the document
    fonts_content = fonts_content.replace(
        r"\begin{document}%",
        r"""% NOTE: This version uses custom fonts from the ./fonts/ directory.
% Download the fonts from:
% https://brailleinstitute.org/freefont
%
% Place these font files in a ./fonts/ subdirectory:
% - AtkinsonHyperlegible-Regular.ttf
% - AtkinsonHyperlegible-Italic.ttf
% - AtkinsonHyperlegible-Bold.ttf
% - AtkinsonHyperlegible-BoldItalic.ttf
% - AtkinsonHyperlegibleMono-VariableFont_wght.ttf
% - AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf
%
% Then compile with: xelatex report_fonts.tex
%
\begin{document}%""",
    )

    with open(fonts_tex_path, "w", encoding="utf-8") as f:
        f.write(fonts_content)


def create_sources_zip(prefix: str, output_zip_path: Optional[str] = None) -> str:
    """Create a ZIP file containing all LaTeX sources and charts (excluding final PDF).

    This allows users to download the source files and make their own edits.

    Args:
        prefix: File prefix used for all generated files (e.g., "output/20060102-150405/radar_data_transits_2025-01-01_to_2025-01-31")
        output_zip_path: Optional custom path for the ZIP file. If not provided, uses "{prefix}_sources.zip"

    Returns:
        Path to the created ZIP file

    Raises:
        FileNotFoundError: If no source files are found to include in the ZIP
    """
    if output_zip_path is None:
        output_zip_path = f"{prefix}_sources.zip"

    # Create a portable version of the TEX file
    original_tex = f"{prefix}_report.tex"
    portable_tex = f"{prefix}_report_portable.tex"
    fonts_tex = f"{prefix}_report_fonts.tex"

    if os.path.isfile(original_tex):
        try:
            create_portable_tex(original_tex, portable_tex)
        except Exception as e:
            print(f"Warning: Failed to create portable TEX file: {e}")
            portable_tex = None

        # Create a version that uses local fonts from ./fonts/
        try:
            create_fonts_tex(original_tex, fonts_tex)
        except Exception as e:
            print(f"Warning: Failed to create fonts TEX file: {e}")
            fonts_tex = None
    else:
        portable_tex = None
        fonts_tex = None

    # Read README content from file
    readme_path_template = Path(__file__).parent / "zip_readme.md"
    try:
        with open(readme_path_template, "r", encoding="utf-8") as f:
            readme_content = f.read()
    except FileNotFoundError:
        # Fallback if README file doesn't exist
        readme_content = "# Velocity Report Source Files\n\nSee https://github.com/banshee-data/velocity.report for documentation.\n"

    # Write README to temp file
    readme_temp = tempfile.NamedTemporaryFile(
        mode="w", suffix="_README.md", delete=False, encoding="utf-8"
    )
    readme_temp.write(readme_content)
    readme_file = readme_temp.name
    readme_temp.close()

    # Create fonts directory instruction file
    fonts_instruction = """Visit: https://brailleinstitute.org/freefont

Download the font family and extract these files:
- AtkinsonHyperlegible-Regular.ttf
- AtkinsonHyperlegible-Italic.ttf
- AtkinsonHyperlegible-Bold.ttf
- AtkinsonHyperlegible-BoldItalic.ttf
- AtkinsonHyperlegibleMono-VariableFont_wght.ttf
- AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf

Place all .ttf files in this directory, then compile with:
    xelatex *_report_fonts.tex
"""
    fonts_instruction_temp = tempfile.NamedTemporaryFile(
        mode="w", suffix="_FONTS_GO_HERE.txt", delete=False, encoding="utf-8"
    )
    fonts_instruction_temp.write(fonts_instruction)
    fonts_instruction_file = fonts_instruction_temp.name
    fonts_instruction_temp.close()

    # Files to include - note that TEX files have _report suffix
    patterns_to_include = [
        portable_tex,  # Portable TEX with standard fonts
        fonts_tex,  # Fonts TEX with custom fonts from ./fonts/
        f"{prefix}_report.log",  # LaTeX log file (if exists)
        f"{prefix}_stats.pdf",  # Stats chart PDF
        f"{prefix}_daily.pdf",  # Daily chart PDF
        f"{prefix}_histogram.pdf",  # Histogram chart PDF
    ]

    # Explicitly exclude the final report PDF
    exclude_pattern = f"{prefix}_report.pdf"

    # Collect all files to include
    files_to_zip = []
    for pattern in patterns_to_include:
        if not pattern:  # Skip if pattern is None
            continue
        matched_files = glob.glob(pattern)
        for filepath in matched_files:
            # Skip the final report PDF (shouldn't match our patterns, but be safe)
            if filepath == exclude_pattern:
                continue
            if os.path.isfile(filepath):
                files_to_zip.append(filepath)

    if not files_to_zip:
        raise FileNotFoundError(f"No source files found matching prefix: {prefix}")

    # Create the ZIP file
    with zipfile.ZipFile(output_zip_path, "w", zipfile.ZIP_DEFLATED) as zipf:
        # Add README first
        zipf.write(readme_file, "README.txt")

        # Add fonts directory with instruction file
        zipf.write(fonts_instruction_file, "fonts/FONTS_GO_HERE.txt")

        # Add all source files
        for filepath in files_to_zip:
            arcname = os.path.basename(filepath)
            # Rename portable TEX to remove _report_portable suffix (becomes *_report.tex)
            if "_report_portable.tex" in arcname:
                arcname = arcname.replace("_report_portable.tex", "_report.tex")
            # Fonts TEX keeps its _report_fonts suffix (stays as *_report_fonts.tex)
            zipf.write(filepath, arcname)

    # Clean up temporary files
    try:
        os.remove(readme_file)
    except Exception:
        pass

    try:
        os.remove(fonts_instruction_file)
    except Exception:
        pass

    if portable_tex and os.path.isfile(portable_tex):
        try:
            os.remove(portable_tex)
        except Exception:
            pass  # Non-critical if cleanup fails

    if fonts_tex and os.path.isfile(fonts_tex):
        try:
            os.remove(fonts_tex)
        except Exception:
            pass  # Non-critical if cleanup fails

    return output_zip_path
