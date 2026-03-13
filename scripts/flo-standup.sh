#!/usr/bin/env bash

set -euo pipefail

ALL_BRANCHES=0

usage() {
  cat <<'EOF'
Usage: scripts/flo-standup.sh [--all-branches]

Generate a Markdown standup snapshot for Florence covering:
- current checkout state
- worktree topology
- local branch sync vs upstream and main
- backlog horizon

By default, the branch table shows checked-out or divergent branches.
Pass --all-branches to include every local branch.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all-branches)
      ALL_BRANCHES=1
      shift
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

join_lines() {
  awk 'NF { if (count++) printf(", "); printf("%s", $0) } END { if (!count) printf("-") }'
}

escape_markdown_table_cell() {
  local value="${1:-}"
  value=${value//\\/\\\\}
  value=${value//|/\\|}
  value=${value//$'\n'/ }
  value=${value//$'\r'/ }
  printf "%s" "$value"
}

shorten_path() {
  local path="${1:-}"
  if [[ -z "$path" ]]; then
    printf -- "-"
    return
  fi
  if [[ "$path" == "$HOME"* ]]; then
    printf "~%s" "${path#$HOME}"
    return
  fi
  printf "%s" "$path"
}

make_temp_file() {
  local temp_file=""

  if temp_file=$(mktemp 2>/dev/null); then
    printf "%s\n" "$temp_file"
    return 0
  fi

  if temp_file=$(mktemp -t velocity-report-flo-standup 2>/dev/null); then
    printf "%s\n" "$temp_file"
    return 0
  fi

  echo "Failed to create temporary file with mktemp" >&2
  exit 1
}

pick_main_ref() {
  if git show-ref --verify --quiet refs/remotes/origin/main; then
    printf "origin/main"
    return
  fi

  local origin_head=""
  origin_head=$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null || true)
  if [[ -n "$origin_head" ]]; then
    printf "%s" "$origin_head"
    return
  fi

  if git show-ref --verify --quiet refs/heads/main; then
    printf "main"
    return
  fi

  printf "HEAD"
}

worktree_path_for_branch() {
  local branch_name="$1"
  awk -v target="refs/heads/$branch_name" '
    /^worktree / { path = substr($0, 10) }
    /^branch / { branch = substr($0, 8) }
    /^$/ {
      if (branch == target) {
        found = 1
        print path
        exit
      }
      path = ""
      branch = ""
    }
    END {
      if (!found && branch == target && path != "") {
        print path
      }
    }
  ' <<<"$WORKTREE_PORCELAIN"
}

print_worktree_rows() {
  local path=""
  local head=""
  local branch_ref=""
  local detached=0
  local ref_label=""
  local subject=""
  local marker=""

  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ -z "$line" ]]; then
      if [[ -n "$path" ]]; then
        if [[ -n "$branch_ref" ]]; then
          ref_label="${branch_ref#refs/heads/}"
        elif [[ "$detached" -eq 1 ]]; then
          ref_label="detached"
        else
          ref_label="unknown"
        fi
        subject=$(git log -1 --pretty=%s "$head" 2>/dev/null || printf "unknown")
        marker=""
        if [[ "$path" == "$REPO_ROOT" ]]; then
          marker=" (current)"
        fi
        printf '| %s%s | %s | `%s` | %s |\n' \
          "$(escape_markdown_table_cell "$(shorten_path "$path")")" \
          "$marker" \
          "$(escape_markdown_table_cell "$ref_label")" \
          "${head:0:10}" \
          "$(escape_markdown_table_cell "$subject")"
      fi
      path=""
      head=""
      branch_ref=""
      detached=0
      continue
    fi

    case "$line" in
      worktree\ *)
        path="${line#worktree }"
        ;;
      HEAD\ *)
        head="${line#HEAD }"
        ;;
      branch\ *)
        branch_ref="${line#branch }"
        ;;
      detached)
        detached=1
        ;;
    esac
  done <<<"$WORKTREE_PORCELAIN"

  if [[ -n "$path" ]]; then
    if [[ -n "$branch_ref" ]]; then
      ref_label="${branch_ref#refs/heads/}"
    elif [[ "$detached" -eq 1 ]]; then
      ref_label="detached"
    else
      ref_label="unknown"
    fi
    subject=$(git log -1 --pretty=%s "$head" 2>/dev/null || printf "unknown")
    marker=""
    if [[ "$path" == "$REPO_ROOT" ]]; then
      marker=" (current)"
    fi
    printf '| %s%s | %s | `%s` | %s |\n' \
      "$(escape_markdown_table_cell "$(shorten_path "$path")")" \
      "$marker" \
      "$(escape_markdown_table_cell "$ref_label")" \
      "${head:0:10}" \
      "$(escape_markdown_table_cell "$subject")"
  fi
}

print_backlog_horizon() {
  if [[ ! -f "$REPO_ROOT/docs/BACKLOG.md" ]]; then
    printf -- '- `docs/BACKLOG.md` not found.\n'
    return
  fi

  awk '
    /^## / {
      if ($0 ~ /^## Complete$/ || $0 ~ /waybacklog/) {
        next
      }
      if (sections >= 2) {
        exit
      }
      sections++
      items = 0
      print ""
      print "### " substr($0, 4)
      next
    }
    sections > 0 && /^- / {
      if (items < 3) {
        print $0
      }
      items++
    }
  ' "$REPO_ROOT/docs/BACKLOG.md"
}

REPO_ROOT=$(git rev-parse --show-toplevel)
CURRENT_HEAD=$(git rev-parse HEAD)
CURRENT_SHORT=$(git rev-parse --short HEAD)
CURRENT_SUBJECT=$(git log -1 --pretty=%s HEAD)
CURRENT_BRANCH=$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)
MAIN_REF=$(pick_main_ref)
MAIN_SHORT=$(git rev-parse --short "$MAIN_REF")
MAIN_SUBJECT=$(git log -1 --pretty=%s "$MAIN_REF")
BRANCH_ROWS_FILE=$(make_temp_file)
trap 'rm -f "$BRANCH_ROWS_FILE"' EXIT
WORKTREE_PORCELAIN=$(git worktree list --porcelain)
WORKTREE_COUNT=$(printf "%s\n" "$WORKTREE_PORCELAIN" | awk '/^worktree / { count++ } END { print count + 0 }')
DETACHED_WORKTREE_COUNT=$(printf "%s\n" "$WORKTREE_PORCELAIN" | awk '/^detached$/ { count++ } END { print count + 0 }')
LOCAL_BRANCH_COUNT=$(git for-each-ref --format='%(refname:short)' refs/heads | count_nonempty_lines)

read -r CURRENT_BEHIND_MAIN CURRENT_AHEAD_MAIN < <(git rev-list --left-right --count "$MAIN_REF"...HEAD)

STAGED_COUNT=$(git diff --cached --name-only | count_nonempty_lines)
UNSTAGED_COUNT=$(git diff --name-only | count_nonempty_lines)
UNTRACKED_COUNT=$(git ls-files --others --exclude-standard | count_nonempty_lines)

CONTAINING_LOCAL="-"
CONTAINING_REMOTE="-"
if [[ -z "$CURRENT_BRANCH" ]]; then
  CONTAINING_LOCAL=$(git for-each-ref --format='%(refname:short)' --contains HEAD refs/heads | head -n 5 | join_lines)
  CONTAINING_REMOTE=$(git for-each-ref --format='%(refname:short)' --contains HEAD refs/remotes | head -n 5 | join_lines)
fi

CURRENT_DIFF_STAT=$(git diff --shortstat "$MAIN_REF"...HEAD | sed 's/^ *//' || true)
if [[ -z "$CURRENT_DIFF_STAT" ]]; then
  CURRENT_DIFF_STAT="no file-level diff from $MAIN_REF"
fi

CURRENT_DIFF_HIGHLIGHTS=$(git diff --name-status "$MAIN_REF"...HEAD | head -n 10 || true)

DISPLAYED_BRANCHES=0
UPSTREAM_DIVERGENCE_COUNT=0
CHECKED_OUT_MAIN_DIVERGENCE_COUNT=0

while IFS=$'\t' read -r branch upstream _subject; do
  [[ -z "$branch" ]] && continue

  ahead_up=0
  behind_up=0
  if [[ -n "$upstream" ]]; then
    read -r behind_up ahead_up < <(git rev-list --left-right --count "${upstream}...${branch}" 2>/dev/null || printf "0 0\n")
    if (( ahead_up > 0 || behind_up > 0 )); then
      UPSTREAM_DIVERGENCE_COUNT=$((UPSTREAM_DIVERGENCE_COUNT + 1))
    fi
  fi

  worktree_path=$(worktree_path_for_branch "$branch")
  read -r behind_main ahead_main < <(git rev-list --left-right --count "${MAIN_REF}...${branch}" 2>/dev/null || printf "0 0\n")
  if [[ -n "$worktree_path" ]] && (( ahead_main > 0 || behind_main > 0 )); then
    CHECKED_OUT_MAIN_DIVERGENCE_COUNT=$((CHECKED_OUT_MAIN_DIVERGENCE_COUNT + 1))
  fi

  include=0
  if (( ALL_BRANCHES == 1 )); then
    include=1
  elif [[ -n "$worktree_path" || "$branch" == "$CURRENT_BRANCH" ]]; then
    include=1
  elif (( ahead_up > 0 || behind_up > 0 )); then
    include=1
  fi

  if (( include == 1 )); then
    if [[ -z "$upstream" ]]; then
      upstream="-"
    fi
    if [[ -z "$worktree_path" ]]; then
      worktree_label="-"
    else
      worktree_label=$(shorten_path "$worktree_path")
    fi
    branch_cell=$(escape_markdown_table_cell "$branch")
    upstream_cell=$(escape_markdown_table_cell "$upstream")
    worktree_cell=$(escape_markdown_table_cell "$worktree_label")
    printf '| %s | %s | +%s / -%s | +%s / -%s | %s |\n' \
      "$branch_cell" \
      "$upstream_cell" \
      "$ahead_up" \
      "$behind_up" \
      "$ahead_main" \
      "$behind_main" \
      "$worktree_cell" >>"$BRANCH_ROWS_FILE"
    DISPLAYED_BRANCHES=$((DISPLAYED_BRANCHES + 1))
  fi
done < <(git for-each-ref --format='%(refname:short)%09%(upstream:short)%09%(contents:subject)' refs/heads)

printf "# Florence Daily Standup Snapshot\n\n"
printf -- "- Generated: %s\n" "$(date '+%Y-%m-%d %H:%M:%S %Z')"
printf -- '- Repo: `%s`\n' "$REPO_ROOT"
printf -- '- Main reference: `%s` (`%s`) %s\n\n' "$MAIN_REF" "$MAIN_SHORT" "$MAIN_SUBJECT"

printf "## Current Checkout\n\n"
printf -- '- Worktree: `%s`\n' "$(shorten_path "$REPO_ROOT")"
printf -- '- HEAD: `%s` %s\n' "$CURRENT_SHORT" "$CURRENT_SUBJECT"
if [[ -n "$CURRENT_BRANCH" ]]; then
  printf -- '- Branch: `%s`\n' "$CURRENT_BRANCH"
else
  printf -- '- Branch: `detached`\n'
  printf -- "- Containing local refs: %s\n" "$CONTAINING_LOCAL"
  printf -- "- Containing remote refs: %s\n" "$CONTAINING_REMOTE"
fi
printf -- '- Vs `%s`: ahead %s, behind %s\n' "$MAIN_REF" "$CURRENT_AHEAD_MAIN" "$CURRENT_BEHIND_MAIN"
printf -- "- Diff summary: %s\n" "$CURRENT_DIFF_STAT"
printf -- "- Local changes: %s staged, %s unstaged, %s untracked\n\n" "$STAGED_COUNT" "$UNSTAGED_COUNT" "$UNTRACKED_COUNT"

if [[ -n "$CURRENT_DIFF_HIGHLIGHTS" ]]; then
  printf "### Current Diff Highlights\n\n"
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    printf -- '- %s\n' "$(escape_markdown_table_cell "$line")"
  done <<<"$CURRENT_DIFF_HIGHLIGHTS"
  printf "\n"
fi

printf "## Active Worktrees\n\n"
printf "| Path | Ref | HEAD | Subject |\n"
printf "| --- | --- | --- | --- |\n"
print_worktree_rows
printf "\n"

printf "## Local Branch Sync\n\n"
if (( ALL_BRANCHES == 1 )); then
  printf "Showing all %s local branches.\n\n" "$LOCAL_BRANCH_COUNT"
else
  printf 'Showing %s branches that are checked out or divergent. Pass `--all-branches` to include all %s local branches.\n\n' "$DISPLAYED_BRANCHES" "$LOCAL_BRANCH_COUNT"
fi
printf "| Branch | Upstream | Vs upstream | Vs %s | Worktree |\n" "$MAIN_REF"
printf "| --- | --- | --- | --- | --- |\n"
if [[ -s "$BRANCH_ROWS_FILE" ]]; then
  cat "$BRANCH_ROWS_FILE"
else
  printf '| `-` | `-` | +0 / -0 | +0 / -0 | - |\n'
fi
printf "\n"

printf "## Standup Signals\n\n"
if [[ -z "$CURRENT_BRANCH" ]]; then
  printf -- '- Current worktree is detached; Florence should anchor the discussion to commit `%s` and the containing refs above.\n' "$CURRENT_SHORT"
fi
if (( CURRENT_BEHIND_MAIN > 0 )); then
  printf -- '- Current checkout is behind `%s` by %s commit(s); sync risk should be addressed before deeper feature work.\n' "$MAIN_REF" "$CURRENT_BEHIND_MAIN"
fi
if (( CURRENT_AHEAD_MAIN > 0 )); then
  printf -- '- Current checkout is ahead of `%s` by %s commit(s); review whether that work belongs in a branch or PR context.\n' "$MAIN_REF" "$CURRENT_AHEAD_MAIN"
fi
if (( STAGED_COUNT + UNSTAGED_COUNT + UNTRACKED_COUNT > 0 )); then
  printf -- "- Current worktree has uncommitted changes; decide whether to keep, split, or discard them before planning the day.\n"
fi
printf -- "- %s worktree(s) are active; %s of them are detached.\n" "$WORKTREE_COUNT" "$DETACHED_WORKTREE_COUNT"
printf -- '- %s local branch(es) diverge from their upstream; %s checked-out branch(es) diverge from `%s`.\n\n' "$UPSTREAM_DIVERGENCE_COUNT" "$CHECKED_OUT_MAIN_DIVERGENCE_COUNT" "$MAIN_REF"

printf "## Backlog Horizon\n"
print_backlog_horizon
printf "\n"
