#!/usr/bin/env python3
"""Ensure DocumentBuilder package list matches velocity-report.ini packages."""

from __future__ import annotations

import ast
import re
import sys
from pathlib import Path
from typing import Dict, Optional

ROOT_DIR = Path(__file__).resolve().parents[1]
DOC_BUILDER_PATH = ROOT_DIR / "pdf_generator" / "core" / "document_builder.py"
FORMAT_INI_PATH = ROOT_DIR / "tex" / "velocity-report.ini"

# geometry is injected by the PyLaTeX Document constructor via geometry_options
# even though it is not explicitly listed in add_packages().
IMPLICIT_PACKAGES: Dict[str, Optional[str]] = {
    "geometry": None,
}

# Packages intentionally loaded at runtime (not dumped into velocity-report.fmt).
RUNTIME_ONLY_PACKAGES = {"fontspec"}

RE_REQUIRE_PACKAGE = re.compile(
    r"""\\RequirePackage(?:\[(?P<options>[^\]]+)\])?\{(?P<name>[^}]+)\}"""
)


def _normalise_options(value: Optional[str]) -> Optional[str]:
    if value is None:
        return None
    parts = [part.strip() for part in value.split(",") if part.strip()]
    return ",".join(parts) if parts else None


def _parse_document_builder_packages(path: Path) -> Dict[str, Optional[str]]:
    tree = ast.parse(path.read_text(encoding="utf-8"))

    for node in tree.body:
        if not isinstance(node, ast.ClassDef) or node.name != "DocumentBuilder":
            continue
        for class_item in node.body:
            if (
                not isinstance(class_item, ast.FunctionDef)
                or class_item.name != "add_packages"
            ):
                continue
            for stmt in class_item.body:
                if not isinstance(stmt, ast.Assign):
                    continue
                if len(stmt.targets) != 1 or not isinstance(stmt.targets[0], ast.Name):
                    continue
                if stmt.targets[0].id != "packages":
                    continue
                if not isinstance(stmt.value, ast.List):
                    continue
                package_map: Dict[str, Optional[str]] = {}
                for element in stmt.value.elts:
                    if not isinstance(element, ast.Tuple) or len(element.elts) != 2:
                        continue
                    pkg_node, opt_node = element.elts
                    if not isinstance(pkg_node, ast.Constant) or not isinstance(
                        pkg_node.value, str
                    ):
                        continue
                    pkg_name = pkg_node.value.strip()
                    option_value: Optional[str]
                    if isinstance(opt_node, ast.Constant):
                        if opt_node.value is None:
                            option_value = None
                        elif isinstance(opt_node.value, str):
                            option_value = opt_node.value.strip()
                        else:
                            option_value = str(opt_node.value)
                    else:
                        continue
                    package_map[pkg_name] = _normalise_options(option_value)
                if package_map:
                    return package_map

    raise RuntimeError(
        f"Could not parse package list in {path}. Expected 'packages = [...]' in add_packages()."
    )


def _parse_format_ini_packages(path: Path) -> Dict[str, Optional[str]]:
    package_map: Dict[str, Optional[str]] = {}
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("%"):
            continue
        match = RE_REQUIRE_PACKAGE.search(line)
        if not match:
            continue
        name = match.group("name").strip()
        options = _normalise_options(match.group("options"))
        package_map[name] = options
    if not package_map:
        raise RuntimeError(f"No \\RequirePackage lines found in {path}.")
    return package_map


def check_parity() -> int:
    builder_packages = _parse_document_builder_packages(DOC_BUILDER_PATH)
    effective_builder_packages = dict(builder_packages)
    effective_builder_packages.update(IMPLICIT_PACKAGES)
    fmt_builder_packages = {
        name: options
        for name, options in effective_builder_packages.items()
        if name not in RUNTIME_ONLY_PACKAGES
    }
    format_packages = _parse_format_ini_packages(FORMAT_INI_PATH)

    builder_set = set(fmt_builder_packages.keys())
    format_set = set(format_packages.keys())

    missing_in_ini = sorted(builder_set - format_set)
    missing_in_builder = sorted(format_set - builder_set)

    option_mismatches = []
    for pkg in sorted(builder_set & format_set):
        builder_opt = _normalise_options(fmt_builder_packages[pkg])
        format_opt = _normalise_options(format_packages[pkg])
        if builder_opt != format_opt:
            option_mismatches.append((pkg, builder_opt, format_opt))

    if missing_in_ini or missing_in_builder or option_mismatches:
        print("LaTeX package parity check failed.")
        print(f"DocumentBuilder packages: {sorted(builder_set)}")
        print(f"velocity-report.ini packages: {sorted(format_set)}")
        if missing_in_ini:
            print(f"Missing in velocity-report.ini: {missing_in_ini}")
        if missing_in_builder:
            print(f"Missing in DocumentBuilder.add_packages(): {missing_in_builder}")
        if option_mismatches:
            print("Option mismatches:")
            for pkg, builder_opt, format_opt in option_mismatches:
                print(
                    f"  - {pkg}: DocumentBuilder={builder_opt!r}, velocity-report.ini={format_opt!r}"
                )
        return 1

    print("LaTeX package parity check passed.")
    print(f"Packages: {sorted(builder_set)}")
    return 0


if __name__ == "__main__":
    sys.exit(check_parity())
