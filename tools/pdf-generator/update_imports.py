#!/usr/bin/env python3
"""Update imports to use new package structure."""

import re
from pathlib import Path

# Modules in core/
CORE_MODULES = [
    "api_client",
    "chart_builder",
    "chart_saver",
    "config_manager",
    "data_transformers",
    "date_parser",
    "dependency_checker",
    "document_builder",
    "map_utils",
    "pdf_generator",
    "report_sections",
    "stats_utils",
    "table_builders",
]


def update_file(filepath: Path):
    """Update imports in a single file."""
    content = filepath.read_text()
    original = content

    for module in CORE_MODULES:
        # Update: from module import ...
        pattern = rf"^from {module} import"
        replacement = rf"from pdf_generator.core.{module} import"
        content = re.sub(pattern, replacement, content, flags=re.MULTILINE)

        # Update: import module
        pattern = rf"^import {module}$"
        replacement = rf"import pdf_generator.core.{module} as {module}"
        content = re.sub(pattern, replacement, content, flags=re.MULTILINE)

    if content != original:
        filepath.write_text(content)
        print(f"✓ Updated {filepath}")
        return True
    else:
        print(f"  No changes needed: {filepath}")
        return False


def main():
    base = Path("pdf_generator")
    updated_count = 0

    # Update CLI files
    print("\n=== Updating CLI files ===")
    for file in (base / "cli").glob("*.py"):
        if file.name != "__init__.py":
            if update_file(file):
                updated_count += 1

    # Update core files (they import from each other)
    print("\n=== Updating core files ===")
    for file in (base / "core").glob("*.py"):
        if file.name != "__init__.py":
            if update_file(file):
                updated_count += 1

    # Update test files
    print("\n=== Updating test files ===")
    for file in (base / "tests").glob("test_*.py"):
        if update_file(file):
            updated_count += 1

    # Update conftest
    if (base / "tests" / "conftest.py").exists():
        if update_file(base / "tests" / "conftest.py"):
            updated_count += 1

    print(f"\n✅ Updated {updated_count} files")


if __name__ == "__main__":
    main()
