#!/usr/bin/env bash

set -euo pipefail

# Agent Drift Detection
# Compares paired agent definitions between Copilot (.github/agents/*.agent.md)
# and Claude (.claude/agents/*.md) to surface unreviewed content drift.
#
# Extracts persona-relevant sections (stripping YAML frontmatter and
# platform-specific directives) and reports:
#   - Missing pairs (agent exists in one platform only)
#   - Content drift (normalised diff of shared persona sections)
#   - Summary statistics for weekly review
#
# Usage: scripts/check-agent-drift.sh [--verbose] [--diff]

VERBOSE=false
SHOW_DIFF=false

usage() {
  cat <<'EOF'
Usage: scripts/check-agent-drift.sh [--verbose] [--diff]

Compare agent definitions between Copilot and Claude platforms.

Options:
  --verbose   Show per-agent comparison details even when aligned
  --diff      Show normalised diff output for drifted agents
  -h, --help  Show this help

Output: Markdown summary suitable for weekly planning review.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --verbose) VERBOSE=true; shift ;;
    --diff)    SHOW_DIFF=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
COPILOT_DIR="$REPO_ROOT/.github/agents"
CLAUDE_DIR="$REPO_ROOT/.claude/agents"

# --- Temp directory ---

make_temp_dir() {
  if TMP_DIR=$(mktemp -d 2>/dev/null); then
    return 0
  fi
  if TMP_DIR=$(mktemp -d -t velocity-report-agent-drift 2>/dev/null); then
    return 0
  fi
  echo "Failed to create temporary directory" >&2
  exit 1
}

make_temp_dir
trap 'rm -rf "$TMP_DIR"' EXIT

# --- Extract persona content ---

# Strip YAML frontmatter and platform-specific lines from a Copilot .agent.md
# Removes: YAML frontmatter block and lines starting with "tools:"
extract_copilot_persona() {
  local file="$1"
  awk '
    BEGIN { in_yaml = 0; yaml_done = 0 }
    /^---[[:space:]]*$/ {
      if (!yaml_done) {
        if (in_yaml) { yaml_done = 1 }
        else         { in_yaml = 1 }
        next
      }
    }
    in_yaml && !yaml_done { next }
    /^[[:space:]]*tools:[[:space:]]*/ { next }
    { print }
  ' "$file"
}

# Strip platform-specific Claude directives (if any emerge in future)
extract_claude_persona() {
  local file="$1"
  cat "$file"
}

# Normalise text for comparison: lowercase, collapse whitespace, strip blank lines
normalise() {
  tr '[:upper:]' '[:lower:]' \
    | sed 's/[[:space:]]\{1,\}/ /g' \
    | sed 's/^[[:space:]]*//' \
    | sed 's/[[:space:]]*$//' \
    | grep -v '^$' \
    || true
}

# --- Collect agent names ---

COPILOT_AGENTS="$TMP_DIR/copilot_agents.txt"
CLAUDE_AGENTS="$TMP_DIR/claude_agents.txt"
ALL_AGENTS="$TMP_DIR/all_agents.txt"
touch "$COPILOT_AGENTS" "$CLAUDE_AGENTS" "$ALL_AGENTS"

if [[ -d "$COPILOT_DIR" ]]; then
  find "$COPILOT_DIR" -maxdepth 1 -name '*.agent.md' -type f 2>/dev/null \
    | while IFS= read -r f; do
        basename "$f" .agent.md
      done \
    | sort > "$COPILOT_AGENTS"
fi

if [[ -d "$CLAUDE_DIR" ]]; then
  find "$CLAUDE_DIR" -maxdepth 1 -name '*.md' -type f 2>/dev/null \
    | while IFS= read -r f; do
        basename "$f" .md
      done \
    | sort > "$CLAUDE_AGENTS"
fi

sort -u "$COPILOT_AGENTS" "$CLAUDE_AGENTS" > "$ALL_AGENTS"

TOTAL=$(wc -l < "$ALL_AGENTS" | tr -d ' ')

if (( TOTAL == 0 )); then
  printf "# Agent Drift Report\n\n"
  printf "No agent definitions found in \`%s\` or \`%s\`.\n" "$COPILOT_DIR" "$CLAUDE_DIR"
  exit 0
fi

# --- Compare ---

MISSING_CLAUDE=0
MISSING_COPILOT=0
ALIGNED=0
DRIFTED=0
DRIFTED_NAMES=""

while IFS= read -r agent; do
  [[ -z "$agent" ]] && continue

  copilot_file="$COPILOT_DIR/${agent}.agent.md"
  claude_file="$CLAUDE_DIR/${agent}.md"

  has_copilot=false
  has_claude=false
  [[ -f "$copilot_file" ]] && has_copilot=true
  [[ -f "$claude_file" ]]  && has_claude=true

  if $has_copilot && ! $has_claude; then
    MISSING_CLAUDE=$((MISSING_CLAUDE + 1))
    continue
  fi

  if ! $has_copilot && $has_claude; then
    MISSING_COPILOT=$((MISSING_COPILOT + 1))
    continue
  fi

  # Both exist — compare normalised persona content
  copilot_norm="$TMP_DIR/${agent}_copilot.norm"
  claude_norm="$TMP_DIR/${agent}_claude.norm"

  extract_copilot_persona "$copilot_file" | normalise > "$copilot_norm"
  extract_claude_persona "$claude_file"   | normalise > "$claude_norm"

  if diff -q "$copilot_norm" "$claude_norm" >/dev/null 2>&1; then
    ALIGNED=$((ALIGNED + 1))
  else
    DRIFTED=$((DRIFTED + 1))
    if [[ -n "$DRIFTED_NAMES" ]]; then
      DRIFTED_NAMES="$DRIFTED_NAMES $agent"
    else
      DRIFTED_NAMES="$agent"
    fi
  fi
done < "$ALL_AGENTS"

# --- Output ---

PAIRED=$((ALIGNED + DRIFTED))

printf "# Agent Drift Report\n\n"
printf -- "- Generated: %s\n" "$(date '+%Y-%m-%d %H:%M:%S %Z')"
printf -- '- Copilot agents dir: `%s`\n' ".github/agents/"
printf -- '- Claude agents dir: `%s`\n\n' ".claude/agents/"

printf "## Summary\n\n"
printf "| Metric | Count |\n"
printf "|--------|-------|\n"
printf "| Total unique agents | %d |\n" "$TOTAL"
printf "| Paired (both platforms) | %d |\n" "$PAIRED"
printf "| Aligned (no drift) | %d |\n" "$ALIGNED"
printf "| **Drifted** | **%d** |\n" "$DRIFTED"
printf "| Missing Claude definition | %d |\n" "$MISSING_CLAUDE"
printf "| Missing Copilot definition | %d |\n\n" "$MISSING_COPILOT"

# --- Missing pairs ---

if (( MISSING_CLAUDE > 0 )); then
  printf "## Missing Claude Definitions\n\n"
  printf "These agents exist in Copilot but have no corresponding \`.claude/agents/\` file:\n\n"
  while IFS= read -r agent; do
    [[ -z "$agent" ]] && continue
    if [[ -f "$COPILOT_DIR/${agent}.agent.md" ]] && [[ ! -f "$CLAUDE_DIR/${agent}.md" ]]; then
      printf -- "- \`%s\`\n" "$agent"
    fi
  done < "$ALL_AGENTS"
  printf "\n"
fi

if (( MISSING_COPILOT > 0 )); then
  printf "## Missing Copilot Definitions\n\n"
  printf "These agents exist in Claude but have no corresponding \`.github/agents/\` file:\n\n"
  while IFS= read -r agent; do
    [[ -z "$agent" ]] && continue
    if [[ ! -f "$COPILOT_DIR/${agent}.agent.md" ]] && [[ -f "$CLAUDE_DIR/${agent}.md" ]]; then
      printf -- "- \`%s\`\n" "$agent"
    fi
  done < "$ALL_AGENTS"
  printf "\n"
fi

# --- Drifted agents ---

if (( DRIFTED > 0 )); then
  printf "## Drifted Agents\n\n"
  printf "These agents have paired definitions that differ after normalisation:\n\n"
  for agent in $DRIFTED_NAMES; do
    copilot_norm="$TMP_DIR/${agent}_copilot.norm"
    claude_norm="$TMP_DIR/${agent}_claude.norm"

    copilot_lines=$(wc -l < "$copilot_norm" | tr -d ' ')
    claude_lines=$(wc -l < "$claude_norm" | tr -d ' ')

    printf -- "### %s\n\n" "$agent"
    printf -- "- Copilot: \`%s\` (%s lines normalised)\n" ".github/agents/${agent}.agent.md" "$copilot_lines"
    printf -- "- Claude: \`%s\` (%s lines normalised)\n" ".claude/agents/${agent}.md" "$claude_lines"

    if $SHOW_DIFF; then
      printf "\n\`\`\`diff\n"
      diff -u "$copilot_norm" "$claude_norm" \
        --label "copilot/${agent}" \
        --label "claude/${agent}" \
        || true
      printf "\`\`\`\n"
    fi
    printf "\n"
  done
fi

# --- Aligned agents (verbose) ---

if $VERBOSE && (( ALIGNED > 0 )); then
  printf "## Aligned Agents\n\n"
  while IFS= read -r agent; do
    [[ -z "$agent" ]] && continue
    if [[ -f "$COPILOT_DIR/${agent}.agent.md" ]] && [[ -f "$CLAUDE_DIR/${agent}.md" ]]; then
      copilot_norm="$TMP_DIR/${agent}_copilot.norm"
      claude_norm="$TMP_DIR/${agent}_claude.norm"
      if diff -q "$copilot_norm" "$claude_norm" >/dev/null 2>&1; then
        printf -- "- \`%s\` ✅\n" "$agent"
      fi
    fi
  done < "$ALL_AGENTS"
  printf "\n"
fi

# --- Health verdict ---

printf "## Health\n\n"
if (( DRIFTED == 0 && MISSING_CLAUDE == 0 && MISSING_COPILOT == 0 )); then
  printf "✅ **All agent definitions are aligned.** No action required.\n"
elif (( DRIFTED == 0 )); then
  printf "⚠️ **No content drift**, but %d agent(s) missing paired definitions. Review missing pairs above.\n" "$((MISSING_CLAUDE + MISSING_COPILOT))"
else
  printf "🔴 **%d agent(s) have drifted.** Review differences above and sync persona content.\n" "$DRIFTED"
  printf "Run \`scripts/check-agent-drift.sh --diff\` to see normalised diffs.\n"
fi
