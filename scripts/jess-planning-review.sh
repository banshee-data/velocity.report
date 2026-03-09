#!/usr/bin/env bash

set -euo pipefail

SINCE_DAYS=7

usage() {
  cat <<'EOF'
Usage: scripts/jess-planning-review.sh [--since-days N]

Generate a Markdown planning review snapshot for Jess covering:
- planning docs added or touched recently
- recent plans missing backlog or decision coverage
- missing plan references in BACKLOG.md and DECISIONS.md
- backlog sections that may need splitting
- backlog items missing supporting doc links
- plan docs with open-question markers
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --since-days)
      if [[ $# -lt 2 ]]; then
        echo "--since-days requires a value" >&2
        exit 1
      fi
      SINCE_DAYS="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

count_nonempty_lines() {
  awk 'NF { count++ } END { print count + 0 }'
}

print_path_list_with_commit() {
  local list_file="$1"

  if [[ ! -s "$list_file" ]]; then
    printf -- "- None.\n"
    return
  fi

  while IFS= read -r path; do
    [[ -z "$path" ]] && continue
    last_commit=$(git log -1 --date=short --format='%ad %h %s' -- "$path" 2>/dev/null || printf "unknown")
    printf -- '- `%s` - %s\n' "$path" "$last_commit"
  done <"$list_file"
}

print_plain_list() {
  local list_file="$1"
  local prefix="${2:-- }"

  if [[ ! -s "$list_file" ]]; then
    printf -- "%sNone.\n" "$prefix"
    return
  fi

  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    printf -- "%s%s\n" "$prefix" "$line"
  done <"$list_file"
}

print_backlog_gap_list() {
  local list_file="$1"
  local shown=0

  if [[ ! -s "$list_file" ]]; then
    printf -- "- None.\n"
    return
  fi

  while IFS=$'\t' read -r section item; do
    [[ -z "$section" ]] && continue
    item="${item#- }"
    printf -- '- `%s` - %s\n' "$section" "$item"
    shown=$((shown + 1))
    if (( shown >= 10 )); then
      break
    fi
  done <"$list_file"
}

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

ALL_PLANS="$TMP_DIR/all_plans.txt"
TOUCHED_PLANS="$TMP_DIR/touched_plans.txt"
NEW_PLANS="$TMP_DIR/new_plans.txt"
TOUCHED_MISSING_BACKLOG="$TMP_DIR/touched_missing_backlog.txt"
TOUCHED_MISSING_DECISIONS="$TMP_DIR/touched_missing_decisions.txt"
MISSING_BACKLOG_PLAN_REFS="$TMP_DIR/missing_backlog_plan_refs.txt"
MISSING_DECISION_PLAN_REFS="$TMP_DIR/missing_decision_plan_refs.txt"
BACKLOG_MISSING_LINKS="$TMP_DIR/backlog_missing_links.txt"
SECTION_COUNTS="$TMP_DIR/section_counts.txt"
SPLIT_CANDIDATES="$TMP_DIR/split_candidates.txt"
DECISION_MARKERS="$TMP_DIR/decision_markers.txt"

touch \
  "$TOUCHED_PLANS" \
  "$NEW_PLANS" \
  "$TOUCHED_MISSING_BACKLOG" \
  "$TOUCHED_MISSING_DECISIONS" \
  "$MISSING_BACKLOG_PLAN_REFS" \
  "$MISSING_DECISION_PLAN_REFS" \
  "$BACKLOG_MISSING_LINKS" \
  "$SECTION_COUNTS" \
  "$SPLIT_CANDIDATES" \
  "$DECISION_MARKERS"

REPO_ROOT=$(git rev-parse --show-toplevel)
BACKLOG_FILE="$REPO_ROOT/docs/BACKLOG.md"
DECISIONS_FILE="$REPO_ROOT/docs/DECISIONS.md"

find "$REPO_ROOT/docs/plans" -maxdepth 1 -type f -name '*.md' | sort >"$ALL_PLANS"
git log --since="${SINCE_DAYS} days ago" --name-only --pretty=format: -- docs/plans 2>/dev/null \
  | grep '^docs/plans/.*\.md$' \
  | sort -u >"$TOUCHED_PLANS" || true
git log --since="${SINCE_DAYS} days ago" --diff-filter=A --name-only --pretty=format: -- docs/plans 2>/dev/null \
  | grep '^docs/plans/.*\.md$' \
  | sort -u >"$NEW_PLANS" || true

while IFS= read -r plan_path; do
  [[ -z "$plan_path" ]] && continue
  plan_base=$(basename "$plan_path")
  if ! grep -Fq "$plan_base" "$BACKLOG_FILE"; then
    printf "%s\n" "$plan_path" >>"$TOUCHED_MISSING_BACKLOG"
  fi
  if ! grep -Fq "$plan_base" "$DECISIONS_FILE"; then
    printf "%s\n" "$plan_path" >>"$TOUCHED_MISSING_DECISIONS"
  fi
done <"$TOUCHED_PLANS"

grep -oE 'plans/[^)#[:space:]]+\.md' "$BACKLOG_FILE" | sort -u | while IFS= read -r rel_path; do
  [[ -z "$rel_path" ]] && continue
  if [[ ! -f "$REPO_ROOT/docs/$rel_path" ]]; then
    printf "%s\n" "$rel_path" >>"$MISSING_BACKLOG_PLAN_REFS"
  fi
done

grep -oE 'plans/[^)#[:space:]]+\.md' "$DECISIONS_FILE" | sort -u | while IFS= read -r rel_path; do
  [[ -z "$rel_path" ]] && continue
  if [[ ! -f "$REPO_ROOT/docs/$rel_path" ]]; then
    printf "%s\n" "$rel_path" >>"$MISSING_DECISION_PLAN_REFS"
  fi
done

awk '
  /^## / {
    section = substr($0, 4)
    next
  }
  /^- / && section != "Complete" && section !~ /waybacklog/ {
    if ($0 !~ /\[[^]]+\]\([^)]+\.md([#][^)]+)?\)/) {
      print section "\t" $0
    }
  }
' "$BACKLOG_FILE" >"$BACKLOG_MISSING_LINKS"

awk '
  /^## / {
    if (section != "") {
      print section "\t" count
    }
    section = substr($0, 4)
    count = 0
    next
  }
  /^- / && section != "" {
    count++
  }
  END {
    if (section != "") {
      print section "\t" count
    }
  }
' "$BACKLOG_FILE" >"$SECTION_COUNTS"

awk -F '\t' '
  $1 != "Complete" &&
  $1 !~ /waybacklog/ &&
  $2 > 12 {
    print $0
  }
' "$SECTION_COUNTS" >"$SPLIT_CANDIDATES"

grep -RniE '(^#+ .*Open Questions)|\bTBD\b|\bTODO\b|\bFIXME\b|decision needed|unresolved|open question' "$REPO_ROOT/docs/plans" >"$DECISION_MARKERS" || true

TOTAL_PLAN_COUNT=$(wc -l <"$ALL_PLANS" | tr -d ' ')
TOUCHED_PLAN_COUNT=$(wc -l <"$TOUCHED_PLANS" | tr -d ' ')
NEW_PLAN_COUNT=$(wc -l <"$NEW_PLANS" | tr -d ' ')
BACKLOG_GAP_COUNT=$(wc -l <"$TOUCHED_MISSING_BACKLOG" 2>/dev/null | tr -d ' ' || printf "0")
DECISION_GAP_COUNT=$(wc -l <"$TOUCHED_MISSING_DECISIONS" 2>/dev/null | tr -d ' ' || printf "0")
BACKLOG_LINK_GAP_COUNT=$(wc -l <"$BACKLOG_MISSING_LINKS" | tr -d ' ')
SPLIT_CANDIDATE_COUNT=$(wc -l <"$SPLIT_CANDIDATES" | tr -d ' ')
DECISION_MARKER_COUNT=$(wc -l <"$DECISION_MARKERS" | tr -d ' ')

printf "# Jess Weekly Planning Review Snapshot\n\n"
printf -- "- Generated: %s\n" "$(date '+%Y-%m-%d %H:%M:%S %Z')"
printf -- '- Repo: `%s`\n' "$REPO_ROOT"
printf -- "- Review window: last %s day(s)\n" "$SINCE_DAYS"
printf -- '- Planning docs in `docs/plans/`: %s\n\n' "$TOTAL_PLAN_COUNT"

printf "## Recent Planning Docs\n\n"
printf -- "- New plan docs in window: %s\n" "$NEW_PLAN_COUNT"
printf -- "- Touched plan docs in window: %s\n\n" "$TOUCHED_PLAN_COUNT"

printf "### New Plan Docs\n\n"
print_path_list_with_commit "$NEW_PLANS"
printf "\n"

printf "### Touched Plan Docs\n\n"
print_path_list_with_commit "$TOUCHED_PLANS"
printf "\n"

printf "## Consistency Review\n\n"
printf -- "- Recent plans missing backlog mention: %s\n" "$BACKLOG_GAP_COUNT"
printf -- "- Recent plans missing decision-register mention: %s\n" "$DECISION_GAP_COUNT"
printf -- "- Active backlog items missing a supporting doc link: %s\n\n" "$BACKLOG_LINK_GAP_COUNT"

printf "### Recent Plans Missing Backlog Coverage\n\n"
print_plain_list "$TOUCHED_MISSING_BACKLOG" "- "
printf "\n"

printf "### Recent Plans Missing Decision Coverage\n\n"
print_plain_list "$TOUCHED_MISSING_DECISIONS" "- "
printf "\n"

printf "### Missing Plan References\n\n"
printf "Backlog:\n"
print_plain_list "$MISSING_BACKLOG_PLAN_REFS" "- "
printf "\nDecisions:\n"
print_plain_list "$MISSING_DECISION_PLAN_REFS" "- "
printf "\n"

printf "### Backlog Items Missing Supporting Doc Links\n\n"
print_backlog_gap_list "$BACKLOG_MISSING_LINKS"
if (( BACKLOG_LINK_GAP_COUNT > 10 )); then
  printf -- "- ... and %s more.\n" "$((BACKLOG_LINK_GAP_COUNT - 10))"
fi
printf "\n"

printf "## Timeline And Backlog Load\n\n"
printf 'Sections above 12 items are split candidates under `docs/DECISIONS.md` milestone guidance.\n\n'
printf "| Backlog section | Items |\n"
printf "| --- | ---: |\n"
while IFS=$'\t' read -r section count; do
  [[ -z "$section" ]] && continue
  if [[ "$section" == "Complete" || "$section" == *"waybacklog"* ]]; then
    continue
  fi
  printf "| %s | %s |\n" "$section" "$count"
done <"$SECTION_COUNTS"
printf "\n"

printf "### Split Candidates\n\n"
if [[ -s "$SPLIT_CANDIDATES" ]]; then
  while IFS=$'\t' read -r section count; do
    [[ -z "$section" ]] && continue
    printf -- '- `%s` has %s items and should be reviewed for a new backlog section or milestone split.\n' "$section" "$count"
  done <"$SPLIT_CANDIDATES"
else
  printf -- "- None.\n"
fi
printf "\n"

printf "## Decision Signals\n\n"
printf -- '- Open-question markers in `docs/plans/`: %s\n\n' "$DECISION_MARKER_COUNT"
if [[ -s "$DECISION_MARKERS" ]]; then
  sed -n '1,20p' "$DECISION_MARKERS" | while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    line="${line#$REPO_ROOT/}"
    printf -- '- `%s`\n' "$line"
  done
  if (( DECISION_MARKER_COUNT > 20 )); then
    printf -- "- ... and %s more.\n" "$((DECISION_MARKER_COUNT - 20))"
  fi
else
  printf -- "- None.\n"
fi
