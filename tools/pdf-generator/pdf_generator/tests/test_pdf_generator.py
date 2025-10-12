#!/usr/bin/env python3

"""Test script for the new PDF generator."""

import sys
import os

sys.path.append(os.path.dirname(__file__))

from pdf_generator.core.pdf_generator import generate_pdf_report

# Sample test data
overall_metrics = [
    {
        "Count": 3469,
        "P50Speed": 30.54,
        "P85Speed": 36.94,
        "P98Speed": 43.05,
        "MaxSpeed": 53.52,
    }
]

daily_metrics = [
    {
        "StartTime": "2025-06-02T00:00:00-07:00",
        "Count": 891,
        "P50Speed": 30.54,
        "P85Speed": 37.23,
        "P98Speed": 43.92,
        "MaxSpeed": 51.19,
    },
    {
        "StartTime": "2025-06-03T00:00:00-07:00",
        "Count": 1315,
        "P50Speed": 30.54,
        "P85Speed": 36.36,
        "P98Speed": 41.59,
        "MaxSpeed": 53.52,
    },
]

granular_metrics = [
    {
        "StartTime": "2025-06-02T08:00:00-07:00",
        "Count": 109,
        "P50Speed": 23.43,
        "P85Speed": 35.71,
        "P98Speed": 43.78,
        "MaxSpeed": 46.47,
    },
    {
        "StartTime": "2025-06-02T09:00:00-07:00",
        "Count": 152,
        "P50Speed": 30.54,
        "P85Speed": 37.52,
        "P98Speed": 42.47,
        "MaxSpeed": 46.83,
    },
]

histogram = {
    "5": 66,
    "10": 239,
    "15": 294,
    "20": 338,
    "25": 720,
    "30": 971,
    "35": 631,
    "40": 183,
    "45": 24,
    "50": 3,
}

if __name__ == "__main__":
    try:
        generate_pdf_report(
            output_path="test_report.pdf",
            start_iso="2025-06-02T00:00:00-07:00",
            end_iso="2025-06-04T23:59:59-07:00",
            group="1h",
            units="mph",
            timezone_display="US/Pacific",
            min_speed_str="5.0 mph",
            location="Clarendon Avenue, San Francisco",
            overall_metrics=overall_metrics,
            daily_metrics=daily_metrics,
            granular_metrics=granular_metrics,
            histogram=histogram,
            tz_name="US/Pacific",
            charts_prefix="test",
            speed_limit=25,
        )
        print("Test PDF generated successfully!")
    except Exception as e:
        print(f"Test failed: {e}")
        import traceback

        traceback.print_exc()
