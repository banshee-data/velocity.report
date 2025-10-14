import os
import sys

# Get the tools/pdf-generator directory (package root)
PKG_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
if PKG_ROOT not in sys.path:
    sys.path.insert(0, PKG_ROOT)

# Get the velocity.report root for matplotlib shims
REPO_ROOT = os.path.abspath(os.path.join(PKG_ROOT, "..", ".."))
if REPO_ROOT not in sys.path:
    sys.path.insert(0, REPO_ROOT)
