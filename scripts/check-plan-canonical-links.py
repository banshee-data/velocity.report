#!/usr/bin/env python3
"""Check plan-file hygiene: Canonical links, symlink targets, and duplicates.

Validates that every non-symlink plan file under ``docs/plans/`` has a
``- **Canonical:**`` metadata line pointing at a hub doc outside
``docs/plans/``, and that symlinked plan files resolve correctly.

Two modes:

    --report     Advisory output only (exit 0).  For PM review.
    --check      Hard-fail mode (exit 1 on any gate violation).

Default is ``--report`` (advisory).

Gate rules (hard-fail when ``--check``):

  1. Non-symlink plan missing ``- **Canonical:**`` link.
  2. Canonical target points at another file under ``docs/plans/``.
  3. Canonical target points outside the repository or to a missing file.
  4. (Removed — shared targets are advisory only.)
  5. Symlink resolves to another file under ``docs/plans/``.
  6. Symlink resolves outside the repository.
  7. Symlink resolves to a missing target.
  8. More than one ``- **Canonical:**`` line in the same plan header.
  9. Canonical target is not under an allowed hub-doc prefix.

Advisory signals (always printed, never block merges):

  - Hub docs with multiple related plan files.
  - Plan files whose canonical target is outside the expected owning hub.

Usage:
    python3 scripts/check-plan-canonical-links.py                 # advisory
    python3 scripts/check-plan-canonical-links.py --report        # advisory
    python3 scripts/check-plan-canonical-links.py --check         # hard-fail
"""

from __future__ import annotations

import argparse
import os
import sys

# ── Configuration ───────────────────────────────────────────────────────

PLANS_DIR = "docs/plans"

# Allowed hub-doc prefixes (relative to repo root).  A canonical target
# must start with one of these to pass gate 9.
ALLOWED_HUB_PREFIXES = (
    "docs/lidar/",
    "docs/radar/",
    "docs/ui/",
    "docs/platform/",
    "config/",
    "data/",
)

# ── Parsing ─────────────────────────────────────────────────────────────


def _repo_root() -> str:
    """Return the repository root (parent of the scripts/ directory)."""
    return os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def _resolve_canonical_href(plan_path: str, href: str, root: str) -> str:
    """Resolve a possibly-relative Canonical href to a repo-relative path."""
    plan_dir = os.path.dirname(plan_path)
    abs_plan_dir = os.path.join(root, plan_dir)
    abs_target = os.path.normpath(os.path.join(abs_plan_dir, href))
    # Strip fragment anchors (#section) if present.
    abs_target = abs_target.split("#")[0]
    return os.path.relpath(abs_target, root)


def _parse_canonical_links(plan_path: str, root: str) -> list[str]:
    """Extract Canonical targets from the header of a plan file.

    Reads lines until the first ``##`` sub-heading or the end of the
    metadata block (first non-metadata, non-blank line after the title).
    Returns a list of resolved repo-relative target paths.
    """
    abs_path = os.path.join(root, plan_path)
    targets: list[str] = []
    try:
        with open(abs_path, encoding="utf-8") as fh:
            for line in fh:
                stripped = line.strip()
                # Stop at first sub-heading.
                if stripped.startswith("## "):
                    break
                # Match:  - **Canonical:** [text](href)
                # or:     - **Canonical:** href
                if "**Canonical:**" not in stripped:
                    continue
                # Try Markdown link first.
                idx = stripped.index("**Canonical:**") + len("**Canonical:**")
                rest = stripped[idx:].strip()
                if "](" in rest:
                    start = rest.index("](") + 2
                    try:
                        end = rest.index(")", start)
                    except ValueError:
                        href = ""
                    else:
                        href = rest[start:end]
                else:
                    href = rest
                if href:
                    targets.append(_resolve_canonical_href(plan_path, href, root))
    except OSError:
        pass
    return targets


# ── Checks ──────────────────────────────────────────────────────────────


def _collect_plans(root: str) -> tuple[list[str], list[str]]:
    """Return (regular_plans, symlink_plans) as repo-relative paths."""
    plans_abs = os.path.join(root, PLANS_DIR)
    regular: list[str] = []
    symlinks: list[str] = []
    if not os.path.isdir(plans_abs):
        return regular, symlinks
    for name in sorted(os.listdir(plans_abs)):
        if not name.endswith(".md"):
            continue
        rel = os.path.join(PLANS_DIR, name)
        abs_path = os.path.join(root, rel)
        if os.path.islink(abs_path):
            symlinks.append(rel)
        elif os.path.isfile(abs_path):
            regular.append(rel)
    return regular, symlinks


class Checker:
    """Accumulates gate violations and advisory notes."""

    def __init__(self, root: str) -> None:
        self.root = root
        self.gates: list[str] = []
        self.advisories: list[str] = []

    # ── Gate checks ─────────────────────────────────────────────────

    def check_regular_plans(self, plans: list[str]) -> dict[str, list[str]]:
        """Check all non-symlink plan files.  Returns {target: [plans]}."""
        target_to_plans: dict[str, list[str]] = {}

        for plan in plans:
            targets = _parse_canonical_links(plan, self.root)

            # Gate 8: duplicate Canonical line in same file.
            if len(targets) > 1:
                self.gates.append(f"[G8] {plan}: multiple Canonical links found")

            # Gate 1: missing Canonical.
            if not targets:
                self.gates.append(f"[G1] {plan}: missing '- **Canonical:**' metadata")
                continue

            target = targets[0]

            # Gate 2: target under docs/plans/.
            if target.startswith(PLANS_DIR + "/") or target.startswith(
                PLANS_DIR + os.sep
            ):
                self.gates.append(
                    f"[G2] {plan}: Canonical target is under {PLANS_DIR}/ "
                    f"({target})"
                )

            # Gate 3: target missing or outside repo.
            abs_target = os.path.join(self.root, target)
            if target.startswith("..") or not os.path.isfile(abs_target):
                self.gates.append(
                    f"[G3] {plan}: Canonical target missing or outside repo "
                    f"({target})"
                )

            # Gate 9: target not under allowed hub prefix.
            if not any(target.startswith(p) for p in ALLOWED_HUB_PREFIXES):
                self.gates.append(
                    f"[G9] {plan}: Canonical target not under allowed hub "
                    f"prefix ({target})"
                )

            target_to_plans.setdefault(target, []).append(plan)

        # Note: shared targets are reported as advisory only.
        # Multiple plans converging on one canonical doc is expected
        # (e.g. two v050 plans → v050-release-migration.md).

        return target_to_plans

    def check_symlink_plans(self, symlinks: list[str]) -> None:
        """Check all symlinked plan files."""
        for plan in symlinks:
            abs_link = os.path.join(self.root, plan)
            raw_target = os.readlink(abs_link)
            abs_resolved = os.path.normpath(
                os.path.join(os.path.dirname(abs_link), raw_target)
            )
            rel_resolved = os.path.relpath(abs_resolved, self.root)

            # Gate 7: symlink resolves to missing file.
            if not os.path.isfile(abs_resolved):
                self.gates.append(
                    f"[G7] {plan}: symlink target missing ({rel_resolved})"
                )
                continue

            # Gate 6: symlink resolves outside repo.
            if rel_resolved.startswith(".."):
                self.gates.append(
                    f"[G6] {plan}: symlink resolves outside repo " f"({rel_resolved})"
                )
                continue

            # Gate 5: symlink resolves to another docs/plans/ file.
            if rel_resolved.startswith(PLANS_DIR + "/") or rel_resolved.startswith(
                PLANS_DIR + os.sep
            ):
                self.gates.append(
                    f"[G5] {plan}: symlink resolves to {PLANS_DIR}/ "
                    f"({rel_resolved})"
                )

    # ── Advisory checks ─────────────────────────────────────────────

    def advisory_multi_plan_targets(
        self, target_to_plans: dict[str, list[str]]
    ) -> None:
        """Report hub docs with more than one related plan file."""
        for target, owners in sorted(target_to_plans.items()):
            if len(owners) > 1:
                self.advisories.append(
                    f"[advisory] Hub doc {target} has {len(owners)} active "
                    f"plans: {', '.join(owners)}"
                )

    # ── Output ──────────────────────────────────────────────────────

    def print_report(self) -> None:
        """Print all findings."""
        if self.gates:
            print("\n=== Gate violations ===\n")
            for msg in self.gates:
                print(f"  {msg}")

        if self.advisories:
            print("\n=== Advisory ===\n")
            for msg in self.advisories:
                print(f"  {msg}")

        if not self.gates and not self.advisories:
            print("Plan hygiene: all checks passed.")
        else:
            print(
                f"\nSummary: {len(self.gates)} gate violation(s), "
                f"{len(self.advisories)} advisory note(s)."
            )


# ── Main ────────────────────────────────────────────────────────────────


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Check plan-file canonical-link hygiene."
    )
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument(
        "--report",
        action="store_true",
        default=True,
        help="Advisory mode (default). Always exits 0.",
    )
    mode.add_argument(
        "--check",
        action="store_true",
        help="Hard-fail mode. Exits 1 if any gate violation is found.",
    )
    args = parser.parse_args()

    root = _repo_root()
    checker = Checker(root)

    regular, symlinks = _collect_plans(root)
    target_to_plans = checker.check_regular_plans(regular)
    checker.check_symlink_plans(symlinks)
    checker.advisory_multi_plan_targets(target_to_plans)

    checker.print_report()

    if args.check and checker.gates:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
