"""Pytest configuration for pdf_generator tests."""

import os
import sys
from pathlib import Path

# Add parent directory to Python path so package imports work
# This allows: from pdf_generator.core.api_client import ...
pkg_root = Path(__file__).parent.parent.parent
if str(pkg_root) not in sys.path:
    sys.path.insert(0, str(pkg_root))

# Also add the repository root for any legacy imports
ROOT = pkg_root.parent.parent  # tools/pdf-generator -> tools -> velocity.report
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))
