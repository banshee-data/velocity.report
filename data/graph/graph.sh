#!/bin/bash

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

# Get the directory of the schema file
SCHEMA_DIR=$(dirname "$SCHEMA_FILE")
SCHEMA_NAME=$(basename "$SCHEMA_FILE" .sql)

# Create temporary database
TEMP_DB=$(mktemp /tmp/schema_XXXXXX.db)

# Import schema into temporary database
sqlite3 "$TEMP_DB" < "$SCHEMA_FILE"

# Check if sqlite_graph.sql exists in current directory
if [ ! -f "$(dirname "$0")/sqlite_graph.sql" ]; then
  echo "Error: sqlite_graph.sql not found in script directory"
  rm "$TEMP_DB"
  exit 1
fi

# Generate DOT file using sqlite_graph.sql
DOT_OUTPUT=$(sqlite3 "$TEMP_DB" < "$(dirname "$0")/sqlite_graph.sql")

# Create SVG using dot
echo "$DOT_OUTPUT" | dot -Tsvg > "$SCHEMA_DIR/schema.svg"

# Clean up
rm "$TEMP_DB"

echo "Schema diagram generated: $SCHEMA_DIR/schema.svg"
