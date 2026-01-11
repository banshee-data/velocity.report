#!/bin/bash
##############################################################################
# analyse-branches.sh
#
# Analyses local git branches and extracts [tag] prefixes from commit logs.
# Generates a report of branch prefixes and their frequencies.
#
# Optional: Fetch deleted branches from remote PRs (GitHub API).
#
# Usage:
#   ./scripts/analyse-branches.sh [--include-remote-prs]
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
TEMP_NO_TAGS=$(mktemp)
TEMP_TAGGED_COMMITS=$(mktemp)

trap "rm -f $TEMP_PREFIXES $TEMP_NO_TAGS $TEMP_TAGGED_COMMITS" EXIT

echo "=========================================="
echo "Git Branch Analysis"
echo "=========================================="
echo "Timestamp: $(date)"
echo "Repository: $(git rev-parse --show-toplevel)"
echo "Current branch: $(git rev-parse --abbrev-ref HEAD)"
echo ""

# Get all local branches (remove leading * for current branch)
branches=$(git branch --list | sed 's/^\* //' | sed 's/^  //')
if [ -z "$branches" ]; then
  echo "No local branches found."
  exit 0
fi

{
  echo "=========================================="
  echo "LOCAL BRANCH ANALYSIS"
  echo "=========================================="
  echo ""

  total_branches=$(echo "$branches" | wc -l)
  echo "Total local branches: $total_branches"
  echo ""

  # Analyse each branch
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
      while IFS= read -r line; do
        echo "  $line"

        # Extract all [TAG]s from the commit message
        tags=$(echo "$line" | grep -oE '\[[a-zA-Z0-9_-]+\]' || true)
        if [ -n "$tags" ]; then
          # Track this commit as having at least one tag
          echo "1" >> "$TEMP_TAGGED_COMMITS"
          for tag in $tags; do
            # Remove brackets
            clean_tag=$(echo "$tag" | sed 's/^\[\(.*\)\]$/\1/')
            echo "$clean_tag" >> "$TEMP_PREFIXES"
          done
        else
          # Track commits without tags
          echo "no-tag" >> "$TEMP_NO_TAGS"
        fi
      done < <(git log --oneline "$branch" --not "$main_ref" 2>/dev/null)
    else
      echo "No unique commits (branch matches $main_ref)"
    fi

    echo ""
  done <<< "$branches"
} > "$ANALYSIS_LOG"

echo "Analysed $total_branches branches → $ANALYSIS_LOG"

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

    # Add [NO TAG] count to the table using TEMP_NO_TAGS
    tag_occurrences=$(wc -l < "$TEMP_PREFIXES" 2>/dev/null || echo "0")
    tagged_commits=$(wc -l < "$TEMP_TAGGED_COMMITS" 2>/dev/null || echo "0")
    untagged_commits=$(wc -l < "$TEMP_NO_TAGS" 2>/dev/null || echo "0")
    total_commits=$((tagged_commits + untagged_commits))

    if [ "$total_commits" -gt 0 ]; then
      printf "  %5d | [NO TAG]\n" "$untagged_commits"
      # Calculate percentages with two decimal places using awk
      tagged_pct=$(awk "BEGIN {printf \"%.2f\", ($tagged_commits / $total_commits) * 100}")
      untagged_pct=$(awk "BEGIN {printf \"%.2f\", ($untagged_commits / $total_commits) * 100}")
      echo ""
      echo "Total tag occurrences: $tag_occurrences"
      echo "Unique tag types: $(sort -u "$TEMP_PREFIXES" | wc -l)"
      echo "Commits with tags: $tagged_commits"
      echo "Commits without tags: $untagged_commits"
      echo "Total commits analysed: $total_commits"
      echo "Distribution: $tagged_pct% with tag / $untagged_pct% without tag"
    fi
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
    owner=$(echo "$origin_url" | sed -E 's|.*[:/]{1}([^/]+)/[^/]+(\.git)?$|\1|')
    repo=$(echo "$origin_url" | sed -E 's|.*[:/]{1}[^/]+/([^/]+)(\.git)?$|\1|;s|.*[:/]{1}([^/]+)(\.git)?$|\1|')

    if [ -n "$owner" ] && [ -n "$repo" ] && [[ "$origin_url" == *"github.com"* ]]
    then
      echo "Detected: github.com/$owner/$repo"
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
