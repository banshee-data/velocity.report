#!/usr/bin/env bash
#
# format-sql.sh - Format SQL files using sql-formatter with project settings
#
# This script formats SQL files using the sql-formatter tool with settings
# that match the VSCode Prettier-SQL extension configuration.
#
# IMPORTANT: Requires sql-formatter v12.2.4 for commaPosition support
#            Newer versions (15+) removed this feature.
#            Install with: npm install -g sql-formatter@12.2.4
#
# Usage:
#   ./scripts/format-sql.sh [file1.sql file2.sql ...]
#   ./scripts/format-sql.sh internal/db/schema.sql
#
# If no files are specified, formats only schema.sql (migrations excluded)
#
# Exit codes:
#   0 - Success
#   1 - Error (sql-formatter not found, wrong version, etc.)

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Check if sql-formatter is available
if ! command -v sql-formatter &> /dev/null; then
    echo -e "${RED}Error: sql-formatter command not found${NC}"
    echo "Please install sql-formatter v12.2.4:"
    echo "  npm install -g sql-formatter@12.2.4"
    echo ""
    echo "Note: We use v12.2.4 because newer versions (15+) removed commaPosition support"
    exit 1
fi

# Check version (warn if not 12.x)
SQL_FORMATTER_VERSION=$(sql-formatter --version 2>/dev/null || echo "unknown")
if [[ ! "$SQL_FORMATTER_VERSION" =~ ^12\. ]]; then
    echo -e "${YELLOW}Warning: sql-formatter version $SQL_FORMATTER_VERSION detected${NC}"
    echo "  This project requires v12.2.4 for commaPosition support"
    echo "  Newer versions (15+) removed this feature"
    echo "  Install correct version with: npm install -g sql-formatter@12.2.4"
    echo ""
fi

# Get list of files to format
FILES=()

if [ $# -eq 0 ]; then
    # No arguments - format only schema.sql (migrations are already formatted)
    echo "No files specified, formatting schema.sql only (migrations excluded)"
    cd "$PROJECT_ROOT"
    if [ -f "internal/db/schema.sql" ]; then
        FILES+=("internal/db/schema.sql")
    fi
else
    # Format specified files
    FILES=("$@")
fi

if [ ${#FILES[@]} -eq 0 ]; then
    echo -e "${YELLOW}No SQL files found to format${NC}"
    exit 0
fi

echo -e "${GREEN}Formatting ${#FILES[@]} SQL file(s)...${NC}"
echo ""

# Format each file
for file in "${FILES[@]}"; do
    # Resolve to absolute path if relative
    if [[ "$file" != /* ]]; then
        file="$PROJECT_ROOT/$file"
    fi

    if [ ! -f "$file" ]; then
        echo -e "${RED}Error: File not found: $file${NC}"
        continue
    fi

    echo "  Formatting: $file"

    # Format in place using project config
    if sql-formatter --fix -l sqlite -c "$PROJECT_ROOT/.sql-formatter.json" "$file" 2>&1; then
        echo -e "    ${GREEN}✓ Formatted${NC}"
    else
        echo -e "    ${RED}✗ Failed${NC}"
    fi
done

echo ""
echo -e "${GREEN}SQL formatting complete!${NC}"
