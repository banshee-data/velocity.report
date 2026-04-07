#!/usr/bin/env python3
"""Download PDFs for all entries in data/maths/references.bib.

Resolves papers via:
  1. arXiv eprint  → https://arxiv.org/pdf/{eprint}.pdf  (direct)
  2. Direct URL    → url field or howpublished URL ending in .pdf
  3. DOI           → Unpaywall API for open-access PDF link

Usage:
    python scripts/download-papers.py [--bib FILE] [--out DIR] [--report FILE]

Outputs:
    - PDFs saved to data/maths/papers/
    - Status report written to data/maths/papers/download-status.md
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import tempfile
import time
from collections import Counter
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import quote
from urllib.request import Request, urlopen

# ---------------------------------------------------------------------------
# BibTeX parser (minimal, no external deps)
# ---------------------------------------------------------------------------
# Uses a brace-balanced parser rather than regex so it handles:
#   - entries whose closing } is indented (common in references.bib)
#   - nested braces in field values (e.g., {{DBSCAN}}, title={{Long Title}})
# ---------------------------------------------------------------------------


def _skip_whitespace(text: str, index: int) -> int:
    while index < len(text) and text[index].isspace():
        index += 1
    return index


def _read_balanced_braces(text: str, index: int) -> tuple[str, int]:
    """Read a BibTeX braced value starting at ``index``."""
    if index >= len(text) or text[index] != "{":
        raise ValueError("expected '{' at start of braced value")

    depth = 0
    start = index + 1
    i = index
    while i < len(text):
        char = text[i]
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return text[start:i], i + 1
        i += 1

    raise ValueError("unterminated braced value in BibTeX")


def _read_quoted_value(text: str, index: int) -> tuple[str, int]:
    """Read a BibTeX quoted value starting at ``index``."""
    if index >= len(text) or text[index] != '"':
        raise ValueError("expected '\"' at start of quoted value")

    chars: list[str] = []
    i = index + 1
    while i < len(text):
        char = text[i]
        if char == "\\" and i + 1 < len(text):
            chars.append(text[i : i + 2])
            i += 2
            continue
        if char == '"':
            return "".join(chars), i + 1
        chars.append(char)
        i += 1

    raise ValueError("unterminated quoted value in BibTeX")


def _read_simple_value(text: str, index: int) -> tuple[str, int]:
    """Read a simple BibTeX value until the next comma or line ending."""
    start = index
    while index < len(text) and text[index] not in ",\r\n":
        index += 1
    return text[start:index].strip(), index


def _parse_fields(body: str) -> dict[str, str]:
    fields: dict[str, str] = {}
    index = 0
    length = len(body)

    while index < length:
        while index < length and (body[index].isspace() or body[index] == ","):
            index += 1
        if index >= length:
            break

        field_start = index
        while index < length and (body[index].isalnum() or body[index] in "_-"):
            index += 1
        if field_start == index:
            index += 1
            continue

        field = body[field_start:index].lower()
        index = _skip_whitespace(body, index)
        if index >= length or body[index] != "=":
            continue

        index += 1
        index = _skip_whitespace(body, index)
        if index >= length:
            break

        if body[index] == "{":
            value, index = _read_balanced_braces(body, index)
        elif body[index] == '"':
            value, index = _read_quoted_value(body, index)
        else:
            value, index = _read_simple_value(body, index)

        fields[field] = re.sub(r"\s+", " ", value.strip())

        while index < length and body[index].isspace():
            index += 1
        if index < length and body[index] == ",":
            index += 1

    return fields


def parse_bibtex(path: Path) -> list[dict[str, Any]]:
    """Return a list of dicts, each with 'key', 'type', and field values."""
    text = path.read_text(encoding="utf-8")
    entries: list[dict[str, Any]] = []
    index = 0
    length = len(text)

    while index < length:
        at_index = text.find("@", index)
        if at_index == -1:
            break

        type_start = at_index + 1
        type_end = type_start
        while type_end < length and (text[type_end].isalnum() or text[type_end] == "_"):
            type_end += 1
        if type_end == type_start:
            index = at_index + 1
            continue

        entry_type = text[type_start:type_end].lower()
        cursor = _skip_whitespace(text, type_end)
        if cursor >= length or text[cursor] != "{":
            index = at_index + 1
            continue

        try:
            entry_content, entry_end = _read_balanced_braces(text, cursor)
        except ValueError:
            index = at_index + 1
            continue

        comma_index = entry_content.find(",")
        if comma_index == -1:
            index = entry_end
            continue

        key = entry_content[:comma_index].strip()
        body = entry_content[comma_index + 1 :]

        entry: dict[str, Any] = {
            "type": entry_type,
            "key": key,
        }
        entry.update(_parse_fields(body))
        entries.append(entry)
        index = entry_end

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

# A valid PDF is at least this many bytes (header + minimal structure)
_MIN_PDF_BYTES = 1024

# Maximum PDF download size (50 MB — academic papers are never larger)
_MAX_PDF_BYTES = 50 * 1024 * 1024

# Allowed URL schemes to prevent SSRF via file:// or other protocols
_ALLOWED_SCHEMES = ("https://", "http://")


def _download(url: str, dest: Path, *, timeout: int = 30) -> bool:
    """Download url to dest atomically.

    Streams to a temporary file in 64 KB chunks, validates the PDF magic
    bytes in the first chunk, enforces a size cap, then renames into place.
    Returns True only if a valid PDF was saved.
    """
    if not any(url.startswith(s) for s in _ALLOWED_SCHEMES):
        return False
    req = Request(url, headers=HEADERS)
    tmp_path: Path | None = None
    try:
        with urlopen(req, timeout=timeout) as resp:  # noqa: S310
            content_type = resp.headers.get("Content-Type", "")
            # Reject clearly non-PDF responses early (allow octet-stream as
            # some servers serve PDFs without a specific content type)
            if content_type and not any(
                t in content_type.lower() for t in ("pdf", "octet-stream")
            ):
                return False

            dest.parent.mkdir(parents=True, exist_ok=True)
            fd, tmp_str = tempfile.mkstemp(dir=dest.parent, suffix=".tmp")
            tmp_path = Path(tmp_str)
            total = 0
            first_chunk = True
            with os.fdopen(fd, "wb") as fh:
                while True:
                    chunk = resp.read(65536)  # 64 KB chunks
                    if not chunk:
                        break
                    if first_chunk:
                        # Validate PDF magic bytes before writing anything
                        if len(chunk) < 5 or chunk[:5] != b"%PDF-":
                            return False
                        first_chunk = False
                    total += len(chunk)
                    if total > _MAX_PDF_BYTES:
                        return False
                    fh.write(chunk)

        if total < _MIN_PDF_BYTES:
            return False

        tmp_path.replace(dest)  # atomic rename
        tmp_path = None  # ownership transferred to dest
        return True
    except (HTTPError, URLError, TimeoutError, OSError):
        return False
    finally:
        if tmp_path is not None:
            tmp_path.unlink(missing_ok=True)


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
    """Try downloading from a direct URL field ending in .pdf.

    Checks both 'url' and 'howpublished' fields for PDF links.
    """
    url = entry.get("url", "")
    if not url:
        # Extract URL from howpublished field: \url{...}
        hp = entry.get("howpublished", "")
        m = re.search(r"\\url\{([^}]+)\}", hp)
        if m:
            url = m.group(1)
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
    # URL-encode the DOI path segment and email — some DOIs contain characters
    # such as parentheses (e.g., 10.1016/S1474-6670(17)31906-7) that must be
    # percent-encoded to form a valid URL.
    api_url = f"https://api.unpaywall.org/v2/{quote(doi, safe='')}?email={quote(UNPAYWALL_EMAIL, safe='')}"
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


def _classify_entry(entry: dict[str, Any]) -> tuple[str, str]:
    """Classify an entry's best download strategy and expected URL.

    Returns (strategy, url) where strategy is one of:
      arxiv, direct_url, unpaywall, none
    """
    eprint = entry.get("eprint", "")
    if eprint:
        return "arxiv", f"https://arxiv.org/pdf/{eprint}.pdf"

    url = entry.get("url", "")
    if not url:
        hp = entry.get("howpublished", "")
        m = re.search(r"\\url\{([^}]+)\}", hp)
        if m:
            url = m.group(1)
    if url and url.lower().endswith(".pdf"):
        return "direct_url", url

    doi = entry.get("doi", "")
    if doi:
        return "unpaywall", f"https://doi.org/{doi}"

    return "none", ""


def download_all(
    bib_path: Path,
    out_dir: Path,
    *,
    dry_run: bool = False,
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

        strategy, expected_url = _classify_entry(entry)

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
            "strategy": strategy,
            "expected_url": expected_url,
        }

        if dry_run:
            if strategy == "none":
                result["reason"] = "No DOI, eprint, or URL in bibtex entry"
                print("  ⊘ DRY RUN: no download path available")
            else:
                # Use a separate flag so the report accurately reflects that
                # nothing was actually downloaded in dry-run mode.
                result["would_download"] = True
                result["source"] = f"{strategy}:{expected_url}"
                print(f"  ⊘ DRY RUN: would try {strategy} → {expected_url}")
            results.append(result)
            continue

        # Strategy 1: arXiv
        ok, src = try_arxiv(entry, out_dir)
        if ok:
            result["downloaded"] = True
            result["source"] = f"arxiv:{src}"
            print("  ✓ Downloaded from arXiv")
            results.append(result)
            time.sleep(1)  # Be polite to arXiv
            continue

        # Strategy 2: Direct URL
        ok, src = try_direct_url(entry, out_dir)
        if ok:
            result["downloaded"] = True
            result["source"] = f"url:{src}"
            print("  ✓ Downloaded from direct URL")
            results.append(result)
            time.sleep(0.5)
            continue

        # Strategy 3: Unpaywall
        ok, src = try_unpaywall(entry, out_dir)
        if ok:
            result["downloaded"] = True
            result["source"] = f"unpaywall:{src}"
            print("  ✓ Downloaded via Unpaywall")
            results.append(result)
            time.sleep(1)
            continue

        # Could not download
        if strategy == "none":
            result["reason"] = "No DOI, eprint, or URL in bibtex entry"
        elif strategy == "arxiv":
            result["reason"] = "arXiv download failed"
        elif strategy == "unpaywall":
            result["reason"] = "DOI-only; no open-access PDF found via Unpaywall"
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
    would_download = [r for r in results if r.get("would_download")]

    lines.append(f"**Total entries:** {len(results)}")
    lines.append(f"**Downloaded:** {len(downloaded)}")
    if would_download:
        lines.append(f"**Would download (dry run):** {len(would_download)}")
    lines.append(f"**Failed:** {len(failed)}")
    lines.append("")

    # Strategy breakdown
    strats = Counter(r.get("strategy", "unknown") for r in results)
    lines.append("### Download Strategy Breakdown")
    lines.append("")
    for s, c in sorted(strats.items()):
        lines.append(f"- **{s}:** {c} entries")
    lines.append("")

    if downloaded:
        lines.append("## Successfully Downloaded")
        lines.append("")
        lines.append("| Key | Title | Source |")
        lines.append("|-----|-------|--------|")
        for r in downloaded:
            title = r["title"][:60] + ("…" if len(r["title"]) > 60 else "")
            src = r["source"][:60]
            lines.append(f"| {r['key']} | {title} | {src} |")
        lines.append("")

    if would_download:
        lines.append("## Would Download (dry run)")
        lines.append("")
        lines.append("| Key | Title | Planned source |")
        lines.append("|-----|-------|----------------|")
        for r in would_download:
            title = r["title"][:60] + ("…" if len(r["title"]) > 60 else "")
            src = r["source"][:60]
            lines.append(f"| {r['key']} | {title} | {src} |")
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


def _repo_root() -> Path:
    """Return the repository root (parent of scripts/)."""
    return Path(__file__).resolve().parent.parent


def main() -> None:
    root = _repo_root()
    parser = argparse.ArgumentParser(description="Download PDFs from references.bib")
    parser.add_argument(
        "--bib",
        type=Path,
        default=root / "data" / "maths" / "references.bib",
        help="Path to BibTeX file (default: data/maths/references.bib)",
    )
    parser.add_argument(
        "--out",
        type=Path,
        default=root / "data" / "maths" / "papers",
        help="Output directory for PDFs (default: data/maths/papers/)",
    )
    parser.add_argument(
        "--report",
        type=Path,
        default=root / "data" / "maths" / "papers" / "download-status.md",
        help="Path for the status report markdown file",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Classify entries and report expected sources without downloading",
    )
    args = parser.parse_args()

    if not args.bib.exists():
        print(f"Error: BibTeX file not found: {args.bib}", file=sys.stderr)
        sys.exit(1)

    print(f"BibTeX file: {args.bib}")
    print(f"Output dir:  {args.out}")
    print(f"Report file: {args.report}")
    if args.dry_run:
        print("Mode:        DRY RUN (no downloads)")
    print()

    results = download_all(args.bib, args.out, dry_run=args.dry_run)
    write_report(results, args.report)

    # Many entries will be paywalled and will not download — that is expected.
    # Only treat it as a non-zero exit when at least one entry had a known
    # download source but the download itself failed (i.e., strategy != none
    # and downloaded is still False).
    failed = sum(
        1
        for r in results
        if not r["downloaded"]
        and not r.get("would_download")
        and r.get("strategy") != "none"
    )
    if failed:
        print(f"\n⚠ {failed} entries had a download source but could not be fetched.")
        sys.exit(1)


if __name__ == "__main__":
    main()
