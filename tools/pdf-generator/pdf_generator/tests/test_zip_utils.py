#!/usr/bin/env python3
"""Tests for ZIP utilities module."""

import os
import subprocess
import sys
import tempfile
import unittest
import unittest.mock
import zipfile

from pdf_generator.core.zip_utils import (
    create_portable_tex,
    create_fonts_tex,
    create_sources_zip,
)


class TestCreatePortableTex(unittest.TestCase):
    """Test portable TEX file creation."""

    def setUp(self):
        """Set up test fixtures."""
        self.temp_dir = tempfile.mkdtemp()

    def tearDown(self):
        """Clean up test fixtures."""
        import shutil

        shutil.rmtree(self.temp_dir)

    def test_removes_fontspec_package(self):
        """Test that fontspec package is commented out."""
        original = os.path.join(self.temp_dir, "original.tex")
        portable = os.path.join(self.temp_dir, "portable.tex")

        content = r"""\usepackage{fontspec}%
\begin{document}%
Test content
\end{document}%"""

        with open(original, "w", encoding="utf-8") as f:
            f.write(content)

        create_portable_tex(original, portable)

        with open(portable, "r", encoding="utf-8") as f:
            result = f.read()

        self.assertIn("% \\usepackage{fontspec}%", result)
        self.assertNotIn("\\usepackage{fontspec}%\n", result)

    def test_replaces_atkinson_mono_with_texttt(self):
        """Test that AtkinsonMono is replaced with texttt."""
        original = os.path.join(self.temp_dir, "original.tex")
        portable = os.path.join(self.temp_dir, "portable.tex")

        content = r"""\usepackage{fontspec}%
\begin{document}%
Some \AtkinsonMono{code here} in text.
\end{document}%"""

        with open(original, "w", encoding="utf-8") as f:
            f.write(content)

        create_portable_tex(original, portable)

        with open(portable, "r", encoding="utf-8") as f:
            result = f.read()

        self.assertIn(r"\texttt{code here}", result)
        self.assertNotIn(r"\AtkinsonMono{", result)

    def test_replaces_table_column_fonts(self):
        """Test that table column font specs are replaced."""
        original = os.path.join(self.temp_dir, "original.tex")
        portable = os.path.join(self.temp_dir, "portable.tex")

        content = r"""\usepackage{fontspec}%
\begin{document}%
\begin{tabular}{|l|>{\AtkinsonMono}l|}
Content
\end{tabular}
\end{document}%"""

        with open(original, "w", encoding="utf-8") as f:
            f.write(content)

        create_portable_tex(original, portable)

        with open(portable, "r", encoding="utf-8") as f:
            result = f.read()

        self.assertIn(r">{\ttfamily}l|", result)
        self.assertNotIn(r">{\AtkinsonMono}", result)

    def test_handles_multiline_font_declarations(self):
        """Test that multi-line font declarations are handled."""
        original = os.path.join(self.temp_dir, "original.tex")
        portable = os.path.join(self.temp_dir, "portable.tex")

        content = r"""\usepackage{fontspec}%
\setsansfont[
    Path=/some/path/,
    UprightFont=*-Regular,
    BoldFont=*-Bold
]{AtkinsonHyperlegible}%
\begin{document}%
Test content
\end{document}%"""

        with open(original, "w", encoding="utf-8") as f:
            f.write(content)

        create_portable_tex(original, portable)

        with open(portable, "r", encoding="utf-8") as f:
            result = f.read()

        # All lines of the font declaration should be commented
        self.assertIn(r"% \setsansfont[", result)
        self.assertIn(r"%     Path=/some/path/,", result)
        self.assertIn(r"%     UprightFont=*-Regular,", result)

    def test_removes_setmonofont_declarations(self):
        """Test that setmonofont declarations are also removed."""
        original = os.path.join(self.temp_dir, "original.tex")
        portable = os.path.join(self.temp_dir, "portable.tex")

        content = r"""\usepackage{fontspec}%
\setsansfont[Path=/Users/test/fonts/]{AtkinsonHyperlegible}%
\setmonofont[Path=/Users/test/fonts/]{AtkinsonHyperlegibleMono-VariableFont_wght}%
\begin{document}%
Test content
\end{document}%"""

        with open(original, "w", encoding="utf-8") as f:
            f.write(content)

        create_portable_tex(original, portable)

        with open(portable, "r", encoding="utf-8") as f:
            result = f.read()

        # Both font declarations should be commented out
        self.assertIn(r"% \setsansfont[", result)
        self.assertIn(r"% \setmonofont[", result)
        # Verify the uncommented version is NOT present
        lines = result.split("\n")
        for line in lines:
            if "setmonofont" in line and not line.strip().startswith("%"):
                self.fail(f"Found uncommented setmonofont: {line}")

    def test_adds_portability_note(self):
        """Test that portability note is added to document."""
        original = os.path.join(self.temp_dir, "original.tex")
        portable = os.path.join(self.temp_dir, "portable.tex")

        content = r"""\usepackage{fontspec}%
\begin{document}%
Test content
\end{document}%"""

        with open(original, "w", encoding="utf-8") as f:
            f.write(content)

        create_portable_tex(original, portable)

        with open(portable, "r", encoding="utf-8") as f:
            result = f.read()

        self.assertIn("% NOTE: This is a portable version", result)
        self.assertIn("pdflatex instead of xelatex", result)


class TestCreateFontsTex(unittest.TestCase):
    """Test fonts TEX file creation."""

    def setUp(self):
        """Set up test fixtures."""
        self.temp_dir = tempfile.mkdtemp()

    def tearDown(self):
        """Clean up test fixtures."""
        import shutil

        shutil.rmtree(self.temp_dir)

    def test_replaces_absolute_paths_with_relative(self):
        """Test that absolute font paths are replaced with ./fonts/."""
        original = os.path.join(self.temp_dir, "original.tex")
        fonts = os.path.join(self.temp_dir, "fonts.tex")

        content = r"""\usepackage{fontspec}%
\setsansfont[
    Path=/Users/someone/fonts/,
    UprightFont=*-Regular
]{AtkinsonHyperlegible}%
\begin{document}%
Test content
\end{document}%"""

        with open(original, "w", encoding="utf-8") as f:
            f.write(content)

        create_fonts_tex(original, fonts)

        with open(fonts, "r", encoding="utf-8") as f:
            result = f.read()

        self.assertIn("Path=./fonts/,", result)
        self.assertNotIn("Path=/Users/someone/fonts/,", result)

    def test_handles_linux_paths(self):
        """Test that Linux absolute paths are replaced."""
        original = os.path.join(self.temp_dir, "original.tex")
        fonts = os.path.join(self.temp_dir, "fonts.tex")

        content = r"""\usepackage{fontspec}%
\setsansfont[
    Path=/home/user/fonts/,
    UprightFont=*-Regular
]{AtkinsonHyperlegible}%
\begin{document}%
Test content
\end{document}%"""

        with open(original, "w", encoding="utf-8") as f:
            f.write(content)

        create_fonts_tex(original, fonts)

        with open(fonts, "r", encoding="utf-8") as f:
            result = f.read()

        self.assertIn("Path=./fonts/,", result)
        self.assertNotIn("Path=/home/user/fonts/,", result)

    def test_adds_fonts_note(self):
        """Test that fonts note is added to document."""
        original = os.path.join(self.temp_dir, "original.tex")
        fonts = os.path.join(self.temp_dir, "fonts.tex")

        content = r"""\usepackage{fontspec}%
\begin{document}%
Test content
\end{document}%"""

        with open(original, "w", encoding="utf-8") as f:
            f.write(content)

        create_fonts_tex(original, fonts)

        with open(fonts, "r", encoding="utf-8") as f:
            result = f.read()

        self.assertIn("% NOTE: This version uses custom fonts", result)
        self.assertIn("./fonts/ directory", result)
        self.assertIn("xelatex", result)


class TestCreateSourcesZip(unittest.TestCase):
    """Test ZIP sources creation."""

    def setUp(self):
        """Set up test fixtures."""
        self.temp_dir = tempfile.mkdtemp()

    def tearDown(self):
        """Clean up test fixtures."""
        import shutil

        shutil.rmtree(self.temp_dir)

    def test_creates_zip_with_both_tex_versions(self):
        """Test that ZIP contains both portable and fonts TEX files."""
        prefix = os.path.join(self.temp_dir, "test")

        # Create original TEX file
        original_tex = f"{prefix}_report.tex"
        with open(original_tex, "w", encoding="utf-8") as f:
            f.write(
                r"""\usepackage{fontspec}%
\setsansfont[Path=/Users/test/fonts/,UprightFont=*-Regular]{AtkinsonHyperlegible}%
\begin{document}%
Test content
\end{document}%"""
            )

        # Create some chart PDFs
        for chart in ["stats", "daily", "histogram"]:
            chart_file = f"{prefix}_{chart}.pdf"
            with open(chart_file, "w", encoding="utf-8") as f:
                f.write("fake pdf content")

        # Create ZIP
        zip_path = create_sources_zip(prefix)

        # Verify ZIP was created
        self.assertTrue(os.path.exists(zip_path))
        self.assertEqual(zip_path, f"{prefix}_sources.zip")

        # Verify ZIP contents
        with zipfile.ZipFile(zip_path, "r") as zf:
            names = zf.namelist()

            # Should have README
            self.assertIn("README.txt", names)

            # Should have both TEX files
            self.assertIn("test_report.tex", names)  # Portable version
            self.assertIn("test_report_fonts.tex", names)  # Fonts version

            # Should have charts
            self.assertIn("test_stats.pdf", names)
            self.assertIn("test_daily.pdf", names)
            self.assertIn("test_histogram.pdf", names)

            # Should NOT have the final report PDF
            self.assertNotIn("test_report.pdf", names)

    def test_custom_output_path(self):
        """Test ZIP creation with custom output path."""
        prefix = os.path.join(self.temp_dir, "test")
        custom_zip = os.path.join(self.temp_dir, "custom.zip")

        # Create original TEX file
        original_tex = f"{prefix}_report.tex"
        with open(original_tex, "w", encoding="utf-8") as f:
            f.write(r"\begin{document}%\nTest\n\end{document}%")

        # Create ZIP with custom path
        zip_path = create_sources_zip(prefix, custom_zip)

        # Verify custom path was used
        self.assertEqual(zip_path, custom_zip)
        self.assertTrue(os.path.exists(custom_zip))

    def test_raises_error_when_no_files_found(self):
        """Test that FileNotFoundError is raised when no files exist."""
        prefix = os.path.join(self.temp_dir, "nonexistent")

        with self.assertRaises(FileNotFoundError):
            create_sources_zip(prefix)

    def test_readme_content(self):
        """Test that README has correct content."""
        prefix = os.path.join(self.temp_dir, "test")

        # Create minimal TEX file
        original_tex = f"{prefix}_report.tex"
        with open(original_tex, "w", encoding="utf-8") as f:
            f.write(r"\begin{document}%\nTest\n\end{document}%")

        # Create ZIP
        zip_path = create_sources_zip(prefix)

        # Read README from ZIP
        with zipfile.ZipFile(zip_path, "r") as zf:
            readme = zf.read("README.txt").decode("utf-8")

            # Check for key sections
            self.assertIn("Velocity Report Source Files", readme)
            self.assertIn("Quick Start (Portable Version)", readme)
            self.assertIn("Using Custom Fonts", readme)
            self.assertIn("pdflatex", readme)
            self.assertIn("xelatex", readme)
            self.assertIn("brailleinstitute.org", readme)

    def test_portable_tex_uses_standard_fonts(self):
        """Test that portable TEX in ZIP uses standard fonts."""
        prefix = os.path.join(self.temp_dir, "test")

        # Create original TEX with custom fonts
        original_tex = f"{prefix}_report.tex"
        with open(original_tex, "w", encoding="utf-8") as f:
            f.write(
                r"""\usepackage{fontspec}%
\setsansfont[Path=/Users/test/fonts/]{AtkinsonHyperlegible}%
\begin{document}%
Some \AtkinsonMono{code} here.
\end{document}%"""
            )

        # Create ZIP
        zip_path = create_sources_zip(prefix)

        # Read portable TEX from ZIP
        with zipfile.ZipFile(zip_path, "r") as zf:
            portable_content = zf.read("test_report.tex").decode("utf-8")

            # Should have fontspec commented out
            self.assertIn("% \\usepackage{fontspec}%", portable_content)

            # Should use \texttt instead of \AtkinsonMono
            self.assertIn(r"\texttt{code}", portable_content)
            self.assertNotIn(r"\AtkinsonMono{", portable_content)

    def test_fonts_tex_uses_relative_paths(self):
        """Test that fonts TEX in ZIP uses ./fonts/ paths."""
        prefix = os.path.join(self.temp_dir, "test")

        # Create original TEX with absolute paths
        original_tex = f"{prefix}_report.tex"
        with open(original_tex, "w", encoding="utf-8") as f:
            f.write(
                r"""\usepackage{fontspec}%
\setsansfont[Path=/Users/test/fonts/,UprightFont=*-Regular]{AtkinsonHyperlegible}%
\begin{document}%
Test content
\end{document}%"""
            )

        # Create ZIP
        zip_path = create_sources_zip(prefix)

        # Read fonts TEX from ZIP
        with zipfile.ZipFile(zip_path, "r") as zf:
            fonts_content = zf.read("test_report_fonts.tex").decode("utf-8")

            # Should use relative path
            self.assertIn("Path=./fonts/,", fonts_content)
            self.assertNotIn("Path=/Users/test/fonts/,", fonts_content)

    def test_fonts_directory_included(self):
        """Test that fonts/ directory with instruction file is included in ZIP."""
        prefix = os.path.join(self.temp_dir, "test")

        # Create minimal TEX file
        original_tex = f"{prefix}_report.tex"
        with open(original_tex, "w", encoding="utf-8") as f:
            f.write(r"\begin{document}%\nTest\n\end{document}%")

        # Create ZIP
        zip_path = create_sources_zip(prefix)

        # Check ZIP contents
        with zipfile.ZipFile(zip_path, "r") as zf:
            names = zf.namelist()

            # Should have fonts directory with instruction file
            self.assertIn("fonts/FONTS_GO_HERE.txt", names)

            # Read the instruction file
            fonts_instruction = zf.read("fonts/FONTS_GO_HERE.txt").decode("utf-8")

            # Verify it contains the key information
            self.assertIn("brailleinstitute.org/freefont", fonts_instruction)
            self.assertIn("AtkinsonHyperlegible-Regular.ttf", fonts_instruction)
            self.assertIn("AtkinsonHyperlegible-Bold.ttf", fonts_instruction)
            self.assertIn(
                "AtkinsonHyperlegibleMono-VariableFont_wght.ttf", fonts_instruction
            )
            self.assertIn("xelatex", fonts_instruction)

    def test_readme_fallback_when_file_missing(self):
        """Test that fallback README content is used when zip_readme.md doesn't exist."""
        import shutil
        from pathlib import Path

        # Path to the README template
        readme_template = Path(__file__).parent / ".." / "core" / "zip_readme.md"
        readme_template = readme_template.resolve()

        # Temporarily move the README file so it appears missing
        backup_path = readme_template.with_suffix(".md.backup")
        if readme_template.exists():
            shutil.move(readme_template, backup_path)

        try:
            prefix = os.path.join(self.temp_dir, "test")

            # Create minimal TEX file
            original_tex = f"{prefix}_report.tex"
            with open(original_tex, "w", encoding="utf-8") as f:
                f.write(r"\begin{document}%\nTest\n\end{document}%")

            # Create ZIP
            zip_path = create_sources_zip(prefix)

            # Read README from ZIP
            with zipfile.ZipFile(zip_path, "r") as zf:
                readme = zf.read("README.txt").decode("utf-8")

                # Check for fallback content
                self.assertIn("Velocity Report Source Files", readme)
                self.assertIn("https://github.com/banshee-data/velocity.report", readme)

        finally:
            # Restore the README file
            if backup_path.exists():
                shutil.move(backup_path, readme_template)


class TestPortableTexCompilation(unittest.TestCase):
    """Integration test to verify portable TEX actually compiles with pdflatex."""

    def setUp(self):
        """Set up test fixtures."""
        self.temp_dir = tempfile.mkdtemp()

    def tearDown(self):
        """Clean up test fixtures."""
        import shutil

        shutil.rmtree(self.temp_dir)

    def test_portable_tex_compiles_with_pdflatex(self):
        """Integration test: verify portable TEX file from ZIP compiles with pdflatex.

        This is a critical end-to-end test that validates the portable version
        actually works with standard pdflatex. It:
        1. Creates a realistic TEX file with custom fonts (as generated normally)
        2. Creates a ZIP with both versions
        3. Extracts the portable version
        4. Attempts to compile it with pdflatex
        5. Verifies compilation succeeds and produces a PDF

        This test requires pdflatex to be installed and will be skipped if not available.
        """

        # Check if pdflatex is available
        try:
            result = subprocess.run(
                ["pdflatex", "--version"],
                capture_output=True,
                timeout=5,
            )
            if result.returncode != 0:
                self.skipTest("pdflatex not available")
        except (FileNotFoundError, subprocess.TimeoutExpired):
            self.skipTest("pdflatex not available")

        # Create a realistic TEX file with custom fonts (like our generator creates)
        prefix = os.path.join(self.temp_dir, "test")
        original_tex = f"{prefix}_report.tex"

        # This is representative of what our PDF generator creates
        realistic_content = r"""\documentclass[11pt,letterpaper]{article}%
\usepackage{fontspec}%
\usepackage{graphicx}%
\usepackage{geometry}%
\usepackage{fancyhdr}%
\usepackage{array}%
\usepackage{longtable}%
\usepackage{booktabs}%
\geometry{letterpaper,margin=0.75in}%
\setsansfont[Path=/Users/test/fonts/,UprightFont=*-Regular,BoldFont=*-Bold]{AtkinsonHyperlegible}%
\setmonofont[Path=/Users/test/fonts/]{AtkinsonHyperlegibleMono-VariableFont_wght}%
\pagestyle{fancy}%
\fancyhf{}%
\fancyfoot[L]{\small 2025-06-02 to 2025-06-04}%
\fancyfoot[R]{\small Page \thepage}%
\renewcommand{\footrulewidth}{0.8pt}%
\begin{document}%
\sffamily%
\section*{Velocity Report}%
\subsection*{Site Information}%
Location: Clarendon Avenue, San Francisco\\
Survey Period: 2025-06-02 to 2025-06-04\\

\subsection*{Overall Statistics}%
\begin{tabular}{|l|r|}%
\hline%
Vehicle Count & 3,469 \\%
\hline%
P50 Speed & 30.54 mph \\%
\hline%
P85 Speed & 36.94 mph \\%
\hline%
P98 Speed & 43.05 mph \\%
\hline%
Max Speed & 53.52 mph \\%
\hline%
\end{tabular}%

\subsection*{Survey Parameters}%
Roll-up Period: 1h\\
Units: mph\\
Timezone: US/Pacific\\

Some example code: \AtkinsonMono{radar.measure()} in monospace.

\subsection*{Table with Column Font Spec}%
\begin{tabular}{|l|>{\AtkinsonMono}l|}%
\hline%
Label & Code \\%
\hline%
Test & function() \\%
\hline%
\end{tabular}%

\end{document}%"""

        with open(original_tex, "w", encoding="utf-8") as f:
            f.write(realistic_content)

        # Create the ZIP with both versions
        zip_path = create_sources_zip(prefix)

        # Extract the portable version to a compilation directory
        compile_dir = os.path.join(self.temp_dir, "compile")
        os.makedirs(compile_dir)

        with zipfile.ZipFile(zip_path, "r") as zf:
            # Extract the portable TEX file
            portable_tex_name = "test_report.tex"
            zf.extract(portable_tex_name, compile_dir)

        portable_tex_path = os.path.join(compile_dir, portable_tex_name)

        # Verify the portable TEX was extracted
        self.assertTrue(os.path.exists(portable_tex_path))

        # Read and verify transformations were applied
        with open(portable_tex_path, "r", encoding="utf-8") as f:
            portable_content = f.read()

        # Sanity checks on the portable version
        self.assertIn(
            "% \\usepackage{fontspec}%",
            portable_content,
            "fontspec should be commented out",
        )
        self.assertIn(
            r"\texttt{radar.measure()}",
            portable_content,
            "AtkinsonMono should be replaced with texttt",
        )
        self.assertIn(
            r">{\ttfamily}l|",
            portable_content,
            "Table column fonts should use ttfamily",
        )
        self.assertIn(
            "NOTE: This is a portable version",
            portable_content,
            "Portability note should be present",
        )

        # Compile with pdflatex
        try:
            result = subprocess.run(
                [
                    "pdflatex",
                    "-interaction=nonstopmode",
                    "-halt-on-error",
                    portable_tex_name,
                ],
                cwd=compile_dir,
                capture_output=True,
                timeout=30,
                text=True,
            )

            # Check compilation result
            pdf_path = os.path.join(compile_dir, "test_report.pdf")

            if result.returncode != 0:
                # Provide detailed error information
                print("\n=== PDFLATEX COMPILATION FAILED ===", file=sys.stderr)
                print("STDOUT:", file=sys.stderr)
                print(result.stdout, file=sys.stderr)
                print("\nSTDERR:", file=sys.stderr)
                print(result.stderr, file=sys.stderr)

                # Try to read the log file for more details
                log_path = os.path.join(compile_dir, "test_report.log")
                if os.path.exists(log_path):
                    print("\n=== LATEX LOG (last 50 lines) ===", file=sys.stderr)
                    with open(log_path, "r", encoding="utf-8", errors="ignore") as lf:
                        log_lines = lf.readlines()
                        print("".join(log_lines[-50:]), file=sys.stderr)

                self.fail(
                    f"pdflatex compilation failed with return code {result.returncode}. "
                    "See stderr for details."
                )

            # Verify PDF was created
            self.assertTrue(
                os.path.exists(pdf_path),
                "PDF file should be created after successful compilation",
            )

            # Verify PDF has some content (not empty)
            pdf_size = os.path.getsize(pdf_path)
            self.assertGreater(
                pdf_size,
                1000,
                f"PDF should have substantial content, got {pdf_size} bytes",
            )

        except subprocess.TimeoutExpired:
            self.fail("pdflatex compilation timed out after 30 seconds")


if __name__ == "__main__":
    unittest.main()
