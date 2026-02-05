#!/usr/bin/env python3
"""Generate a PDF/TEX from a JSON config using dummy metrics.

This script loads a config JSON (default: ../config.with-site-info.json),
builds minimal overall metrics, and calls the CLI assemble routine to produce
the report files. It's meant to help debug whether `site.speed_limit_note`
is propagated into the generated .tex.
"""

import os
import sys
from pathlib import Path


def main(config_path: str):
    # Ensure the pdf_generator package from tools/pdf-generator/pdf_generator is
    # importable. We modify sys.path at runtime before importing the local
    # package modules to avoid placing module-level code between top-level
    # imports (ruff E402).
    # Set ROOT to the tools/pdf-generator directory so ROOT / 'pdf_generator'
    # resolves to tools/pdf-generator/pdf_generator where the package lives.
    ROOT = Path(__file__).resolve().parents[1]
    # Add tools/pdf-generator (parent of the pdf_generator package directory)
    # to sys.path so `import pdf_generator` works.
    sys.path.insert(0, str(ROOT))

    from pdf_generator.cli.main import assemble_pdf_report
    from pdf_generator.core.config_manager import load_config

    cfg = load_config(config_file=config_path)

    # Build simple ISO strings from query dates
    start_iso = cfg.query.start_date + "T00:00:00"
    end_iso = cfg.query.end_date + "T23:59:59"

    # Minimal overall metrics with expected fields
    overall_metrics = [
        {
            "p50": 30.54,
            "p85": 36.94,
            "p98": 43.05,
            "max_speed": 53.52,
            "Count": 123,
        }
    ]

    prefix = cfg.output.file_prefix or "debug-report"
    # Ensure outputs go to a tmp folder to avoid clobbering real files
    out_dir = Path("tmp-debug-output")
    out_dir.mkdir(exist_ok=True)
    prefix = str(out_dir / prefix)

    print(f"Using config: {config_path}")
    print(f"Site.speed_limit_note: {cfg.site.speed_limit_note!r}")

    assembled = assemble_pdf_report(
        prefix,
        start_iso,
        end_iso,
        overall_metrics,
        daily_metrics=None,
        granular_metrics=[],
        histogram=None,
        config=cfg,
    )

    print(f"assemble_pdf_report returned: {assembled}")
    tex_path = Path(prefix + "_report.tex")
    pdf_path = Path(prefix + "_report.pdf")
    print(f"Expecting TEX at: {tex_path}")
    print(f"Expecting PDF at: {pdf_path}")

    if tex_path.exists():
        print("--- BEGIN .tex preview ---")
        print(tex_path.read_text(errors="ignore")[:4000])
        print("--- END .tex preview ---")
    else:
        print(".tex not found; list tmp-debug-output:")
        for p in sorted(out_dir.iterdir()):
            print(" -", p)


if __name__ == "__main__":
    cfg = os.path.join(os.path.dirname(__file__), "..", "config.with-site-info.json")
    cfg = os.path.normpath(cfg)
    if not os.path.exists(cfg):
        print(f"Config file not found: {cfg}")
        sys.exit(2)
    main(cfg)
