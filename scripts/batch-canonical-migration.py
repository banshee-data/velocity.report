#!/usr/bin/env python3
"""Batch Phase 2 migration: create canonical doc stubs and add Canonical metadata.

This script is single-use tooling for the platform-hub-restructure-plan.
It reads the hard-coded mapping, creates stub canonical docs where they
do not already exist, and inserts '- **Canonical:**' lines into plan
files that lack them.

Run from repo root:
    python3 scripts/batch-canonical-migration.py
"""

import os
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent

# ── Mapping: plan filename (without .md) → canonical doc path (relative to repo root) ──

MAPPING = {
    # Already done — included for completeness / verification
    "data-database-alignment-plan": "docs/platform/architecture/database-sql-boundary.md",
    "data-sqlite-client-standardisation-plan": "docs/platform/architecture/database-sql-boundary.md",
    "data-track-description-language-plan": "docs/platform/architecture/track-description-language.md",
    "platform-hub-restructure-plan": "docs/platform/architecture/canonical-plan-graduation.md",
    # ── docs/lidar/architecture/ ──
    "lidar-layer-dependency-hygiene-plan": "docs/lidar/architecture/lidar-data-layer-model.md",
    "lidar-architecture-graph-plan": "docs/lidar/architecture/lidar-pipeline-reference.md",
    "lidar-bodies-in-motion-plan": "docs/lidar/architecture/foreground-tracking.md",
    "lidar-l7-scene-plan": "docs/lidar/architecture/vector-scene-map.md",
    "lidar-ml-classifier-training-plan": "docs/lidar/architecture/ml-solver-expansion.md",
    "lidar-av-lidar-integration-plan": "docs/lidar/architecture/av-range-image-format-alignment.md",
    "lidar-l8-analytics-l9-endpoints-l10-clients-plan": "docs/lidar/architecture/l8-l9-l10-migration-notes.md",
    "lidar-l2-dual-representation-plan": "docs/lidar/architecture/l2-dual-representation.md",
    "lidar-velocity-coherent-foreground-extraction-plan": "docs/lidar/architecture/velocity-foreground-extraction.md",
    "lidar-architecture-dynamic-algorithm-selection-plan": "docs/lidar/architecture/pluggable-algorithm-selection.md",
    "lidar-distributed-sweep-workers-plan": "docs/lidar/architecture/distributed-sweep.md",
    "lidar-tracks-table-consolidation-plan": "docs/lidar/architecture/track-storage-consolidation.md",
    "label-vocabulary-consolidation-plan": "docs/lidar/architecture/label-vocabulary.md",
    # ── docs/lidar/operations/ ──
    "lidar-parameter-tuning-optimisation-plan": "docs/lidar/operations/auto-tuning.md",
    "lidar-sweep-hint-mode-plan": "docs/lidar/operations/hint-sweep-mode.md",
    "lidar-track-labelling-auto-aware-tuning-plan": "docs/lidar/operations/track-labelling-auto-aware-tuning.md",
    "lidar-performance-measurement-harness-plan": "docs/lidar/operations/performance-regression-testing.md",
    "lidar-analysis-run-infrastructure-plan": "docs/lidar/operations/pcap-analysis-mode.md",
    "lidar-architecture-foundations-fixit-plan": "docs/lidar/operations/foundations-fixit-progress.md",
    "lidar-immutable-run-config-asset-plan": "docs/lidar/operations/immutable-run-config.md",
    "lidar-clustering-observability-and-benchmark-plan": "docs/lidar/operations/clustering-diagnostics.md",
    "lidar-test-corpus-plan": "docs/lidar/operations/test-corpus.md",
    "hint-metric-observability-plan": "docs/lidar/operations/observability-surfaces.md",
    "lidar-static-pose-alignment-plan": "docs/lidar/operations/static-pose-alignment.md",
    "lidar-motion-capture-architecture-plan": "docs/lidar/operations/motion-capture.md",
    "unpopulated-data-structures-remediation-plan": "docs/lidar/operations/data-completeness-remediation.md",
    # ── docs/lidar/operations/visualiser/ ──
    "lidar-visualiser-labelling-qc-enhancements-overview-plan": "docs/lidar/operations/visualiser/qc-enhancements-overview.md",
    "lidar-visualiser-track-quality-score-plan": "docs/lidar/operations/visualiser/track-quality-scoring.md",
    "lidar-visualiser-track-event-timeline-bar-plan": "docs/lidar/operations/visualiser/track-event-timeline.md",
    "lidar-visualiser-split-merge-repair-workbench-plan": "docs/lidar/operations/visualiser/split-merge-repair.md",
    "lidar-visualiser-physics-checks-and-confirmation-gates-plan": "docs/lidar/operations/visualiser/physics-checks.md",
    "lidar-visualiser-trails-and-uncertainty-visualisation-plan": "docs/lidar/operations/visualiser/trails-and-uncertainty.md",
    "lidar-visualiser-priority-review-queue-plan": "docs/lidar/operations/visualiser/priority-review-queue.md",
    "lidar-visualiser-qc-dashboard-and-audit-export-plan": "docs/lidar/operations/visualiser/qc-dashboard-and-audit.md",
    "lidar-visualiser-run-list-labelling-rollup-icon-plan": "docs/lidar/operations/visualiser/run-list-labelling-rollup.md",
    "lidar-visualiser-light-mode-plan": "docs/lidar/operations/visualiser/light-mode.md",
    "lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan": "docs/lidar/operations/visualiser/proto-contract.md",
    "lidar-visualiser-performance-and-scene-health-timeline-metrics-plan": "docs/lidar/operations/visualiser/performance-and-timeline-metrics.md",
    # ── docs/radar/architecture/ ──
    "speed-percentile-aggregation-alignment-plan": "docs/radar/architecture/percentile-aggregation-semantics.md",
    # ── docs/ui/ ──
    "server-manager": "docs/ui/velocity-visualiser-architecture.md",
    "web-frontend-background-debug-surfaces-plan": "docs/ui/velocity-visualiser-implementation.md",
    "web-frontend-consolidation-plan": "docs/ui/web-frontend-consolidation.md",
    "homepage-responsive-gif-strategies": "docs/ui/homepage.md",
    "wireshark-menu-alignment": "docs/ui/macos-menu-layout-design.md",
    # ── docs/platform/architecture/ ──
    "go-codebase-structural-hygiene-plan": "docs/platform/architecture/go-package-structure.md",
    "go-god-file-split-plan": "docs/platform/architecture/go-package-structure.md",
    "go-structured-logging-plan": "docs/platform/architecture/structured-logging.md",
    "platform-typed-uuid-prefixes-plan": "docs/platform/architecture/typed-uuid-prefixes.md",
    "tictactail-platform-plan": "docs/platform/architecture/tictactail-library.md",
    "metrics-registry-and-observability-plan": "docs/platform/architecture/metrics-registry.md",
    "platform-canonical-project-files-plan": "docs/platform/architecture/canonical-plan-graduation.md",
    # ── docs/platform/operations/ ──
    "deploy-distribution-packaging-plan": "docs/platform/operations/distribution-packaging.md",
    "deploy-rpi-imager-fork-plan": "docs/platform/operations/rpi-imager.md",
    "schema-simplification-migration-030-plan": "docs/platform/operations/schema-migration-030.md",
    "v050-backward-compatibility-shim-removal-plan": "docs/platform/operations/v050-release-migration.md",
    "v050-tech-debt-removal-plan": "docs/platform/operations/v050-release-migration.md",
    "platform-documentation-standardisation-plan": "docs/platform/operations/documentation-standards.md",
    "line-width-standardisation-plan": "docs/platform/operations/documentation-standards.md",
    "platform-quality-coverage-improvement-plan": "docs/platform/operations/quality-coverage.md",
    "platform-data-science-metrics-first-plan": "docs/platform/operations/data-science-methodology.md",
    "tooling-python-venv-consolidation-plan": "docs/platform/operations/python-venv.md",
    "pdf-go-chart-migration-plan": "docs/platform/operations/pdf-reporting.md",
    "pdf-latex-precompiled-format-plan": "docs/platform/operations/pdf-reporting.md",
    "agent-claude-preparedness-review-plan": "docs/platform/operations/agent-preparedness.md",
    "platform-simplification-and-deprecation-plan": "docs/platform/operations/simplification-deprecation.md",
}

# ── Human-readable titles for NEW stub files ──

STUB_TITLES = {
    "docs/lidar/architecture/l2-dual-representation.md": "L2 Dual Representation (Polar and Cartesian)",
    "docs/lidar/architecture/velocity-foreground-extraction.md": "Velocity-Coherent Foreground Extraction",
    "docs/lidar/architecture/pluggable-algorithm-selection.md": "Pluggable Algorithm Selection",
    "docs/lidar/architecture/distributed-sweep.md": "Distributed Sweep Workers",
    "docs/lidar/architecture/track-storage-consolidation.md": "Track Storage Consolidation",
    "docs/lidar/architecture/label-vocabulary.md": "Label Vocabulary",
    "docs/lidar/operations/immutable-run-config.md": "Immutable Run Configuration",
    "docs/lidar/operations/clustering-diagnostics.md": "Clustering Diagnostics",
    "docs/lidar/operations/test-corpus.md": "Test Corpus",
    "docs/lidar/operations/observability-surfaces.md": "Observability Surfaces",
    "docs/lidar/operations/static-pose-alignment.md": "Static Pose Alignment",
    "docs/lidar/operations/motion-capture.md": "Motion Capture Architecture",
    "docs/lidar/operations/data-completeness-remediation.md": "Data Completeness Remediation",
    "docs/lidar/operations/visualiser/qc-enhancements-overview.md": "QC Enhancements Overview",
    "docs/lidar/operations/visualiser/track-quality-scoring.md": "Track Quality Scoring",
    "docs/lidar/operations/visualiser/track-event-timeline.md": "Track Event Timeline",
    "docs/lidar/operations/visualiser/split-merge-repair.md": "Split/Merge Repair Workbench",
    "docs/lidar/operations/visualiser/physics-checks.md": "Physics Checks and Confirmation Gates",
    "docs/lidar/operations/visualiser/trails-and-uncertainty.md": "Trails and Uncertainty Visualisation",
    "docs/lidar/operations/visualiser/priority-review-queue.md": "Priority Review Queue",
    "docs/lidar/operations/visualiser/qc-dashboard-and-audit.md": "QC Dashboard and Audit Export",
    "docs/lidar/operations/visualiser/run-list-labelling-rollup.md": "Run List Labelling Rollup",
    "docs/lidar/operations/visualiser/light-mode.md": "Light Mode",
    "docs/lidar/operations/visualiser/proto-contract.md": "Proto Contract and Debug Overlays",
    "docs/lidar/operations/visualiser/performance-and-timeline-metrics.md": "Performance and Timeline Metrics",
    "docs/radar/architecture/percentile-aggregation-semantics.md": "Percentile Aggregation Semantics",
    "docs/ui/web-frontend-consolidation.md": "Web Frontend Consolidation",
    "docs/ui/homepage.md": "Homepage",
    "docs/ui/macos-menu-layout-design.md": "macOS Menu Layout Design",
    "docs/platform/architecture/go-package-structure.md": "Go Package Structure",
    "docs/platform/architecture/structured-logging.md": "Structured Logging",
    "docs/platform/architecture/typed-uuid-prefixes.md": "Typed UUID Prefixes",
    "docs/platform/architecture/tictactail-library.md": "TicTacTail Library",
    "docs/platform/architecture/metrics-registry.md": "Metrics Registry",
    "docs/platform/architecture/canonical-plan-graduation.md": "Canonical Plan Graduation",
    "docs/platform/operations/distribution-packaging.md": "Distribution Packaging",
    "docs/platform/operations/rpi-imager.md": "Raspberry Pi Imager",
    "docs/platform/operations/schema-migration-030.md": "Schema Migration 030",
    "docs/platform/operations/v050-release-migration.md": "v0.5.0 Release Migration",
    "docs/platform/operations/documentation-standards.md": "Documentation Standards",
    "docs/platform/operations/quality-coverage.md": "Quality and Coverage",
    "docs/platform/operations/data-science-methodology.md": "Data Science Methodology",
    "docs/platform/operations/python-venv.md": "Python Virtual Environment",
    "docs/platform/operations/pdf-reporting.md": "PDF Reporting",
    "docs/platform/operations/agent-preparedness.md": "Agent Preparedness",
    "docs/platform/operations/simplification-deprecation.md": "Simplification and Deprecation",
}


def create_stubs():
    """Create canonical doc stubs for files that do not yet exist."""
    targets = set(MAPPING.values())
    created = 0
    for target in sorted(targets):
        full = REPO / target
        if full.exists():
            continue
        if target not in STUB_TITLES:
            print(f"  SKIP (no title): {target}")
            continue
        full.parent.mkdir(parents=True, exist_ok=True)
        title = STUB_TITLES[target]
        full.write_text(
            f"# {title}\n\nStub — content to be consolidated from plan files.\n"
        )
        created += 1
        print(f"  CREATED: {target}")
    print(f"\n  Total stubs created: {created}")


def add_canonical_links():
    """Insert Canonical metadata into plan files that lack it."""
    updated = 0
    skipped = 0
    for plan_name, target in sorted(MAPPING.items()):
        plan_path = REPO / "docs" / "plans" / f"{plan_name}.md"
        if not plan_path.exists():
            print(f"  MISSING: {plan_path.name}")
            continue

        text = plan_path.read_text()

        # Already has Canonical?
        if re.search(r"^\- \*\*Canonical:\*\*", text, re.MULTILINE):
            skipped += 1
            continue

        # Build the relative link from docs/plans/ to the target
        plan_dir = Path("docs/plans")
        rel = os.path.relpath(target, plan_dir)
        display = os.path.basename(target)
        canonical_line = f"- **Canonical:** [{display}]({rel})"

        # Insert after the title line (first # heading) or after existing
        # metadata lines (- **Key:** value).
        lines = text.split("\n")
        insert_idx = None

        # Find the end of the header metadata block:
        # title line, then optional blank, then metadata lines (- **...**)
        i = 0
        # Skip title
        while i < len(lines) and not lines[i].startswith("# "):
            i += 1
        if i < len(lines):
            i += 1  # past the title line

        # Skip blank lines after title
        while i < len(lines) and lines[i].strip() == "":
            i += 1

        # Now we should be in metadata lines (- **Status:**, - **Layers:**, etc.)
        # Find the last metadata line in this block
        last_meta = None
        while i < len(lines):
            stripped = lines[i].strip()
            if stripped.startswith("- **"):
                last_meta = i
                i += 1
                # Handle multi-line metadata (continuation lines starting with spaces)
                while (
                    i < len(lines)
                    and lines[i].strip()
                    and not lines[i].strip().startswith("- **")
                    and not lines[i].strip().startswith("#")
                    and not lines[i].strip().startswith("```")
                ):
                    if lines[i].startswith("  "):
                        last_meta = i
                        i += 1
                    else:
                        break
            else:
                break

        if last_meta is not None:
            insert_idx = last_meta + 1
        else:
            # No metadata found — insert after title + blank line
            insert_idx = min(i, len(lines))

        lines.insert(insert_idx, canonical_line)
        plan_path.write_text("\n".join(lines))
        updated += 1
        print(f"  UPDATED: {plan_name}.md → {target}")

    print(f"\n  Updated: {updated}, Skipped (already done): {skipped}")


def main():
    print("=== Creating canonical doc stubs ===\n")
    create_stubs()
    print("\n=== Adding Canonical metadata to plans ===\n")
    add_canonical_links()
    print("\n=== Done ===")
    return 0


if __name__ == "__main__":
    sys.exit(main())
