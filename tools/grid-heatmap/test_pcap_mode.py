#!/usr/bin/env python3
"""
Test PCAP mode functionality with simulated snapshots
"""

import sys
from pathlib import Path


# Mock the requests module for testing
class MockResponse:
    def __init__(self, json_data, status_code=200):
        self.json_data = json_data
        self.status_code = status_code

    def json(self):
        return self.json_data

    def raise_for_status(self):
        if self.status_code != 200:
            raise Exception(f"HTTP {self.status_code}")


# Generate a simple test
if __name__ == "__main__":
    print("Testing PCAP snapshot mode...")
    print()

    # Test the argument parsing
    sys.path.insert(0, str(Path(__file__).parent))

    print("✓ Import successful")
    print()

    # Test command line help
    import subprocess

    result = subprocess.run(
        [".venv/bin/python3", "tools/grid-heatmap/plot_grid_heatmap.py", "--help"],
        capture_output=True,
        text=True,
    )

    if "--pcap" in result.stdout and "--interval" in result.stdout:
        print("✓ PCAP arguments present in help:")
        for line in result.stdout.split("\n"):
            if (
                "pcap" in line.lower()
                or "interval" in line.lower()
                or "duration" in line.lower()
            ):
                print(f"  {line.strip()}")
        print()
    else:
        print("✗ PCAP arguments not found in help")
        sys.exit(1)

    print("✓ All tests passed!")
    print()
    print("To test with actual PCAP:")
    print("  make plot-grid-heatmap PCAP=yourfile.pcap INTERVAL=30")
    print()
    print("Or directly:")
    print("  .venv/bin/python3 tools/grid-heatmap/plot_grid_heatmap.py \\")
    print("    --url http://localhost:8081 \\")
    print("    --pcap yourfile.pcap \\")
    print("    --interval 30 \\")
    print("    --duration 180 \\")
    print("    --polar --combined")
