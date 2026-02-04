#!/usr/bin/env python3
"""Convert Xcode JSON coverage report to LCOV format for Codecov."""

import json
import sys


def convert_xcode_to_lcov(json_path: str) -> None:
    """Read Xcode JSON coverage and output LCOV format to stdout."""
    with open(json_path) as f:
        data = json.load(f)

    print("TN:")
    for target in data.get("targets", []):
        for file_data in target.get("files", []):
            path = file_data.get("path", "")
            if "VelocityVisualiser" in path and ".swift" in path:
                covered_lines = file_data.get("coveredLines", 0)
                executable_lines = file_data.get("executableLines", 1)

                print(f"SF:{path}")
                # Simplified - report coverage based on ratio
                for i in range(1, executable_lines + 1):
                    if i <= covered_lines:
                        print(f"DA:{i},1")
                    else:
                        print(f"DA:{i},0")
                print(f"LF:{executable_lines}")
                print(f"LH:{covered_lines}")
                print("end_of_record")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <coverage.json>", file=sys.stderr)
        sys.exit(1)
    convert_xcode_to_lcov(sys.argv[1])
