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

# Extract output_dir, end_date, and location from JSON config
OUTPUT_DIR=$(grep -o '"output_dir": *"[^"]*"' "$CONFIG_FILE" | sed 's/"output_dir": *"\([^"]*\)"/\1/')
END_DATE=$(grep -o '"end_date": *"[^"]*"' "$CONFIG_FILE" | sed 's/"end_date": *"\([^"]*\)"/\1/')
LOCATION=$(grep -o '"location": *"[^"]*"' "$CONFIG_FILE" | sed 's/"location": *"\([^"]*\)"/\1/' | sed 's/ /_/g')

if [ -z "$OUTPUT_DIR" ] || [ -z "$END_DATE" ] || [ -z "$LOCATION" ]; then
  echo "Could not extract required fields from config" >&2
  echo "output_dir: $OUTPUT_DIR, end_date: $END_DATE, location: $LOCATION" >&2
  exit 1
fi

# Create the output directory and files with new filename format:
# {endDate}_velocity.report_{location}.ext
mkdir -p "$OUTPUT_DIR"
touch "$OUTPUT_DIR/${END_DATE}_velocity.report_${LOCATION}.pdf"
touch "$OUTPUT_DIR/${END_DATE}_velocity.report_${LOCATION}.zip"
exit 0
