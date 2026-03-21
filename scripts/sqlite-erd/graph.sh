#!/bin/bash

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
DEFAULT_DOT_FILE="$REPO_ROOT/data/structures/SCHEMA.dot"
DEFAULT_SVG_FILE="$REPO_ROOT/data/structures/SCHEMA.svg"

MODE="${SCHEMA_RENDER_MODE:-current}"
ACTION="generate"
PACK_COLUMNS="${SCHEMA_PACK_COLUMNS:-2}"
LAYOUT_ENGINE="${SCHEMA_LAYOUT_ENGINE:-osage}"
DOT_FILE="$DEFAULT_DOT_FILE"
SVG_FILE="$DEFAULT_SVG_FILE"
INPUT_FILE=""

usage() {
  cat <<EOF
Usage:
  $0 [--generate] [--mode current|minimal] [--pack-columns N] [--dot-output path] [--svg-output path] <schema.sql>
  $0 --compile [--mode current|minimal] [--pack-columns N] [--svg-output path] [<schema.dot>]

Modes:
  current  grouped DOT rendered with the current Graphviz layout settings
  minimal  grouped DOT rendered with plain 'dot -Tsvg' and minimal extra layout choices

Examples:
  $0 --generate internal/db/schema.sql
  $0 --generate --mode minimal internal/db/schema.sql
  $0 --compile --mode minimal
  $0 --compile data/structures/SCHEMA.dot
EOF
}

require_file() {
  local path="$1"
  local label="$2"
  if [ ! -f "$path" ]; then
    echo "Error: $label '$path' not found" >&2
    exit 1
  fi
}

ensure_positive_integer() {
  local value="$1"
  local label="$2"
  if ! [[ "$value" =~ ^[1-9][0-9]*$ ]]; then
    echo "Error: $label must be a positive integer" >&2
    exit 1
  fi
}

render_dot() {
  local dot_input="$1"
  local svg_output="$2"
  local tmp_output

  tmp_output=$(mktemp -t schema_svg)
  case "$MODE" in
    current)
      if ! command -v "$LAYOUT_ENGINE" >/dev/null 2>&1; then
        echo "Error: Graphviz layout engine '$LAYOUT_ENGINE' not found" >&2
        rm -f "$tmp_output"
        exit 1
      fi
      if ! "$LAYOUT_ENGINE" -Gpack=true "-Gpackmode=array_i${PACK_COLUMNS}" -Gsplines=curved -Tsvg "$dot_input" >"$tmp_output"; then
        echo "Error: failed to render schema SVG with Graphviz '$LAYOUT_ENGINE'" >&2
        rm -f "$tmp_output"
        exit 1
      fi
      ;;
    minimal)
      if ! command -v dot >/dev/null 2>&1; then
        echo "Error: Graphviz 'dot' not found" >&2
        rm -f "$tmp_output"
        exit 1
      fi
      if ! dot -Tsvg "$dot_input" >"$tmp_output"; then
        echo "Error: failed to render schema SVG with Graphviz 'dot'" >&2
        rm -f "$tmp_output"
        exit 1
      fi
      ;;
    *)
      echo "Error: unsupported mode '$MODE' (expected 'current' or 'minimal')" >&2
      rm -f "$tmp_output"
      exit 1
      ;;
  esac

  mv "$tmp_output" "$svg_output"
}

generate_dot() {
  local schema_file="$1"
  local dot_output="$2"
  local temp_db

  require_file "$schema_file" "Schema file"
  require_file "$SCRIPT_DIR/sqlite_graph.sql" "sqlite_graph.sql"
  require_file "$SCRIPT_DIR/group-dot.py" "group-dot.py"

  temp_db=$(mktemp -t schema_db)
  sqlite3 "$temp_db" < "$schema_file"
  sqlite3 "$temp_db" < "$SCRIPT_DIR/sqlite_graph.sql" | python3 "$SCRIPT_DIR/group-dot.py" >"$dot_output"
  rm -f "$temp_db"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --generate)
      ACTION="generate"
      shift
      ;;
    --compile)
      ACTION="compile"
      shift
      ;;
    --mode)
      shift
      if [ $# -eq 0 ]; then
        echo "Error: --mode requires a value" >&2
        usage
        exit 1
      fi
      MODE="$1"
      shift
      ;;
    --pack-columns)
      shift
      if [ $# -eq 0 ]; then
        echo "Error: --pack-columns requires a value" >&2
        usage
        exit 1
      fi
      PACK_COLUMNS="$1"
      shift
      ;;
    --dot-output)
      shift
      if [ $# -eq 0 ]; then
        echo "Error: --dot-output requires a value" >&2
        usage
        exit 1
      fi
      DOT_FILE="$1"
      shift
      ;;
    --svg-output)
      shift
      if [ $# -eq 0 ]; then
        echo "Error: --svg-output requires a value" >&2
        usage
        exit 1
      fi
      SVG_FILE="$1"
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --*)
      echo "Error: unknown option '$1'" >&2
      usage
      exit 1
      ;;
    *)
      if [ -n "$INPUT_FILE" ]; then
        echo "Error: unexpected extra argument '$1'" >&2
        usage
        exit 1
      fi
      INPUT_FILE="$1"
      shift
      ;;
  esac
done

ensure_positive_integer "$PACK_COLUMNS" "pack-columns"

mkdir -p "$(dirname "$DOT_FILE")" "$(dirname "$SVG_FILE")"

case "$ACTION" in
  generate)
    if [ -z "$INPUT_FILE" ]; then
      echo "Error: generate mode requires a schema.sql path" >&2
      usage
      exit 1
    fi
    generate_dot "$INPUT_FILE" "$DOT_FILE"
    render_dot "$DOT_FILE" "$SVG_FILE"
    echo "Schema DOT generated: $DOT_FILE"
    echo "Schema SVG generated: $SVG_FILE (mode: $MODE)"
    ;;
  compile)
    if [ -n "$INPUT_FILE" ]; then
      DOT_FILE="$INPUT_FILE"
    fi
    require_file "$DOT_FILE" "DOT file"
    render_dot "$DOT_FILE" "$SVG_FILE"
    echo "Schema SVG generated: $SVG_FILE (from: $DOT_FILE, mode: $MODE)"
    ;;
  *)
    echo "Error: unsupported action '$ACTION'" >&2
    exit 1
    ;;
esac
