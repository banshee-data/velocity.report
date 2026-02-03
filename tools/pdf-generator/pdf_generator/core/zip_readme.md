# Velocity Report Source Files

This ZIP file contains the LaTeX source and chart PDFs for your velocity report.

## Contents

- `*_report.tex` - LaTeX source (portable, standard fonts - compile with pdflatex)
- `*_report_fonts.tex` - LaTeX source (custom fonts - compile with xelatex)
- `*_stats.pdf` - Statistics chart over time
- `*_daily.pdf` - Daily summary chart (if available)
- `*_histogram.pdf` - Velocity distribution histogram
- `*.log` - LaTeX compilation log (if available)

## Quick Start (Portable Version)

The portable version uses standard LaTeX fonts and works out-of-box:

```bash
pdflatex *_report.tex
```

## Using Custom Fonts (Original Version)

To compile with the original custom fonts (Atkinson Hyperlegible):

### 1. Download the fonts

Visit: https://brailleinstitute.org/freefont

Download the font family and extract these files:

- AtkinsonHyperlegible-Regular.ttf
- AtkinsonHyperlegible-Italic.ttf
- AtkinsonHyperlegible-Bold.ttf
- AtkinsonHyperlegible-BoldItalic.ttf
- AtkinsonHyperlegibleMono-VariableFont_wght.ttf
- AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf

### 2. Create fonts directory

```bash
mkdir fonts
# Move or copy the .ttf files into the fonts/ directory
```

### 3. Compile with XeLaTeX

```bash
xelatex *_report_fonts.tex
```

The `*_report_fonts.tex` file expects fonts in `./fonts/` relative to the TEX file.

## Editing Charts

The chart PDFs are included so you can:

1. Edit the TEX file to adjust layout, text, or tables
2. Replace chart PDFs with your own custom versions
3. Recompile to generate a modified report

## Notes

- The portable version (`*_report.tex`) uses Latin Modern fonts (standard LaTeX)
- The fonts version (`*_report_fonts.tex`) uses Atkinson Hyperlegible (better readability)
- Chart PDFs are final rendered versions and cannot be edited directly
- To modify chart data, regenerate reports using the full system

## Support

For issues or questions, see: https://github.com/banshee-data/velocity.report
