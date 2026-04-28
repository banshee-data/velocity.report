#!/usr/bin/env python3
"""Compare two generated report PDFs with Poppler tools.

The script is intentionally non-mutating for inputs. It emits:
  - summary.json with page size/page count and generated artifact paths
  - pdfinfo, pdffonts, pdfimages output for each PDF
  - pdftotext bbox-layout XHTML for each PDF
  - 144 DPI page PNGs for visual side-by-side review
"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
from pathlib import Path


def run(cmd: list[str], output: Path | None = None) -> str:
    result = subprocess.run(cmd, check=True, text=True, capture_output=True)
    if output is not None:
        output.write_text(result.stdout, encoding="utf-8")
    return result.stdout


def parse_pdfinfo(text: str) -> dict[str, object]:
    page_size = None
    pages = None
    for line in text.splitlines():
        if line.startswith("Pages:"):
            pages = int(line.split(":", 1)[1].strip())
        elif line.startswith("Page size:"):
            page_size = line.split(":", 1)[1].strip()
    return {"pages": pages, "page_size": page_size}


def png_dimensions(path: Path) -> tuple[int, int] | None:
    if shutil.which("identify") is None:
        return None
    out = run(["identify", "-format", "%w %h", str(path)])
    match = re.fullmatch(r"(\d+) (\d+)", out.strip())
    if match is None:
        return None
    return int(match.group(1)), int(match.group(2))


def inspect_pdf(label: str, pdf: Path, out_dir: Path) -> dict[str, object]:
    pdf_dir = out_dir / label
    pdf_dir.mkdir(parents=True, exist_ok=True)

    info_text = run(["pdfinfo", str(pdf)], pdf_dir / "pdfinfo.txt")
    run(["pdffonts", str(pdf)], pdf_dir / "pdffonts.txt")
    run(["pdfimages", "-list", str(pdf)], pdf_dir / "pdfimages.txt")
    run(["pdftotext", "-bbox-layout", str(pdf), str(pdf_dir / "bbox.xhtml")])

    png_prefix = pdf_dir / "page"
    run(["pdftoppm", "-png", "-r", "144", str(pdf), str(png_prefix)])
    pngs = sorted(pdf_dir.glob("page-*.png"))
    rendered_pages = []
    for png in pngs:
        rendered_pages.append(
            {
                "path": str(png),
                "dimensions": png_dimensions(png),
            }
        )

    summary = parse_pdfinfo(info_text)
    summary["pdf"] = str(pdf)
    summary["artifacts"] = {
        "pdfinfo": str(pdf_dir / "pdfinfo.txt"),
        "pdffonts": str(pdf_dir / "pdffonts.txt"),
        "pdfimages": str(pdf_dir / "pdfimages.txt"),
        "bbox": str(pdf_dir / "bbox.xhtml"),
        "pages": rendered_pages,
    }
    return summary


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("old_pdf", type=Path)
    parser.add_argument("new_pdf", type=Path)
    parser.add_argument(
        "--out-dir",
        type=Path,
        default=Path("/tmp/velocity-report-layout-compare"),
        help="Directory for comparison artifacts.",
    )
    args = parser.parse_args()

    args.out_dir.mkdir(parents=True, exist_ok=True)
    summary = {
        "old": inspect_pdf("old", args.old_pdf.resolve(), args.out_dir),
        "new": inspect_pdf("new", args.new_pdf.resolve(), args.out_dir),
    }
    (args.out_dir / "summary.json").write_text(
        json.dumps(summary, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(args.out_dir / "summary.json")


if __name__ == "__main__":
    main()
