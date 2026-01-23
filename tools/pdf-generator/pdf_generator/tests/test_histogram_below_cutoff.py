#!/usr/bin/env python3
"""Test histogram labels with below-cutoff bucket."""

import os
from pdf_generator.core.chart_builder import HistogramChartBuilder
import matplotlib.pyplot as plt


def test_with_below_cutoff_bucket():
    """Test histogram labels including a '<5' below-cutoff bucket."""
    # Histogram data with below-cutoff bucket
    # Note: The API might return this if there's data below min-speed
    histogram = {
        "<5": 12,  # Below-cutoff bucket (if present in API response)
        "5": 45,
        "10": 120,
        "15": 180,
        "20": 210,
        "25": 195,
    }

    builder = HistogramChartBuilder()
    fig = builder.build(
        histogram,
        title="Histogram with Below-Cutoff Bucket Test",
        units="mph",
        debug=True,
    )

    ax = fig.axes[0]
    tick_labels = [label.get_text() for label in ax.get_xticklabels()]

    print("\n" + "=" * 60)
    print("HISTOGRAM WITH BELOW-CUTOFF BUCKET TEST")
    print("=" * 60)
    print("\nInput buckets:")
    for bucket, count in sorted(histogram.items(), key=lambda x: str(x[0])):
        print(f"  {bucket}: {count}")

    print("\nGenerated labels:")
    for i, label in enumerate(tick_labels):
        if label:
            print(f"  {i}: '{label}'")

    print("\nExpected behavior:")
    print("  - '<5' bucket preserved as-is (non-numeric)")
    print("  - Numeric buckets formatted as ranges")
    print("  - Last bucket formatted as 'N+'")

    # Check for expected labels
    checks = [
        ("<5" in tick_labels, "Below-cutoff '<5' preserved"),
        ("5-10" in tick_labels, "Range '5-10' found"),
        ("10-15" in tick_labels, "Range '10-15' found"),
        ("25+" in tick_labels, "Open-ended '25+' found"),
    ]

    print("\nVerification:")
    all_passed = True
    for passed, description in checks:
        status = "✓" if passed else "✗"
        print(f"  {status} {description}")
        all_passed = all_passed and passed

    if all_passed:
        print("\n✅ SUCCESS: Mixed numeric/non-numeric labels handled correctly!")
    else:
        print("\n⚠️  Some checks failed")

    print("=" * 60 + "\n")

    filename = "histogram_below_cutoff_test.png"
    plt.savefig(filename, dpi=150, bbox_inches="tight")
    print(f"Saved figure to: {filename}\n")

    # Clean up the generated file
    plt.close(fig)
    if os.path.exists(filename):
        os.remove(filename)
        print(f"Cleaned up: {filename}\n")


if __name__ == "__main__":
    test_with_below_cutoff_bucket()
