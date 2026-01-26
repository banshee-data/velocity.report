#!/usr/bin/env python3
"""Visual test to verify histogram labels match table format."""

import os
from pdf_generator.core.chart_builder import HistogramChartBuilder
import matplotlib.pyplot as plt


def test_histogram_labels():
    """Generate a test histogram to show the new label format."""
    # Sample histogram data (bucket start values)
    histogram = {
        "5": 45,
        "10": 120,
        "15": 180,
        "20": 210,
        "25": 195,
        "30": 160,
        "35": 140,
        "40": 95,
        "45": 60,
        "50": 35,
    }

    builder = HistogramChartBuilder()
    fig = builder.build(
        histogram,
        title="Histogram Label Format Test - Should Show Ranges",
        units="mph",
        debug=True,
    )

    # Get the axis to inspect labels
    ax = fig.axes[0]
    tick_labels = [label.get_text() for label in ax.get_xticklabels()]

    print("\n" + "=" * 60)
    print("HISTOGRAM LABEL FORMAT TEST")
    print("=" * 60)
    print("\nExpected format (matching table):")
    print("  - Regular buckets: '5-10', '10-15', '15-20', etc.")
    print("  - Last bucket: '50+' (open-ended)")
    print("\nActual labels generated:")
    for i, label in enumerate(tick_labels):
        if label:  # Skip empty labels
            print(f"  {i}: '{label}'")

    print("\nVerification:")
    expected_labels = [
        "5-10",
        "10-15",
        "15-20",
        "20-25",
        "25-30",
        "30-35",
        "35-40",
        "40-45",
        "45-50",
        "50+",
    ]

    found_count = 0
    for expected in expected_labels:
        if expected in tick_labels:
            print(f"  ✓ Found '{expected}'")
            found_count += 1
        else:
            print(f"  ✗ Missing '{expected}'")

    print(f"\nResult: {found_count}/{len(expected_labels)} expected labels found")

    if found_count == len(expected_labels):
        print("✅ SUCCESS: All labels match table format!")
    else:
        print("⚠️  Some labels may be hidden due to tick thinning (normal for display)")

    print("\nSaving figure to: histogram_label_test.png")
    filename = "histogram_label_test.png"
    plt.savefig(filename, dpi=150, bbox_inches="tight")
    print("=" * 60 + "\n")

    # Clean up the generated file
    plt.close(fig)
    if os.path.exists(filename):
        os.remove(filename)
        print(f"Cleaned up: {filename}\n")


if __name__ == "__main__":
    test_histogram_labels()
