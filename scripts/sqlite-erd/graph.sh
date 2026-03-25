#!/bin/bash

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
DEFAULT_DOT_FILE="$REPO_ROOT/data/structures/SCHEMA.dot"
DEFAULT_SVG_FILE="$REPO_ROOT/data/structures/SCHEMA.svg"

ACTION="generate"
DOT_FILE="$DEFAULT_DOT_FILE"
SVG_FILE="$DEFAULT_SVG_FILE"
INPUT_FILE=""
LAYOUT_MODE="full"
TEMP_FILES=()

cleanup_temp_files() {
  if [ ${#TEMP_FILES[@]} -gt 0 ]; then
    rm -f "${TEMP_FILES[@]}"
  fi
}

make_temp_file() {
  local prefix="$1"
  local suffix="${2:-}"
  local temp_dir="${TMPDIR:-/tmp}"
  local template="${temp_dir%/}/${prefix}.XXXXXX"
  local temp_file

  temp_file=$(mktemp "$template")
  if [ -n "$suffix" ]; then
    local suffixed_temp_file="${temp_file}${suffix}"
    mv "$temp_file" "$suffixed_temp_file"
    temp_file="$suffixed_temp_file"
  fi
  TEMP_FILES+=("$temp_file")
  printf '%s\n' "$temp_file"
}

trap cleanup_temp_files EXIT

usage() {
  cat <<EOF
Usage:
  $0 [--generate] [--layout full|auto] [--svg-output path] <schema.sql>
  $0 --generate-dot [--layout full|auto] [--dot-output path] <schema.sql>
  $0 --compile [--svg-output path] [<schema.dot>]

Layout modes:
  full  — (default) clustered with subgroups and alignment edges.
  auto  — family clusters only; Graphviz handles all routing.

Examples:
  $0 --generate internal/db/schema.sql
  $0 --layout auto --generate internal/db/schema.sql
  $0 --generate-dot --dot-output /tmp/schema.dot internal/db/schema.sql
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

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Error: required command '$command_name' not found" >&2
    exit 1
  fi
}

render_dot() {
  local dot_input="$1"
  local svg_output="$2"
  local tmp_output

  tmp_output=$(make_temp_file schema_svg .svg)
  require_command dot
  if ! dot -Tsvg "$dot_input" >"$tmp_output"; then
    echo "Error: failed to render schema SVG with Graphviz 'dot'" >&2
    exit 1
  fi

  mv "$tmp_output" "$svg_output"
}

generate_dot() {
  local schema_file="$1"
  local dot_output="$2"
  local temp_db
  local tmp_dot_output

  require_file "$schema_file" "Schema file"
  require_file "$SCRIPT_DIR/sqlite_graph.sql" "sqlite_graph.sql"
  require_file "$SCRIPT_DIR/group-dot.py" "group-dot.py"
  require_command sqlite3
  require_command python3

  temp_db=$(make_temp_file schema_db .db)
  tmp_dot_output=$(make_temp_file schema_dot .dot)
  if ! sqlite3 "$temp_db" < "$schema_file"; then
    echo "Error: failed to import schema into temporary SQLite database" >&2
    exit 1
  fi
  if ! sqlite3 "$temp_db" < "$SCRIPT_DIR/sqlite_graph.sql" | python3 "$SCRIPT_DIR/group-dot.py" --layout "$LAYOUT_MODE" >"$tmp_dot_output"; then
    echo "Error: failed to generate schema DOT" >&2
    exit 1
  fi
  mv "$tmp_dot_output" "$dot_output"
}

generate_svg() {
  local schema_file="$1"
  local svg_output="$2"
  local temp_dot

  temp_dot=$(make_temp_file schema_dot .dot)
  generate_dot "$schema_file" "$temp_dot"
  render_dot "$temp_dot" "$svg_output"
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
    --generate-dot)
      ACTION="generate-dot"
      shift
      ;;
    --layout)
      shift
      if [ $# -eq 0 ]; then
        echo "Error: --layout requires a value (full or auto)" >&2
        usage
        exit 1
      fi
      case "$1" in
        full|auto)
          LAYOUT_MODE="$1"
          ;;
        *)
          echo "Error: invalid layout mode '$1' (expected: full or auto)" >&2
          usage
          exit 1
          ;;
      esac
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

mkdir -p "$(dirname "$DOT_FILE")" "$(dirname "$SVG_FILE")"

case "$ACTION" in
  generate)
    if [ -z "$INPUT_FILE" ]; then
      echo "Error: generate mode requires a schema.sql path" >&2
      usage
      exit 1
    fi
    generate_svg "$INPUT_FILE" "$SVG_FILE"
    echo "Schema SVG generated: $SVG_FILE"
    ;;
  generate-dot)
    if [ -z "$INPUT_FILE" ]; then
      echo "Error: generate-dot mode requires a schema.sql path" >&2
      usage
      exit 1
    fi
    generate_dot "$INPUT_FILE" "$DOT_FILE"
    echo "Schema DOT generated: $DOT_FILE"
    ;;
  compile)
    if [ -n "$INPUT_FILE" ]; then
      DOT_FILE="$INPUT_FILE"
    fi
    require_file "$DOT_FILE" "DOT file"
    render_dot "$DOT_FILE" "$SVG_FILE"
    echo "Schema SVG generated: $SVG_FILE (from: $DOT_FILE)"
    ;;
  *)
    echo "Error: unsupported action '$ACTION'" >&2
    exit 1
    ;;
esac
