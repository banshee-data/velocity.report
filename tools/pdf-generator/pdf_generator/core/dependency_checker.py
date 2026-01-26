#!/usr/bin/env python3
"""Dependency checker for velocity.report system requirements.

This module validates that all required dependencies are available:
- Python packages (matplotlib, PyLaTeX, PIL, etc.)
- LaTeX distribution (xelatex)
- Virtual environment setup
- System utilities
"""

import importlib.util
import os
import shutil
import subprocess
import sys
from typing import List, Tuple, Optional


class DependencyCheckResult:
    """Results of a dependency check."""

    def __init__(
        self, name: str, available: bool, details: str = "", critical: bool = True
    ):
        self.name = name
        self.available = available
        self.details = details
        self.critical = critical

    def __repr__(self):
        status = "✓" if self.available else "✗"
        critical_marker = " (CRITICAL)" if self.critical and not self.available else ""
        detail_str = f" - {self.details}" if self.details else ""
        return f"[{status}] {self.name}{critical_marker}{detail_str}"


class DependencyChecker:
    """Checks system dependencies for velocity.report."""

    def __init__(self, verbose: bool = False):
        self.verbose = verbose
        self.results: List[DependencyCheckResult] = []

    def check_all(self) -> Tuple[bool, List[DependencyCheckResult]]:
        """Run all dependency checks.

        Returns:
            Tuple of (all_ok, results_list)
        """
        self.results = []

        # Check Python version
        self._check_python_version()

        # Check virtual environment
        self._check_venv()

        # Check critical Python packages
        self._check_python_package("matplotlib", "Chart generation", critical=True)
        self._check_python_package(
            "pylatex", "LaTeX document generation", critical=True
        )
        self._check_python_package("numpy", "Numerical operations", critical=True)
        self._check_python_package("PIL", "Image processing (Pillow)", critical=False)

        # Check optional packages
        self._check_python_package("cairosvg", "SVG to PDF conversion", critical=False)
        self._check_python_package("requests", "HTTP API requests", critical=True)

        # Check LaTeX installation
        self._check_latex()

        # Check optional system utilities
        self._check_system_command(
            "inkscape", "SVG conversion fallback", critical=False
        )
        self._check_system_command(
            "rsvg-convert", "SVG conversion fallback", critical=False
        )

        # Determine overall status
        all_ok = all(r.available or not r.critical for r in self.results)

        return all_ok, self.results

    def _check_python_version(self):
        """Check Python version is 3.9+."""
        version = sys.version_info
        version_str = f"{version.major}.{version.minor}.{version.micro}"

        if version.major >= 3 and version.minor >= 9:
            self.results.append(
                DependencyCheckResult(
                    "Python Version", True, f"Python {version_str}", critical=True
                )
            )
        else:
            self.results.append(
                DependencyCheckResult(
                    "Python Version",
                    False,
                    f"Python {version_str} (requires 3.9+)",
                    critical=True,
                )
            )

    def _check_venv(self):
        """Check if running in a virtual environment."""
        in_venv = (
            hasattr(sys, "real_prefix")
            or (hasattr(sys, "base_prefix") and sys.base_prefix != sys.prefix)
            or os.environ.get("VIRTUAL_ENV") is not None
        )

        if in_venv:
            venv_path = os.environ.get("VIRTUAL_ENV", sys.prefix)
            self.results.append(
                DependencyCheckResult(
                    "Virtual Environment", True, f"Active: {venv_path}", critical=False
                )
            )
        else:
            self.results.append(
                DependencyCheckResult(
                    "Virtual Environment",
                    False,
                    "Not detected - recommended for dependency isolation",
                    critical=False,
                )
            )

    def _check_python_package(
        self, package_name: str, description: str, critical: bool = True
    ):
        """Check if a Python package is available."""
        # Handle special case for PIL (Pillow)
        import_name = "PIL" if package_name == "PIL" else package_name

        spec = importlib.util.find_spec(import_name)
        available = spec is not None

        if available:
            try:
                # Try to get version
                if import_name == "PIL":
                    import PIL

                    version = getattr(PIL, "__version__", "unknown")
                else:
                    module = importlib.import_module(import_name)
                    version = getattr(module, "__version__", "unknown")

                details = f"{description} (v{version})"
            except Exception:  # Version check or import failed
                details = description
        else:
            install_name = "Pillow" if package_name == "PIL" else package_name
            details = f"{description} - Install with: pip install {install_name}"

        self.results.append(
            DependencyCheckResult(package_name, available, details, critical)
        )

    def _check_latex(self):
        """Check if LaTeX (xelatex) is available."""
        xelatex_path = shutil.which("xelatex")

        if xelatex_path:
            # Try to get version
            try:
                result = subprocess.run(
                    ["xelatex", "--version"], capture_output=True, text=True, timeout=5
                )
                # Extract first line which usually has version
                version_line = result.stdout.split("\n")[0] if result.stdout else ""
                details = f"Found at {xelatex_path}"
                if version_line:
                    details = f"{version_line}"
            except Exception:  # Version check failed or timeout
                details = f"Found at {xelatex_path}"

            self.results.append(
                DependencyCheckResult("xelatex (LaTeX)", True, details, critical=True)
            )
        else:
            install_msg = (
                "LaTeX distribution not found. Install:\n"
                "  - macOS: brew install --cask mactex-no-gui\n"
                "  - Ubuntu/Debian: sudo apt-get install texlive-xetex\n"
                "  - Windows: Install MiKTeX or TeX Live"
            )
            self.results.append(
                DependencyCheckResult(
                    "xelatex (LaTeX)", False, install_msg, critical=True
                )
            )

    def _check_system_command(
        self, command: str, description: str, critical: bool = False
    ):
        """Check if a system command is available."""
        cmd_path = shutil.which(command)

        if cmd_path:
            self.results.append(
                DependencyCheckResult(
                    command,
                    True,
                    f"{description} - Found at {cmd_path}",
                    critical=critical,
                )
            )
        else:
            self.results.append(
                DependencyCheckResult(
                    command,
                    False,
                    f"{description} - Not found (optional)",
                    critical=critical,
                )
            )

    def print_results(self, results: Optional[List[DependencyCheckResult]] = None):
        """Print formatted results."""
        if results is None:
            results = self.results

        print("\n" + "=" * 70)
        print("DEPENDENCY CHECK RESULTS")
        print("=" * 70 + "\n")

        critical_failures = []
        warnings = []
        successes = []

        for result in results:
            if not result.available and result.critical:
                critical_failures.append(result)
            elif not result.available:
                warnings.append(result)
            else:
                successes.append(result)

        # Print successes
        if successes:
            print("✓ Available Dependencies:")
            print("-" * 70)
            for result in successes:
                print(f"  {result}")
            print()

        # Print warnings
        if warnings:
            print("⚠ Optional Dependencies (missing but not critical):")
            print("-" * 70)
            for result in warnings:
                print(f"  {result}")
            print()

        # Print critical failures
        if critical_failures:
            print("✗ CRITICAL MISSING DEPENDENCIES:")
            print("-" * 70)
            for result in critical_failures:
                print(f"  {result}")
            print()

        # Summary
        print("=" * 70)
        total = len(results)
        available = sum(1 for r in results if r.available)
        critical_missing = len(critical_failures)

        print(f"Summary: {available}/{total} dependencies available")

        if critical_missing > 0:
            print(f"         {critical_missing} CRITICAL dependencies missing")
            print("\n⚠ System is NOT ready. Install missing dependencies above.")
            return False
        elif warnings:
            print(f"         {len(warnings)} optional dependencies missing")
            print("\n✓ System is ready (some optional features may be unavailable)")
            return True
        else:
            print("\n✓ All dependencies available! System is ready.")
            return True


def check_dependencies(verbose: bool = False) -> bool:
    """Convenience function to check dependencies and print results.

    Args:
        verbose: Print verbose output

    Returns:
        True if all critical dependencies are available
    """
    checker = DependencyChecker(verbose=verbose)
    all_ok, results = checker.check_all()
    system_ready = checker.print_results(results)
    return system_ready


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(
        description="Check velocity.report system dependencies"
    )
    parser.add_argument("-v", "--verbose", action="store_true", help="Verbose output")

    args = parser.parse_args()

    system_ready = check_dependencies(verbose=args.verbose)
    sys.exit(0 if system_ready else 1)
