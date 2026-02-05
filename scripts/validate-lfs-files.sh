#!/bin/bash
# validate-lfs-files.sh - Validate Git LFS files are properly fetched
#
# This script checks that Git LFS files are actual binary content,
# not LFS pointer files. Used in CI to catch LFS fetch failures.

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Colour

echo "=== Git LFS File Validation ==="
echo ""

# Check if git-lfs is installed
if ! command -v git-lfs &> /dev/null; then
    echo -e "${YELLOW}Warning: git-lfs not installed, attempting to install...${NC}"
    if command -v apt-get &> /dev/null; then
        sudo apt-get update && sudo apt-get install -y git-lfs
    elif command -v brew &> /dev/null; then
        brew install git-lfs
    else
        echo -e "${RED}Error: Cannot install git-lfs automatically${NC}"
        exit 1
    fi
fi

echo "Git LFS version: $(git lfs version)"
echo ""

# Track validation results
ERRORS=0
VALIDATED=0

# Function to check if a file is an LFS pointer (text file with specific format)
is_lfs_pointer() {
    local file="$1"
    # LFS pointer files start with "version https://git-lfs.github.com/spec/"
    if head -c 100 "$file" 2>/dev/null | grep -q "^version https://git-lfs.github.com/spec/"; then
        return 0  # Is a pointer
    fi
    return 1  # Is actual content
}

# Function to validate a specific file
validate_file() {
    local file="$1"
    local expected_type="$2"

    echo "Checking: $file"

    if [ ! -f "$file" ]; then
        echo -e "  ${RED}✗ File does not exist${NC}"
        return 1
    fi

    local size
    size=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null)
    echo "  Size: $size bytes"

    # Check if it's still an LFS pointer (unfetched)
    if is_lfs_pointer "$file"; then
        echo -e "  ${RED}✗ File is an LFS pointer (not fetched)${NC}"
        echo "  Content preview:"
        head -5 "$file" | sed 's/^/    /'
        return 1
    fi

    # Check file type
    local file_type
    file_type=$(file "$file")
    echo "  Type: $file_type"

    if [ -n "$expected_type" ]; then
        if echo "$file_type" | grep -qi "$expected_type"; then
            echo -e "  ${GREEN}✓ File type matches expected: $expected_type${NC}"
        else
            echo -e "  ${RED}✗ File type mismatch. Expected: $expected_type${NC}"
            return 1
        fi
    fi

    echo -e "  ${GREEN}✓ Valid${NC}"
    return 0
}

echo "--- Validating LFS-tracked files ---"
echo ""

# Validate kirk0.pcapng specifically (critical for LiDAR tests)
PCAP_FILE="internal/lidar/perf/pcap/kirk0.pcapng"
if [ -f "$PCAP_FILE" ] || git lfs ls-files | grep -q "kirk0.pcapng"; then
    if validate_file "$PCAP_FILE" "pcapng"; then
        VALIDATED=$((VALIDATED + 1))
    else
        ERRORS=$((ERRORS + 1))
        echo ""
        echo -e "${YELLOW}Attempting to fetch LFS file...${NC}"
        git lfs pull --include="$PCAP_FILE" || true
        echo ""
        echo "Retrying validation after fetch..."
        if validate_file "$PCAP_FILE" "pcapng"; then
            VALIDATED=$((VALIDATED + 1))
            ERRORS=$((ERRORS - 1))
        fi
    fi
else
    echo "  Skipping $PCAP_FILE (not present in repo)"
fi

echo ""
echo "--- Summary ---"
echo "Validated: $VALIDATED"
echo "Errors: $ERRORS"

if [ $ERRORS -gt 0 ]; then
    echo ""
    echo -e "${RED}LFS validation failed!${NC}"
    echo ""
    echo "To fix, ensure Git LFS is properly configured:"
    echo "  1. Install git-lfs: https://git-lfs.github.com/"
    echo "  2. Run: git lfs install"
    echo "  3. Run: git lfs pull"
    echo ""
    echo "In CI, add 'lfs: true' to actions/checkout or run 'git lfs pull'"
    exit 1
fi

echo ""
echo -e "${GREEN}All LFS files validated successfully!${NC}"
exit 0
