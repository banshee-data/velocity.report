#!/usr/bin/env python3
"""Download PDFs for all entries in references.bib.

Resolves papers via:
  1. arXiv eprint  → https://arxiv.org/pdf/{eprint}.pdf  (direct)
  2. Direct URL    → download as-is
  3. DOI           → Unpaywall API for open-access PDF link
  4. DOI fallback  → Sci-Hub style redirect (disabled by default)

Usage:
    python data/maths/download-papers.py [--bib FILE] [--out DIR] [--report FILE]

Outputs:
    - PDFs saved to data/maths/papers/
    - Status report written to stdout and optionally to --report file
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

# ---------------------------------------------------------------------------
# BibTeX parser (minimal, no external deps)
# ---------------------------------------------------------------------------

_ENTRY_RE = re.compile(
    r"@(\w+)\s*\{([^,]+),\s*\n(.*?)\n\}",
    re.DOTALL,
)
_FIELD_RE = re.compile(
    r"^\s*(\w+)\s*=\s*\{(.*?)\}\s*,?\s*$",
    re.MULTILINE | re.DOTALL,
)


def parse_bibtex(path: Path) -> list[dict[str, Any]]:
    """Return a list of dicts, each with 'key', 'type', and field values."""
    text = path.read_text(encoding="utf-8")
    entries: list[dict[str, Any]] = []
    for m in _ENTRY_RE.finditer(text):
        entry: dict[str, Any] = {
            "type": m.group(1).lower(),
            "key": m.group(2).strip(),
        }
        body = m.group(3)
        for fm in _FIELD_RE.finditer(body):
            field = fm.group(1).lower()
            value = fm.group(2).strip()
            # Collapse internal whitespace
            value = re.sub(r"\s+", " ", value)
            entry[field] = value
        entries.append(entry)
    return entries


# ---------------------------------------------------------------------------
# Download helpers
# ---------------------------------------------------------------------------

HEADERS = {
    "User-Agent": (
        "velocity.report-paper-downloader/1.0 "
        "(https://github.com/banshee-data/velocity.report; "
        "academic research; mailto:dev@velocity.report)"
    ),
}

# Unpaywall requires an email for their API
UNPAYWALL_EMAIL = "dev@velocity.report"


def _download(url: str, dest: Path, *, timeout: int = 30) -> bool:
    """Download url to dest. Return True on success."""
    req = Request(url, headers=HEADERS)
    try:
        with urlopen(req, timeout=timeout) as resp:  # noqa: S310
            content = resp.read()
            if len(content) < 1024:
                # Too small to be a real PDF
                return False
            dest.write_bytes(content)
            return True
    except (HTTPError, URLError, TimeoutError, OSError):
        return False


def try_arxiv(entry: dict[str, Any], out_dir: Path) -> tuple[bool, str]:
    """Try downloading from arXiv using the eprint field."""
    eprint = entry.get("eprint", "")
    if not eprint:
        return False, ""
    url = f"https://arxiv.org/pdf/{eprint}.pdf"
    dest = out_dir / f"{entry['key']}.pdf"
    ok = _download(url, dest, timeout=60)
    return ok, url


def try_direct_url(entry: dict[str, Any], out_dir: Path) -> tuple[bool, str]:
    """Try downloading from a direct URL field ending in .pdf."""
    url = entry.get("url", "")
    if not url or not url.lower().endswith(".pdf"):
        return False, ""
    dest = out_dir / f"{entry['key']}.pdf"
    ok = _download(url, dest, timeout=60)
    return ok, url


def try_unpaywall(entry: dict[str, Any], out_dir: Path) -> tuple[bool, str]:
    """Try Unpaywall API for open-access PDF."""
    doi = entry.get("doi", "")
    if not doi:
        return False, ""
    api_url = f"https://api.unpaywall.org/v2/{doi}?email={UNPAYWALL_EMAIL}"
    req = Request(api_url, headers=HEADERS)
    try:
        with urlopen(req, timeout=15) as resp:  # noqa: S310
            data = json.loads(resp.read())
    except (HTTPError, URLError, TimeoutError, OSError, json.JSONDecodeError):
        return False, ""

    # Look for best open-access location with a PDF URL
    best = data.get("best_oa_location") or {}
    pdf_url = best.get("url_for_pdf", "")
    if not pdf_url:
        # Try any OA location
        for loc in data.get("oa_locations", []):
            pdf_url = loc.get("url_for_pdf", "")
            if pdf_url:
                break
    if not pdf_url:
        return False, ""

    dest = out_dir / f"{entry['key']}.pdf"
    ok = _download(pdf_url, dest, timeout=60)
    return ok, pdf_url


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def download_all(
    bib_path: Path,
    out_dir: Path,
) -> list[dict[str, Any]]:
    """Download all papers. Return list of result dicts."""
    entries = parse_bibtex(bib_path)
    out_dir.mkdir(parents=True, exist_ok=True)

    results: list[dict[str, Any]] = []
    total = len(entries)

    for i, entry in enumerate(entries, 1):
        key = entry["key"]
        title = entry.get("title", "(no title)")
        print(f"[{i}/{total}] {key}: {title[:70]}...")

        result: dict[str, Any] = {
            "key": key,
            "title": title,
            "type": entry.get("type", ""),
            "doi": entry.get("doi", ""),
            "eprint": entry.get("eprint", ""),
            "url": entry.get("url", ""),
            "downloaded": False,
            "source": "",
            "reason": "",
        }

        # Strategy 1: arXiv
        ok, src = try_arxiv(entry, out_dir)
        if ok:
            result["downloaded"] = True
            result["source"] = f"arxiv:{src}"
            print(f"  ✓ Downloaded from arXiv")
            results.append(result)
            time.sleep(1)  # Be polite to arXiv
            continue

        # Strategy 2: Direct URL
        ok, src = try_direct_url(entry, out_dir)
        if ok:
            result["downloaded"] = True
            result["source"] = f"url:{src}"
            print(f"  ✓ Downloaded from direct URL")
            results.append(result)
            time.sleep(0.5)
            continue

        # Strategy 3: Unpaywall
        ok, src = try_unpaywall(entry, out_dir)
        if ok:
            result["downloaded"] = True
            result["source"] = f"unpaywall:{src}"
            print(f"  ✓ Downloaded via Unpaywall")
            results.append(result)
            time.sleep(1)
            continue

        # Could not download
        if not entry.get("doi") and not entry.get("eprint") and not entry.get("url"):
            result["reason"] = "No DOI, eprint, or URL in bibtex entry"
        elif entry.get("doi") and not entry.get("eprint"):
            result["reason"] = "DOI-only; no open-access PDF found via Unpaywall"
        elif entry.get("eprint"):
            result["reason"] = "arXiv download failed"
        else:
            result["reason"] = "Download failed from all sources"

        print(f"  ✗ FAILED: {result['reason']}")
        results.append(result)
        time.sleep(0.5)

    return results


def write_report(results: list[dict[str, Any]], report_path: Path) -> None:
    """Write a markdown status report."""
    downloaded = [r for r in results if r["downloaded"]]
    failed = [r for r in results if not r["downloaded"]]

    lines: list[str] = []
    lines.append("# Paper Download Status Report")
    lines.append("")
    lines.append(f"**Total entries:** {len(results)}")
    lines.append(f"**Downloaded:** {len(downloaded)}")
    lines.append(f"**Failed:** {len(failed)}")
    lines.append("")

    if downloaded:
        lines.append("## Successfully Downloaded")
        lines.append("")
        lines.append("| Key | Title | Source |")
        lines.append("|-----|-------|--------|")
        for r in downloaded:
            title = r["title"][:60] + ("…" if len(r["title"]) > 60 else "")
            lines.append(f"| {r['key']} | {title} | {r['source'][:50]} |")
        lines.append("")

    if failed:
        lines.append("## Failed Downloads")
        lines.append("")
        lines.append("| Key | Title | Reason |")
        lines.append("|-----|-------|--------|")
        for r in failed:
            title = r["title"][:60] + ("…" if len(r["title"]) > 60 else "")
            lines.append(f"| {r['key']} | {title} | {r['reason']} |")
        lines.append("")

    report_path.write_text("\n".join(lines), encoding="utf-8")
    print(f"\nReport written to {report_path}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Download PDFs from references.bib")
    parser.add_argument(
        "--bib",
        type=Path,
        default=Path(__file__).parent / "references.bib",
        help="Path to BibTeX file (default: references.bib alongside this script)",
    )
    parser.add_argument(
        "--out",
        type=Path,
        default=Path(__file__).parent / "papers",
        help="Output directory for PDFs (default: papers/ alongside this script)",
    )
    parser.add_argument(
        "--report",
        type=Path,
        default=Path(__file__).parent / "papers" / "download-status.md",
        help="Path for the status report markdown file",
    )
    args = parser.parse_args()

    if not args.bib.exists():
        print(f"Error: BibTeX file not found: {args.bib}", file=sys.stderr)
        sys.exit(1)

    print(f"BibTeX file: {args.bib}")
    print(f"Output dir:  {args.out}")
    print(f"Report file: {args.report}")
    print()

    results = download_all(args.bib, args.out)
    write_report(results, args.report)

    # Exit with non-zero if any downloads failed
    failed = sum(1 for r in results if not r["downloaded"])
    if failed:
        print(f"\n⚠ {failed} entries could not be downloaded.")
        sys.exit(0)  # Not an error — some papers are paywalled


if __name__ == "__main__":
    main()
