#!/usr/bin/env bash
# Check latest available CPython version and validate repo references
# Usage: ./scripts/check-python-version.sh [--fix]

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if running in fix mode
FIX_MODE=false
if [[ "${1:-}" == "--fix" ]]; then
    FIX_MODE=true
fi

echo -e "${BLUE}=== Python Version Consistency Checker ===${NC}"
echo ""

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Detect latest available CPython version on this system
echo -e "${BLUE}Step 1: Detecting available Python versions...${NC}"
AVAILABLE_VERSIONS=()

# Dynamically detect Python versions from 3.20 down to 3.8
# This ensures the script works for future Python releases
for major in 3; do
    for minor in {20..8}; do
        version="${major}.${minor}"
        if command_exists "python${version}"; then
            AVAILABLE_VERSIONS+=("${version}")
            version_output=$(python${version} --version 2>&1 || true)
            echo -e "  ${GREEN}✓${NC} python${version}: ${version_output}"
        fi
    done
done

if [ ${#AVAILABLE_VERSIONS[@]} -eq 0 ]; then
    echo -e "${RED}✗ No Python 3.x versions found!${NC}"
    exit 1
fi

LATEST_VERSION="${AVAILABLE_VERSIONS[0]}"
echo ""
echo -e "${GREEN}Latest available Python version: ${LATEST_VERSION}${NC}"
echo ""

# Check GitHub Actions availability
echo -e "${BLUE}Step 2: Determining target Python version...${NC}"
echo -e "  ${YELLOW}Note: GitHub Actions supports officially released CPython versions only${NC}"

# Determine target version for ENTIRE repository
# If the latest system version is unreleased/preview (e.g., 3.15-dev, 3.14.0a1),
# fall back to the next stable version
TARGET_VERSION="$LATEST_VERSION"

# Check if there's a more stable version available
if [ ${#AVAILABLE_VERSIONS[@]} -gt 1 ]; then
    # Check if latest version might be pre-release by looking at version output
    latest_full_version=$(python${LATEST_VERSION} --version 2>&1 || echo "")

    # If version contains alpha, beta, rc, dev, or is very recent (3.14+), use second-latest
    if echo "$latest_full_version" | grep -qE "(alpha|beta|rc|dev|a[0-9]|b[0-9])"; then
        TARGET_VERSION="${AVAILABLE_VERSIONS[1]}"
        echo -e "  ${RED}⚠ WARNING: Latest version ${LATEST_VERSION} is pre-release${NC}"
        echo -e "  ${YELLOW}→ Using stable version ${TARGET_VERSION} for repository consistency${NC}"
    elif [[ "${LATEST_VERSION}" > "3.13" ]]; then
        # Future-proofing: versions beyond 3.13 may not be in GitHub Actions yet
        TARGET_VERSION="${AVAILABLE_VERSIONS[1]}"
        echo -e "  ${RED}⚠ WARNING: Python ${LATEST_VERSION} may not be available in GitHub Actions yet${NC}"
        echo -e "  ${YELLOW}→ Using version ${TARGET_VERSION} for CI compatibility${NC}"
    else
        echo -e "  ${GREEN}→ Target version: ${TARGET_VERSION}${NC}"
    fi
else
    echo -e "  ${GREEN}→ Target version: ${TARGET_VERSION}${NC}"
fi
echo ""
echo -e "${BLUE}Policy: ALL Python references must use version ${TARGET_VERSION}${NC}"
echo ""

# Find all Python version references in the repo
echo -e "${BLUE}Step 3: Scanning repository for Python version references...${NC}"
ISSUES=()
CHECKED_FILES=0

# Scan workflows
echo -e "\n${BLUE}Checking GitHub Actions workflows...${NC}"
if [ -d ".github/workflows" ]; then
    shopt -s nullglob 2>/dev/null || true  # Bash: don't error on no matches
    setopt NULL_GLOB 2>/dev/null || true    # Zsh: don't error on no matches

    for file in .github/workflows/*.yml .github/workflows/*.yaml; do
        [ -f "$file" ] || continue
        ((CHECKED_FILES++))

        if grep -q "python-version:" "$file" 2>/dev/null; then
            while IFS= read -r line; do
                line_num=$(echo "$line" | cut -d: -f1)
                content=$(echo "$line" | cut -d: -f2-)

                # Extract version from YAML (handles quotes and whitespace)
                version=$(echo "$content" | sed -n 's/.*python-version: *"\([0-9.]*\)".*/\1/p')

                if [[ -n "$version" ]]; then
                    if [[ "$version" != "$TARGET_VERSION" ]]; then
                        ISSUES+=("${file}:${line_num}: WRONG VERSION python-version: \"${version}\" (MUST be ${TARGET_VERSION})")
                        echo -e "  ${RED}✗ MISMATCH${NC} ${file}:${line_num} - python-version: \"${version}\" ${RED}(MUST be ${TARGET_VERSION})${NC}"
                    else
                        echo -e "  ${GREEN}✓${NC} ${file}:${line_num} - python-version: \"${version}\""
                    fi
                fi
            done < <(grep -n "python-version:" "$file" 2>/dev/null)
        fi
    done
fi

# Scan Makefile
echo -e "\n${BLUE}Checking Makefile...${NC}"
if [ -f "Makefile" ]; then
    ((CHECKED_FILES++))
    if grep -n "python3\.[0-9]\+" Makefile >/dev/null 2>&1; then
        while IFS= read -r line; do
            line_num=$(echo "$line" | cut -d: -f1)
            content=$(echo "$line" | cut -d: -f2-)

            # Extract version
            versions=$(echo "$content" | grep -o "python3\.[0-9]\+" | sort -u)
            for version in $versions; do
                version_num=$(echo "$version" | sed 's/python//')
                if [[ "$version_num" != "$TARGET_VERSION" ]]; then
                    ISSUES+=("Makefile:${line_num}: WRONG VERSION ${version} (MUST be python${TARGET_VERSION})")
                    echo -e "  ${RED}✗ MISMATCH${NC} Makefile:${line_num} - ${version} ${RED}(MUST be python${TARGET_VERSION})${NC}"
                else
                    echo -e "  ${GREEN}✓${NC} Makefile:${line_num} - ${version}"
                fi
            done
        done < <(grep -n "python3\.[0-9]\+" Makefile)
    fi
fi

# Scan scripts
echo -e "\n${BLUE}Checking scripts...${NC}"
if [ -d "scripts" ]; then
    while IFS= read -r -d '' file; do
        ((CHECKED_FILES++))
        if grep -n "python3\.[0-9]\+" "$file" >/dev/null 2>&1; then
            while IFS= read -r line; do
                line_num=$(echo "$line" | cut -d: -f1)
                content=$(echo "$line" | cut -d: -f2-)

                versions=$(echo "$content" | grep -o "python3\.[0-9]\+" | sort -u)
                for version in $versions; do
                    version_num=$(echo "$version" | sed 's/python//')
                    if [[ "$version_num" != "$TARGET_VERSION" ]]; then
                        ISSUES+=("${file}:${line_num}: WRONG VERSION ${version} (MUST be python${TARGET_VERSION})")
                        echo -e "  ${RED}✗ MISMATCH${NC} ${file}:${line_num} - ${version} ${RED}(MUST be python${TARGET_VERSION})${NC}"
                    else
                        echo -e "  ${GREEN}✓${NC} ${file}:${line_num} - ${version}"
                    fi
                done
            done < <(grep -n "python3\.[0-9]\+" "$file")
        fi
    done < <(find scripts -name "*.sh" -type f -print0 2>/dev/null)
fi

# Scan Python shebangs
echo -e "\n${BLUE}Checking Python file shebangs...${NC}"
shebang_count=0
while IFS= read -r -d '' file; do
    if head -n1 "$file" | grep -q "#!/usr/bin/env python3\.[0-9]\+"; then
        ((shebang_count++))
        ((CHECKED_FILES++))
        line=$(head -n1 "$file")
        version=$(echo "$line" | grep -o "python3\.[0-9]\+" | head -1)
        version_num=$(echo "$version" | sed 's/python//')

        if [[ "$version_num" != "$TARGET_VERSION" ]]; then
            ISSUES+=("${file}:1: WRONG VERSION shebang ${version} (MUST be python${TARGET_VERSION})")
            echo -e "  ${RED}✗ MISMATCH${NC} ${file}:1 - ${version} ${RED}(MUST be python${TARGET_VERSION})${NC}"
        fi
    fi
done < <(find . -name "*.py" -type f ! -path "./.venv/*" ! -path "./venv/*" ! -path "*/node_modules/*" ! -path "*/__pycache__/*" -print0 2>/dev/null)

if [ $shebang_count -gt 0 ]; then
    echo -e "  ${GREEN}Checked ${shebang_count} Python files with versioned shebangs${NC}"
fi

# Scan documentation
echo -e "\n${BLUE}Checking documentation...${NC}"
doc_files=0
if [ -f "README.md" ]; then
    ((doc_files++))
    ((CHECKED_FILES++))
    if grep -n "python3\.[0-9]\+\|Python 3\.[0-9]\+" README.md >/dev/null 2>&1; then
        while IFS= read -r line; do
            line_num=$(echo "$line" | cut -d: -f1)
            echo -e "  ${BLUE}ℹ${NC} README.md:${line_num} - contains Python version reference"
        done < <(grep -n "python3\.[0-9]\+\|Python 3\.[0-9]\+" README.md | head -5)
    fi
fi

if [ -d "docs" ]; then
    doc_count=0
    while IFS= read -r -d '' file; do
        ((doc_files++))
        ((CHECKED_FILES++))
        ((doc_count++))
        if grep -n "python3\.[0-9]\+\|Python 3\.[0-9]\+" "$file" >/dev/null 2>&1; then
            count=$(grep -c "python3\.[0-9]\+\|Python 3\.[0-9]\+" "$file")
            echo -e "  ${BLUE}ℹ${NC} ${file} - ${count} Python version reference(s)"
        fi
        # Limit to first 10 docs
        if [ $doc_count -ge 10 ]; then
            break
        fi
    done < <(find docs -name "*.md" -type f -print0 2>/dev/null)
fi

echo ""
echo -e "${BLUE}=== Summary ===${NC}"
echo -e "Checked ${CHECKED_FILES} files"
echo -e "Latest system Python: ${GREEN}${LATEST_VERSION}${NC}"
echo -e "Target version (REQUIRED): ${GREEN}${TARGET_VERSION}${NC}"
echo ""

if [ ${#ISSUES[@]} -gt 0 ]; then
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${RED}POLICY VIOLATION: Found ${#ISSUES[@]} version mismatch(es)${NC}"
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    for issue in "${ISSUES[@]}"; do
        echo -e "  ${RED}▶${NC} $issue"
    done
    echo ""
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}ACTION REQUIRED:${NC}"
    echo -e "  All Python references MUST use version ${TARGET_VERSION}"
    echo -e "  Update ALL mismatched references to: python${TARGET_VERSION}"
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""

    if [ "$FIX_MODE" = true ]; then
        echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e "${YELLOW}FIXING VERSION MISMATCHES...${NC}"
        echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo ""

        FIXED_COUNT=0
        FAILED_COUNT=0

        # Fix GitHub Actions workflows
        if [ -d ".github/workflows" ]; then
            for file in .github/workflows/*.yml .github/workflows/*.yaml; do
                [ -f "$file" ] || continue

                if grep -q "python-version:" "$file" 2>/dev/null; then
                    # Find all mismatched versions in this file
                    while IFS= read -r line; do
                        line_num=$(echo "$line" | cut -d: -f1)
                        content=$(echo "$line" | cut -d: -f2-)
                        version=$(echo "$content" | sed -n 's/.*python-version: *"\([0-9.]*\)".*/\1/p')

                        if [[ -n "$version" && "$version" != "$TARGET_VERSION" ]]; then
                            # Use sed to replace the version in-place
                            if sed -i '' "s/python-version: \"${version}\"/python-version: \"${TARGET_VERSION}\"/g" "$file" 2>/dev/null; then
                                echo -e "  ${GREEN}✓ FIXED${NC} ${file}:${line_num} - python-version: \"${version}\" → \"${TARGET_VERSION}\""
                                ((FIXED_COUNT++))
                            else
                                echo -e "  ${RED}✗ FAILED${NC} ${file}:${line_num} - could not update"
                                ((FAILED_COUNT++))
                            fi
                        fi
                    done < <(grep -n "python-version:" "$file" 2>/dev/null)
                fi
            done
        fi

        # Fix Makefile
        if [ -f "Makefile" ]; then
            if grep -q "python3\.[0-9]\+" Makefile 2>/dev/null; then
                # Get unique wrong versions
                wrong_versions=$(grep -o "python3\.[0-9]\+" Makefile | sort -u | grep -v "python${TARGET_VERSION}" || true)

                for wrong_version in $wrong_versions; do
                    # Replace all occurrences of this wrong version
                    if sed -i '' "s/${wrong_version}/python${TARGET_VERSION}/g" Makefile 2>/dev/null; then
                        count=$(grep -c "python${TARGET_VERSION}" Makefile || echo "0")
                        echo -e "  ${GREEN}✓ FIXED${NC} Makefile - ${wrong_version} → python${TARGET_VERSION}"
                        ((FIXED_COUNT++))
                    else
                        echo -e "  ${RED}✗ FAILED${NC} Makefile - could not update ${wrong_version}"
                        ((FAILED_COUNT++))
                    fi
                done
            fi
        fi

        # Fix shell scripts
        if [ -d "scripts" ]; then
            while IFS= read -r -d '' file; do
                if grep -q "python3\.[0-9]\+" "$file" 2>/dev/null; then
                    wrong_versions=$(grep -o "python3\.[0-9]\+" "$file" | sort -u | grep -v "python${TARGET_VERSION}" || true)

                    for wrong_version in $wrong_versions; do
                        if sed -i '' "s/${wrong_version}/python${TARGET_VERSION}/g" "$file" 2>/dev/null; then
                            echo -e "  ${GREEN}✓ FIXED${NC} ${file} - ${wrong_version} → python${TARGET_VERSION}"
                            ((FIXED_COUNT++))
                        else
                            echo -e "  ${RED}✗ FAILED${NC} ${file} - could not update ${wrong_version}"
                            ((FAILED_COUNT++))
                        fi
                    done
                fi
            done < <(find scripts -name "*.sh" -type f -print0 2>/dev/null)
        fi

        # Fix Python shebangs
        while IFS= read -r -d '' file; do
            if head -n1 "$file" | grep -q "#!/usr/bin/env python3\.[0-9]\+"; then
                line=$(head -n1 "$file")
                version=$(echo "$line" | grep -o "python3\.[0-9]\+" | head -1)
                version_num=$(echo "$version" | sed 's/python//')

                if [[ "$version_num" != "$TARGET_VERSION" ]]; then
                    if sed -i '' "1s|#!/usr/bin/env ${version}|#!/usr/bin/env python${TARGET_VERSION}|" "$file" 2>/dev/null; then
                        echo -e "  ${GREEN}✓ FIXED${NC} ${file}:1 - shebang ${version} → python${TARGET_VERSION}"
                        ((FIXED_COUNT++))
                    else
                        echo -e "  ${RED}✗ FAILED${NC} ${file}:1 - could not update shebang"
                        ((FAILED_COUNT++))
                    fi
                fi
            fi
        done < <(find . -name "*.py" -type f ! -path "./.venv/*" ! -path "./venv/*" ! -path "*/node_modules/*" ! -path "*/__pycache__/*" -print0 2>/dev/null)

        echo ""
        echo -e "${BLUE}=== Fix Summary ===${NC}"
        echo -e "Fixed: ${GREEN}${FIXED_COUNT}${NC} reference(s)"
        if [ $FAILED_COUNT -gt 0 ]; then
            echo -e "Failed: ${RED}${FAILED_COUNT}${NC} reference(s)"
            echo ""
            echo -e "${YELLOW}Some fixes failed. Please review and fix manually.${NC}"
            exit 1
        else
            echo ""
            echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
            echo -e "${GREEN}✓ All version references updated to ${TARGET_VERSION}${NC}"
            echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
            exit 0
        fi
    else
        echo -e "${YELLOW}Run with --fix to automatically update references${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}✓ SUCCESS: All Python references use version ${TARGET_VERSION}${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    exit 0
fi
