#!/bin/bash

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
OUTPUT_FILE="$REPO_ROOT/data/structures/SCHEMA.svg"

# Check if argument is provided
if [ $# -eq 0 ] || [ $# -gt 2 ]; then
  echo "Usage: $0 <schema.sql> [pack-columns]"
  exit 1
fi

SCHEMA_FILE="$1"
PACK_COLUMNS="${2:-2}"

# Check if schema file exists
if [ ! -f "$SCHEMA_FILE" ]; then
  echo "Error: Schema file '$SCHEMA_FILE' not found"
  exit 1
fi

if ! [[ "$PACK_COLUMNS" =~ ^[1-9][0-9]*$ ]]; then
  echo "Error: pack-columns must be a positive integer"
  exit 1
fi

# Create temporary database
TEMP_DB=$(mktemp -t schema_db)

# Import schema into temporary database
sqlite3 "$TEMP_DB" < "$SCHEMA_FILE"

# Check if sqlite_graph.sql exists in the script directory
if [ ! -f "$SCRIPT_DIR/sqlite_graph.sql" ]; then
  echo "Error: sqlite_graph.sql not found in script directory"
  rm "$TEMP_DB"
  exit 1
fi

if [ ! -f "$SCRIPT_DIR/group-dot.py" ]; then
  echo "Error: group-dot.py not found in script directory"
  rm "$TEMP_DB"
  exit 1
fi

# Generate DOT file using sqlite_graph.sql
DOT_OUTPUT=$(sqlite3 "$TEMP_DB" < "$SCRIPT_DIR/sqlite_graph.sql" | python3 "$SCRIPT_DIR/group-dot.py")

# Create SVG using dot. group-dot.py organizes tables into site/lidar/radar
# subgraphs, and pack/packmode arranges those subgraphs into a compact grid.
TMP_OUTPUT=$(mktemp -t schema_svg)
if ! echo "$DOT_OUTPUT" | dot -Gpack=true "-Gpackmode=array_i${PACK_COLUMNS}" -Gordering=out -Tsvg >"$TMP_OUTPUT"; then
  echo "Error: failed to render schema SVG with Graphviz 'dot'" >&2
  rm -f "$TMP_OUTPUT" "$TEMP_DB"
  exit 1
fi
mv "$TMP_OUTPUT" "$OUTPUT_FILE"

# Clean up
rm "$TEMP_DB"

echo "Schema diagram generated: $OUTPUT_FILE (pack columns: $PACK_COLUMNS)"
