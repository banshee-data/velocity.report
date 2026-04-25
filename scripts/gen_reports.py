#!/usr/bin/env python3
"""
Generate test PDF reports via the local velocity.report API and download
them to archive/go-tex/<YYYYMMDD-HHMMSS>/ under the repo root.

Usage:
    python3 scripts/gen_reports.py [http://localhost:8080] [--zip] [--all] [1 2]

Notes:
- By default the script will generate all reports. Use positional integers to
  request specific report numbers (1-based). Use `--zip` to also download
  the ZIP asset for each generated report; zips are skipped by default.
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from datetime import datetime

# Default base URL; legacy behaviour allows passing a base URL as the first
# argument (if it starts with http:// or https://).
BASE_URL = "http://localhost:8080"

# Output under repo root regardless of cwd.
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ARCHIVE_BASE = os.path.join(REPO_ROOT, "archive", "go-tex")

COMMON = {
    "site_id": 1,
    "timezone": "US/Pacific",
    "units": "mph",
    # Default source for queries; individual reports may override this.
    "source": "radar_objects",
    # Default compare source when generating comparison reports.
    "compare_source": "radar_objects",
    "min_speed": 5.0,
    "boundary_threshold": 5,
    "histogram": True,
    "hist_bucket_size": 5.0,
}

REPORTS = [
    {
        "label": "r1_comparison_2025-06-02_vs_2026-01-15",
        "record_id": 1,
        "group": "1h",
        "hist_max": 50.0,
        "start_date": "2025-06-02",
        "end_date": "2025-06-04",
        "compare_start_date": "2026-01-15",
        "compare_end_date": "2026-01-19",
        # per-report source override (optional)
        "source": "radar_data_transits",
        "compare_source": "radar_objects",
    },
    {
        "label": "r2_single_2025-07-01",
        "record_id": 2,
        "group": "24h",
        "hist_max": 35.0,
        "start_date": "2025-07-01",
        "end_date": "2026-08-31",
        "source": "radar_objects",
    },
    {
        "label": "r3_single_2025-08-01",
        "record_id": 3,
        "group": "2d",
        "hist_max": 30.0,
        "start_date": "2025-08-01",
        "end_date": "2025-12-31",
        "source": "radar_objects",
    },
    {
        "label": "r4_comparison_2025-07-01_vs_2025-08-01",
        "record_id": 4,
        "group": "24h",
        "hist_max": 30.0,
        "start_date": "2025-07-01",
        "end_date": "2025-07-30",
        "compare_start_date": "2025-08-01",
        "compare_end_date": "2025-08-31",
        "source": "radar_objects",
        "compare_source": "radar_objects",
    },
]


def post_json(url, payload):
    body = json.dumps(payload).encode()
    req = urllib.request.Request(
        url,
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as e:
        detail = e.read().decode(errors="replace")
        raise RuntimeError(f"HTTP {e.code} from {url}: {detail}") from None


def download(url, dest):
    try:
        urllib.request.urlretrieve(url, dest)
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"HTTP {e.code} downloading {url}") from None


def parse_cli(argv):
    # Support legacy first-arg base URL (starts with http) and strip it out for
    # the argparse parser so positional report ids still work.
    base_url = BASE_URL
    args_to_parse = list(argv)
    if len(args_to_parse) >= 1 and args_to_parse[0].startswith("http"):
        base_url = args_to_parse[0].rstrip("/")
        args_to_parse = args_to_parse[1:]

    parser = argparse.ArgumentParser(
        description="Generate test PDF reports via local velocity.report API"
    )
    parser.add_argument(
        "--all", action="store_true", help="Generate all reports (default)"
    )
    parser.add_argument(
        "--zip", action="store_true", help="Also download ZIP assets for each report"
    )
    parser.add_argument(
        "ids",
        metavar="N",
        nargs="*",
        type=int,
        help="Report numbers to generate (1-based)",
    )
    parsed = parser.parse_args(args_to_parse)

    if parsed.ids:
        selected = set(parsed.ids)
    else:
        selected = set(range(1, len(REPORTS) + 1))

    return base_url, selected, parsed.zip


def main():
    base_url, selected, keep_zip = parse_cli(sys.argv[1:])

    timestamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    out_dir = os.path.join(ARCHIVE_BASE, timestamp)
    os.makedirs(out_dir, exist_ok=True)
    print(f"Output: {out_dir}")
    print(f"API:    {base_url}\n")

    generated = 0
    for i, spec in enumerate(REPORTS, 1):
        if i not in selected:
            continue

        label = spec.get("label", f"report_{i}")
        payload = {**COMMON, **spec}
        # Remove label from payload if present (server doesn't generally need it)
        payload.pop("label", None)

        # Ensure comparison source is set when a comparison period is requested.
        if "compare_start_date" in payload and "compare_source" not in payload:
            payload["compare_source"] = payload.get("source")

        primary_source = payload.get("source", "<unset>")
        compare_note = ""
        if "compare_start_date" in payload:
            compare_src = payload.get("compare_source", primary_source)
            compare_note = f"  compare {payload['compare_start_date']}–{payload['compare_end_date']} src={compare_src}"

        print(f"[{i}/{len(REPORTS)}] {label} (id={payload.get('record_id', i)})")
        print(
            f"      {payload['start_date']}–{payload['end_date']} {payload['group']} hist_max={payload['hist_max']} src={primary_source}{compare_note}"
        )
        print("      Generating...", end="", flush=True)

        resp = post_json(f"{base_url}/api/generate_report", payload)
        if not resp.get("success"):
            print(f"\nFAILED: {resp}", file=sys.stderr)
            sys.exit(1)

        report_id = resp.get("report_id")
        pdf_name = os.path.basename(resp.get("pdf_path") or f"report_{report_id}.pdf")
        zip_name = os.path.basename(resp.get("zip_path") or f"report_{report_id}.zip")

        print(f" done (id={report_id})")

        pdf_url = f"{base_url}/api/reports/{report_id}/download/{pdf_name}"
        zip_url = f"{base_url}/api/reports/{report_id}/download/{zip_name}"

        print("      Downloading PDF...", end="", flush=True)
        download(pdf_url, os.path.join(out_dir, pdf_name))
        print(" done")

        if keep_zip:
            print("      Downloading ZIP...", end="", flush=True)
            download(zip_url, os.path.join(out_dir, zip_name))
            print(" done")

        generated += 1

    print(f"\n{generated} reports in: {out_dir}")


if __name__ == "__main__":
    main()
