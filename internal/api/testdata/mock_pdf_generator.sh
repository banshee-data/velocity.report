#!/bin/bash
# Mock PDF generator for end-to-end testing
# This script simulates the PDF generator by creating expected output files
# It reads the JSON config, extracts the output directory, and creates stub PDF/ZIP files

# The server calls this as: script -m pdf_generator.cli.main config_file
# So we need to handle these arguments and find the config file
while [[ $# -gt 0 ]]; do
  case $1 in
    -m)
      shift 2  # Skip -m and the module name
      ;;
    *)
      CONFIG_FILE="$1"
      shift
      ;;
  esac
done

if [ ! -f "$CONFIG_FILE" ]; then
  echo "Config file not found: $CONFIG_FILE" >&2
  exit 1
fi

# Extract output_dir from JSON config using grep and sed
OUTPUT_DIR=$(grep -o '"output_dir": *"[^"]*"' "$CONFIG_FILE" | sed 's/"output_dir": *"\([^"]*\)"/\1/')
if [ -z "$OUTPUT_DIR" ]; then
  echo "Could not extract output_dir from config" >&2
  exit 1
fi

# Create the output directory and files
mkdir -p "$OUTPUT_DIR"
touch "$OUTPUT_DIR/velocity.report_radar_data_transits_2025-10-01_to_2025-10-02_report.pdf"
touch "$OUTPUT_DIR/velocity.report_radar_data_transits_2025-10-01_to_2025-10-02_sources.zip"
exit 0
