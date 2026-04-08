#!/usr/bin/env bash
# check-quarter-blocks.sh — Reject Unicode quarter-block characters (U+2596–U+259F)
#
# Quarter-block glyphs render as blanks on the Raspberry Pi Linux console
# because the default framebuffer font does not include them.  Only full
# blocks (█), half blocks (▀ ▄ ▌ ▐), shade blocks (░ ▒ ▓), and standard
# box-drawing characters are safe for console output.
#
# Usage:
#   scripts/check-quarter-blocks.sh            # exit 1 on failures
#   scripts/check-quarter-blocks.sh --report   # advisory, always exit 0

set -euo pipefail

REPORT_MODE=false
if [[ "${1:-}" == "--report" ]]; then
    REPORT_MODE=true
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Quarter-block characters: ▖▗▘▙▚▛▜▝▞▟ (U+2596 through U+259F)
QUARTER_BLOCKS='[▖▗▘▙▚▛▜▝▞▟]'

found=0

while IFS= read -r file; do
    # Search for quarter-block characters with line numbers
    if grep -n "$QUARTER_BLOCKS" "$file" > /dev/null 2>&1; then
        grep -n "$QUARTER_BLOCKS" "$file" | while IFS= read -r match; do
            lineno="${match%%:*}"
            rel="${file#"$REPO_ROOT"/}"
            echo "  $rel:$lineno  contains quarter-block character (U+2596–U+259F)"
        done
        found=1
    fi
done < <(
    cd "$REPO_ROOT"
    git ls-files -z -- '*.sh' '*.md' '*.txt' '*.py' '*.go' '*.swift' '*.svelte' '*.ts' '*.js' \
        | tr '\0' '\n'                                                                          \
        | while IFS= read -r f; do echo "$REPO_ROOT/$f"; done
)

if [[ "$found" -eq 1 ]]; then
    echo ""
    echo "Quarter-block characters (▖▗▘▙▚▛▜▝▞▟) do not render on the"
    echo "Raspberry Pi console font.  Use only full blocks (█), half"
    echo "blocks (▀ ▄ ▌ ▐), shade blocks (░ ▒ ▓), or box-drawing instead."
    if [[ "$REPORT_MODE" == true ]]; then
        exit 0
    fi
    exit 1
fi
