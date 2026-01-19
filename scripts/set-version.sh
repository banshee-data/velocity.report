#!/usr/bin/env bash
# set-version.sh - Update version numbers across the velocity.report codebase
# Usage: ./scripts/set-version.sh <version> [--all|--makefile|--deploy|--web|--docs]

set -euo pipefail

VERSION=""
UPDATE_MAKEFILE=0
UPDATE_WEB=0
UPDATE_DOCS=0
UPDATE_PDF=0

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Print usage information
usage() {
    cat <<EOF
Usage: $0 <version> [targets...]

Update version strings across the codebase.

Arguments:
  <version>     Version string (e.g., 0.4.0-pre2, 1.0.0, 0.5.0-rc1)

Targets (default: --all):
  --all         Update all version references
  --makefile    Update Makefile VERSION variable (affects Go binaries)
  --web         Update web/package.json version
  --docs        Update docs/package.json version
  --pdf         Update tools/pdf-generator/pyproject.toml version

Examples:
  # Update all version references
  $0 0.4.0-pre2 --all

  # Update only Go-related versions (Makefile)
  $0 0.4.0-pre2 --makefile

  # Update only web frontend
  $0 0.5.0 --web

  # Update only PDF generator
  $0 0.5.0 --pdf

EOF
    exit 1
}

# Print colored message
log_info() {
    echo -e "${GREEN}✓${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

# Validate version format (basic semver check)
validate_version() {
    local ver="$1"
    # Match: X.Y.Z, X.Y.Z-preN, X.Y.Z-rcN, X.Y.Z-betaN, X.Y.Z-alphaN
    if [[ ! "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$ ]]; then
        log_error "Invalid version format: $ver"
        log_error "Expected format: X.Y.Z or X.Y.Z-suffix (e.g., 0.4.0, 1.0.0-pre2, 0.5.0-rc1)"
        exit 1
    fi
}

# Update Makefile
update_makefile() {
    local file="Makefile"
    local old_version

    if [[ ! -f "$file" ]]; then
        log_error "$file not found"
        return 1
    fi

    old_version=$(grep -E '^VERSION := ' "$file" | sed 's/VERSION := //')

    if [[ -z "$old_version" ]]; then
        log_error "Could not find VERSION in $file"
        return 1
    fi

    # Use sed to replace the version
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS sed requires -i with backup extension
        sed -i '' "s/^VERSION := .*/VERSION := $VERSION/" "$file"
    else
        # Linux sed
        sed -i "s/^VERSION := .*/VERSION := $VERSION/" "$file"
    fi

    log_info "Updated $file: $old_version → $VERSION"
}

# Update web/package.json
update_web() {
    local file="web/package.json"
    local old_version

    if [[ ! -f "$file" ]]; then
        log_error "$file not found"
        return 1
    fi

    old_version=$(grep -E '^\s*"version":' "$file" | sed 's/.*"version": "\(.*\)".*/\1/')

    if [[ -z "$old_version" ]]; then
        log_error "Could not find version in $file"
        return 1
    fi

    # Use sed to replace the version
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "s/\"version\": \".*\"/\"version\": \"$VERSION\"/" "$file"
    else
        sed -i "s/\"version\": \".*\"/\"version\": \"$VERSION\"/" "$file"
    fi

    log_info "Updated $file: $old_version → $VERSION"
}

# Update docs/package.json
update_docs() {
    local file="docs/package.json"
    local old_version

    if [[ ! -f "$file" ]]; then
        log_warn "$file not found, skipping"
        return 0
    fi

    old_version=$(grep -E '^\s*"version":' "$file" | sed 's/.*"version": "\(.*\)".*/\1/')

    if [[ -z "$old_version" ]]; then
        log_error "Could not find version in $file"
        return 1
    fi

    # Use sed to replace the version
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "s/\"version\": \".*\"/\"version\": \"$VERSION\"/" "$file"
    else
        sed -i "s/\"version\": \".*\"/\"version\": \"$VERSION\"/" "$file"
    fi

    log_info "Updated $file: $old_version → $VERSION"
}

# Update tools/pdf-generator/pyproject.toml
update_pdf() {
    local file="tools/pdf-generator/pyproject.toml"
    local old_version

    if [[ ! -f "$file" ]]; then
        log_warn "$file not found, skipping"
        return 0
    fi

    # pyproject.toml format: version = "0.2.0"
    old_version=$(grep -E '^version = ' "$file" | sed 's/version = "\(.*\)"/\1/')

    if [[ -z "$old_version" ]]; then
        log_error "Could not find version in $file"
        return 1
    fi

    # Use sed to replace the version
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "s/^version = \".*\"/version = \"$VERSION\"/" "$file"
    else
        sed -i "s/^version = \".*\"/version = \"$VERSION\"/" "$file"
    fi

    log_info "Updated $file: $old_version → $VERSION"
}

# Parse arguments
if [[ $# -lt 1 ]]; then
    usage
fi

VERSION="$1"
shift

# Validate version format
validate_version "$VERSION"

# Parse targets
if [[ $# -eq 0 ]]; then
    # No targets specified, show usage
    usage
fi

for arg in "$@"; do
    case "$arg" in
        --all)
            UPDATE_MAKEFILE=1
            UPDATE_WEB=1
            UPDATE_DOCS=1
            UPDATE_PDF=1
            ;;
        --makefile)
            UPDATE_MAKEFILE=1
            ;;
        --web)
            UPDATE_WEB=1
            ;;
        --docs)
            UPDATE_DOCS=1
            ;;
        --pdf)
            UPDATE_PDF=1
            ;;
        *)
            log_error "Unknown target: $arg"
            usage
            ;;
    esac
done

# Check if at least one target is selected
if [[ $UPDATE_MAKEFILE -eq 0 && $UPDATE_WEB -eq 0 && $UPDATE_DOCS -eq 0 && $UPDATE_PDF -eq 0 ]]; then
    log_error "No targets specified"
    usage
fi

# Perform updates
echo "Updating version to: $VERSION"
echo ""

EXIT_CODE=0
[[ $UPDATE_MAKEFILE -eq 1 ]] && update_makefile || EXIT_CODE=$?
[[ $UPDATE_WEB -eq 1 ]] && update_web || EXIT_CODE=$?
[[ $UPDATE_DOCS -eq 1 ]] && update_docs || EXIT_CODE=$?
[[ $UPDATE_PDF -eq 1 ]] && update_pdf || EXIT_CODE=$?

echo ""
log_info "Version update complete!"
log_warn "Remember to commit these changes and tag the release if appropriate"

exit $EXIT_CODE
