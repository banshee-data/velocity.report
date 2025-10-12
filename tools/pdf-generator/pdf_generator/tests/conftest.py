import os
import sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
# Ensure the project root is on sys.path so top-level shims (like matplotlib/) are
# importable during test collection.
if ROOT not in sys.path:
    sys.path.insert(0, ROOT)

# Ensure the package directory (where stats_utils lives) is also on sys.path so
# imports like `from stats_utils import ...` resolve correctly.
PKG_DIR = os.path.join(ROOT, "internal", "report", "query_data")
if PKG_DIR not in sys.path:
    sys.path.insert(0, PKG_DIR)
