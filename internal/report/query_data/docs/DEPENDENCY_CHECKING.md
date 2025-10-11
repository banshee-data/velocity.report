# Dependency Checking

## Overview

The velocity.report system now includes a comprehensive dependency checker to validate that all required software is installed before attempting to generate reports.

## Quick Start

Check your system dependencies before running reports:

```bash
# Check dependencies using the main script
python get_stats.py --check

# Or use the standalone checker
python dependency_checker.py

# Or check from the API script
python generate_report_api.py --check
```

## What It Checks

### Critical Dependencies (Required)
- **Python Version**: 3.9 or higher
- **matplotlib**: Chart generation library
- **PyLaTeX**: LaTeX document generation
- **numpy**: Numerical operations
- **requests**: HTTP API requests
- **xelatex**: LaTeX distribution for PDF generation

### Optional Dependencies (Nice to have)
- **Virtual Environment**: Recommended for dependency isolation
- **Pillow (PIL)**: Image processing
- **cairosvg**: SVG to PDF conversion (primary method)
- **inkscape**: SVG conversion fallback
- **rsvg-convert**: SVG conversion fallback

## Example Output

When all dependencies are available:
```
======================================================================
DEPENDENCY CHECK RESULTS
======================================================================

✓ Available Dependencies:
----------------------------------------------------------------------
  [✓] Python Version - Python 3.13.7
  [✓] Virtual Environment - Active: /path/to/.venv
  [✓] matplotlib - Chart generation (v3.10.6)
  [✓] pylatex - LaTeX document generation (v1.4.2)
  [✓] numpy - Numerical operations (v2.3.3)
  [✓] PIL - Image processing (Pillow) (v11.3.0)
  [✓] requests - HTTP API requests (v2.32.5)
  [✓] xelatex (LaTeX) - XeTeX 3.141592653-2.6-0.999997

⚠ Optional Dependencies (missing but not critical):
----------------------------------------------------------------------
  [✗] cairosvg - SVG to PDF conversion - Install with: pip install cairosvg

======================================================================
Summary: 8/9 dependencies available
         1 optional dependencies missing

✓ System is ready (some optional features may be unavailable)
```

When critical dependencies are missing:
```
======================================================================
DEPENDENCY CHECK RESULTS
======================================================================

✗ CRITICAL MISSING DEPENDENCIES:
----------------------------------------------------------------------
  [✗] xelatex (LaTeX) (CRITICAL) - LaTeX distribution not found. Install:
    - macOS: brew install --cask mactex-no-gui
    - Ubuntu/Debian: sudo apt-get install texlive-xetex
    - Windows: Install MiKTeX or TeX Live

======================================================================
Summary: 7/9 dependencies available
         1 CRITICAL dependencies missing

⚠ System is NOT ready. Install missing dependencies above.
```

## Installing Missing Dependencies

### Python Packages
```bash
# Activate your virtual environment first (recommended)
source .venv/bin/activate  # On macOS/Linux
# or
.venv\Scripts\activate  # On Windows

# Install missing packages
pip install matplotlib pylatex numpy requests pillow
```

### LaTeX Distribution

**macOS:**
```bash
brew install --cask mactex-no-gui
```

**Ubuntu/Debian:**
```bash
sudo apt-get install texlive-xetex texlive-fonts-recommended texlive-fonts-extra
```

**Windows:**
Download and install either:
- [MiKTeX](https://miktex.org/download)
- [TeX Live](https://www.tug.org/texlive/)

### Optional SVG Tools

**macOS:**
```bash
brew install librsvg  # For rsvg-convert
brew install inkscape
```

**Ubuntu/Debian:**
```bash
sudo apt-get install librsvg2-bin inkscape
```

## Exit Codes

The dependency checker returns appropriate exit codes:
- `0`: All critical dependencies available (system ready)
- `1`: One or more critical dependencies missing (system NOT ready)

This makes it suitable for use in scripts and CI/CD pipelines:

```bash
if python get_stats.py --check; then
    echo "System ready, proceeding with report generation"
    python get_stats.py config.json
else
    echo "System not ready, please install missing dependencies"
    exit 1
fi
```

## Integration with Main Scripts

The `--check` flag has been added to both main entry points:

1. **get_stats.py**: The primary CLI tool for report generation
2. **generate_report_api.py**: The web API entry point

When using `--check`, the config file argument becomes optional:

```bash
# These all work:
python get_stats.py --check
python generate_report_api.py --check
./dependency_checker.py
```

## Programmatic Usage

You can also use the dependency checker in your own Python code:

```python
from dependency_checker import check_dependencies, DependencyChecker

# Simple check
if check_dependencies():
    print("System ready!")
else:
    print("Dependencies missing!")

# Advanced usage with custom checks
checker = DependencyChecker(verbose=True)
all_ok, results = checker.check_all()

for result in results:
    if not result.available and result.critical:
        print(f"Missing critical: {result.name}")
```

## Troubleshooting

### Virtual Environment Not Detected

If you're running in a virtual environment but it's not detected:
- Ensure you activated it: `source .venv/bin/activate`
- Check `VIRTUAL_ENV` environment variable: `echo $VIRTUAL_ENV`

### LaTeX Not Found But Installed

If LaTeX is installed but not found:
- Verify `xelatex` is in your PATH: `which xelatex`
- On macOS, you may need to add `/Library/TeX/texbin` to your PATH
- Try running `xelatex --version` directly

### Package Import Errors

If a package shows as "not found" but you just installed it:
- Ensure you're in the correct virtual environment
- Try: `python -c "import matplotlib; print(matplotlib.__version__)"`
- Restart your terminal/shell after installation

## See Also

- [Installation Guide](../../../README.md#installation)
- [Configuration Guide](README.md)
- [Troubleshooting](README.md#troubleshooting)
