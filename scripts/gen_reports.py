#!/usr/bin/env python3
"""
Generate 4 test PDF reports via the local velocity.report API and download
them to archive/go-tex/<YYYYMMDD-HHMMSS>/ under the repo root.

Usage:
    python3 scripts/gen_reports.py [http://localhost:8080]
"""

import json
import os
import sys
import urllib.error
import urllib.request
from datetime import datetime

BASE_URL = sys.argv[1].rstrip("/") if len(sys.argv) > 1 else "http://localhost:8080"

# Output under repo root regardless of cwd.
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ARCHIVE_BASE = os.path.join(REPO_ROOT, "archive", "go-tex")

COMMON = {
    "site_id": 1,
    "timezone": "US/Pacific",
    "units": "mph",
    "source": "radar_data_transits",
    "min_speed": 5.0,
    "boundary_threshold": 5,
    "histogram": True,
    "hist_bucket_size": 5.0,
}

REPORTS = [
    {
        "_label": "r1_comparison_2025-06-02_vs_2026-01-15",
        "group": "1h",
        "hist_max": 50.0,
        "start_date": "2025-06-02",
        "end_date": "2025-06-04",
        "compare_start_date": "2026-01-15",
        "compare_end_date": "2026-01-19",
    },
    {
        "_label": "r2_single_2025-07-01",
        "group": "24h",
        "hist_max": 35.0,
        "start_date": "2025-07-01",
        "end_date": "2026-08-31",
    },
    {
        "_label": "r3_single_2025-08-01",
        "group": "24h",
        "hist_max": 35.0,
        "start_date": "2025-08-01",
        "end_date": "2025-12-31",
    },
    {
        "_label": "r4_comparison_2025-07-01_vs_2025-08-01",
        "group": "24h",
        "hist_max": 35.0,
        "start_date": "2025-07-01",
        "end_date": "2025-07-30",
        "compare_start_date": "2025-08-01",
        "compare_end_date": "2025-08-31",
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


def main():
    timestamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    out_dir = os.path.join(ARCHIVE_BASE, timestamp)
    os.makedirs(out_dir, exist_ok=True)
    print(f"Output: {out_dir}")
    print(f"API:    {BASE_URL}\n")

    for i, spec in enumerate(REPORTS, 1):
        label = spec.pop("_label")
        payload = {**COMMON, **spec}

        compare_note = ""
        if "compare_start_date" in payload:
            compare_note = f"  compare {payload['compare_start_date']}–{payload['compare_end_date']}"

        print(f"[{i}/4] {label}")
        print(
            f"      {payload['start_date']}–{payload['end_date']} {payload['group']} hist_max={payload['hist_max']}{compare_note}"
        )
        print(f"      Generating...", end="", flush=True)

        resp = post_json(f"{BASE_URL}/api/generate_report", payload)
        if not resp.get("success"):
            print(f"\nFAILED: {resp}", file=sys.stderr)
            sys.exit(1)

        report_id = resp["report_id"]
        pdf_name = os.path.basename(resp["pdf_path"])
        zip_name = os.path.basename(resp["zip_path"])

        print(f" done (id={report_id})")

        pdf_url = f"{BASE_URL}/api/reports/{report_id}/download/{pdf_name}"
        zip_url = f"{BASE_URL}/api/reports/{report_id}/download/{zip_name}"

        print(f"      Downloading PDF...", end="", flush=True)
        download(pdf_url, os.path.join(out_dir, pdf_name))
        print(" done")

        print(f"      Downloading ZIP...", end="", flush=True)
        download(zip_url, os.path.join(out_dir, zip_name))
        print(" done")

    print(f"\n{len(REPORTS)} reports in: {out_dir}")


if __name__ == "__main__":
    main()
