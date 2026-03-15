#!/bin/bash

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
OUTPUT_FILE="$REPO_ROOT/data/structures/SCHEMA.svg"

# Check if argument is provided
if [ $# -eq 0 ]; then
  echo "Usage: $0 <schema.sql>"
  exit 1
fi

SCHEMA_FILE="$1"

# Check if schema file exists
if [ ! -f "$SCHEMA_FILE" ]; then
  echo "Error: Schema file '$SCHEMA_FILE' not found"
  exit 1
fi

# Create temporary database
TEMP_DB=$(mktemp /tmp/schema_XXXXXX.db)

# Import schema into temporary database
sqlite3 "$TEMP_DB" < "$SCHEMA_FILE"

# Check if sqlite_graph.sql exists in the script directory
if [ ! -f "$SCRIPT_DIR/sqlite_graph.sql" ]; then
  echo "Error: sqlite_graph.sql not found in script directory"
  rm "$TEMP_DB"
  exit 1
fi

# Generate DOT file using sqlite_graph.sql
DOT_OUTPUT=$(sqlite3 "$TEMP_DB" < "$SCRIPT_DIR/sqlite_graph.sql")

# Create SVG using dot
echo "$DOT_OUTPUT" | dot -Tsvg > "$OUTPUT_FILE"

# Clean up
rm "$TEMP_DB"

echo "Schema diagram generated: $OUTPUT_FILE"
