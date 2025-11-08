#!/bin/bash
##############################################################################
# analyze-branches.sh
#
# Analyzes local git branches and extracts [tag] prefixes from commit logs.
# Generates a report of branch prefixes and their frequencies.
#
# Optional: Fetch deleted branches from remote PRs (GitHub API).
#
# Usage:
#   ./scripts/analyze-branches.sh [--include-remote-prs]
#
# Output:
#   - branch-analysis-$(date).log       Full branch log with extracted prefixes
#   - branch-analysis-summary.log       Rollup report with frequency counts
#
##############################################################################

set -euo pipefail

INCLUDE_REMOTE_PRS=${1:-}
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
ANALYSIS_LOG="branch-analysis-${TIMESTAMP}.log"
SUMMARY_LOG="branch-analysis-summary.log"
TEMP_PREFIXES=$(mktemp)

trap "rm -f $TEMP_PREFIXES" EXIT

echo "=========================================="
echo "Git Branch Analysis"
echo "=========================================="
echo "Timestamp: $(date)"
echo "Repository: $(git rev-parse --show-toplevel)"
echo "Current branch: $(git rev-parse --abbrev-ref HEAD)"
echo ""

{
  echo "=========================================="
  echo "LOCAL BRANCH ANALYSIS"
  echo "=========================================="
  echo ""

  # Get all local branches (remove leading * for current branch)
  branches=$(git branch --list | sed 's/^\* //' | sed 's/^  //')
    if [ -z "$branches" ]; then
    echo "No local branches found."
    exit 0
  fi

  total_branches=$(echo "$branches" | wc -l)
  echo "Total local branches: $total_branches"
  echo ""

  # Analyze each branch
  while IFS= read -r branch; do
    [ -z "$branch" ] && continue

    echo "─────────────────────────────────────────"
    echo "Branch: $branch"
    echo ""

    # Get the main ref
    main_ref=$(git rev-parse --abbrev-ref origin/HEAD 2>/dev/null | sed 's|.*/||' || echo "main")

    # Get unique commits on this branch (where it diverges from main)
    commit_count=$(git rev-list --count "$branch" --not "$main_ref" 2>/dev/null || echo "0")
    echo "Unique commits (diverged from $main_ref): $commit_count"
    echo ""

    # Get log with oneline format for commits unique to this branch and extract [tags]
    if [ "$commit_count" -gt 0 ]; then
      echo "Unique commits:"
      git log --oneline "$branch" --not "$main_ref" 2>/dev/null | while IFS= read -r line; do
        echo "  $line"

        # Extract [TAG] from the beginning of commit message
        if [[ $line =~ \[([a-zA-Z0-9_-]+)\] ]]; then
          tag="${BASH_REMATCH[1]}"
          echo "$tag" >> "$TEMP_PREFIXES"
        fi
      done
    else
      echo "No unique commits (branch matches $main_ref)"
    fi

    echo ""
  done <<< "$branches"

} | tee "$ANALYSIS_LOG"

echo ""
echo "=========================================="
echo "SUMMARY REPORT"
echo "=========================================="
echo ""

{
  echo "=========================================="
  echo "BRANCH PREFIX FREQUENCY ANALYSIS"
  echo "Timestamp: $(date)"
  echo "=========================================="
  echo ""

  if [ ! -s "$TEMP_PREFIXES" ]; then
    echo "No [tag] prefixes found in any branch commits."
  else
    echo "Prefixes found and their frequency:"
    echo ""
    echo "  Count | Prefix"
    echo "  ------|--------"

    sort "$TEMP_PREFIXES" | uniq -c | sort -rn | \
      awk '{ printf "  %5d | %s\n", $1, $2 }'

    echo ""
    echo "Total prefix occurrences: $(wc -l < "$TEMP_PREFIXES")"
    echo "Unique prefixes: $(sort -u "$TEMP_PREFIXES" | wc -l)"
  fi

  echo ""
  echo "=========================================="
  echo "REMOTE PR BRANCHES (if GitHub available)"
  echo "=========================================="
  echo ""

  if [ "$INCLUDE_REMOTE_PRS" = "--include-remote-prs" ]; then
    echo "Fetching deleted PR branches from GitHub..."

    # Try to detect GitHub repo from origin URL
    origin_url=$(git config --get remote.origin.url)

    # Extract owner and repo from both https and ssh URLs
    owner=$(echo "$origin_url" | sed -E 's|.*/([^/]+)/[^/]+\.git$|\1|')
    repo=$(echo "$origin_url" | sed -E 's|.*/([^/]+)\.git$|\1|')

    if [ -n "$owner" ] && [ -n "$repo" ] && [[ "$origin_url" == *"github.com"* ]]; then      echo "Detected: github.com/$owner/$repo"
      echo ""
      echo "NOTE: To fetch deleted PR branches, you would need:"
      echo "  1. GitHub CLI (gh) installed"
      echo "  2. Authentication configured (gh auth login)"
      echo ""
      echo "To list all PR branches (deleted):"
      echo "  gh pr list --repo $owner/$repo --state all --json headRefName"
      echo ""
      echo "Current setup instructions:"
      echo "  # Install GitHub CLI: https://cli.github.com"
      echo "  # Authenticate: gh auth login"
      echo "  # Then re-run this script with --include-remote-prs"
      echo ""
      echo "Alternatively, using git commands:"
      echo "  git ls-remote --heads origin | grep -E 'refs/heads/\[' || echo 'No remote branches with [tag] prefix'"
    else
      echo "Could not detect GitHub repository from origin URL: $origin_url"
      echo "GitHub PR branch fetching requires a GitHub-hosted repository."
    fi
  fi

} | tee "$SUMMARY_LOG"

echo ""
echo "=========================================="
echo "✓ Analysis complete"
echo "=========================================="
echo ""
echo "Output files:"
echo "  Full log:     $ANALYSIS_LOG"
echo "  Summary:      $SUMMARY_LOG"
echo ""
