#!/bin/bash
# worktree-sweep - find, and optionally remove, idle git worktrees whose work is safe elsewhere.
#
#   worktree-sweep                  dry run over every repo in ~/code that has worktrees
#   worktree-sweep ~/code/foo       dry run for one repo
#   worktree-sweep --apply          remove what passes
#   worktree-sweep --days 14        "idle" = no file changed for 14 days (default 7)
#
# A worktree is removed only if ALL of these hold at that moment:
#   1. no process has anything open inside it (shell, editor, Claude/Codex session...), and it is not locked
#   2. no file in it changed in the last N days
#   3. no uncommitted work - untracked node_modules links and .claude/launch.json don't count
#      (removing a worktree deletes such a link, never the folder it points to)
#   4. its commits survive: HEAD is on a branch (git keeps branches), inside another branch, or on GitHub
# Removal deletes only the checkout folder. Branches, commits and Claude/Codex sessions stay.

APPLY=0; DAYS=7; REPOS=()
while [ $# -gt 0 ]; do
  case $1 in
    --apply) APPLY=1 ;;
    --days) DAYS=$2; shift ;;
    -h|--help) sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) REPOS+=("$1") ;;
  esac
  shift
done
if [ ${#REPOS[@]} -eq 0 ]; then
  for d in "$HOME"/code/*/; do
    d=${d%/}
    [ -d "$d/.git" ] && [ "$(git -C "$d" worktree list 2> /dev/null | wc -l)" -gt 1 ] && REPOS+=("$d")
  done
fi
[ ${#REPOS[@]} -eq 0 ] && { echo "no repos with worktrees found"; exit 0; }

TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
lsof -n -Fcn 2> /dev/null | awk '/^c/{c = substr($0, 2)} /^n\//{print c "\t" substr($0, 2)}' > "$TMP/open"

who_has() {  # names of processes with anything open inside $1
  awk -F'\t' -v p="$1" 'index($2, p) == 1 && (length($2) == length(p) || substr($2, length(p) + 1, 1) == "/") {print $1}' "$TMP/open" \
    | sort -u | head -3 | tr '\n' ',' | sed 's/,$//'
}
real_dirt() {  # uncommitted entries that are real work (ignores node_modules links and .claude/launch.json)
  local p=$1 line path
  git -C "$p" status --porcelain --untracked-files=all 2> /dev/null | while IFS= read -r line; do
    path=${line:3}
    if [ "${line:0:2}" = "??" ]; then
      [ "$(basename "$path")" = node_modules ] && [ -L "$p/$path" ] && continue
      [ "$path" = .claude/launch.json ] && continue
    fi
    echo "$line"
  done
}
kept_elsewhere() {  # succeeds if HEAD's commits survive removing the worktree
  local p=$1 repo=$2 sha slug
  git -C "$p" symbolic-ref -q HEAD > /dev/null && return 0
  sha=$(git -C "$p" rev-parse HEAD 2> /dev/null) || return 1
  [ -n "$(git -C "$repo" branch -a --contains "$sha" 2> /dev/null | head -1)" ] && return 0
  slug=$(git -C "$repo" remote get-url origin 2> /dev/null | sed -nE 's#^(https://github\.com/|git@github\.com:)([^/]+/[^/]+)$#\2#p' | sed 's/\.git$//')
  [ -n "$slug" ] && command -v gh > /dev/null && gh api "repos/$slug/commits/$sha" --silent 2> /dev/null
}
gb() { echo "$1/1048576" | bc -l | awk '{printf "%.1f", $1}'; }

[ $APPLY = 1 ] && echo "worktree-sweep: removing idle worktrees (idle = no changes for $DAYS days)" \
               || echo "worktree-sweep: dry run, nothing removed (idle = no changes for $DAYS days; add --apply to remove)"
nr=0; nk=0; tot=0
for repo in "${REPOS[@]}"; do
  repo=$(git -C "$repo" rev-parse --show-toplevel 2> /dev/null) || { echo "not a git repo: $repo"; continue; }
  git -C "$repo" worktree list --porcelain | sed -n 's/^worktree //p' | tail -n +2 > "$TMP/wts"
  echo; echo "${repo/#$HOME/~}  ($(wc -l < "$TMP/wts" | tr -d ' ') linked worktrees)"
  while IFS= read -r p; do
    s=${p/#$HOME/\~}
    if [ ! -d "$p" ]; then echo "  gone              $s  (cleared by git worktree prune)"; continue; fi
    b=$(git -C "$p" symbolic-ref --short -q HEAD 2> /dev/null || echo detached)
    why=""
    gd=$(git -C "$p" rev-parse --absolute-git-dir 2> /dev/null)
    if [ -f "$gd/locked" ]; then why="locked"
    elif w=$(who_has "$p"); [ -n "$w" ]; then why="in use by $w"
    elif [ -n "$(find "$p" \( -name .git -o -name node_modules \) -prune -o -type f -mtime -"$DAYS" -print 2> /dev/null | head -1)" ]; then
      why="changed in the last $DAYS days"
    elif d=$(real_dirt "$p"); [ -n "$d" ]; then why="uncommitted work ($(printf '%s\n' "$d" | wc -l | tr -d ' ') files)"
    elif ! kept_elsewhere "$p" "$repo"; then why="detached commits that exist nowhere else"
    fi
    if [ -n "$why" ]; then printf "  KEEP              %s  [%s]  %s\n" "$s" "$b" "$why"; nk=$((nk + 1)); continue; fi
    kb=$(du -sk "$p" 2> /dev/null | cut -f1)
    if [ $APPLY = 0 ]; then
      printf "  remove   %5s GB  %s  [%s]\n" "$(gb "$kb")" "$s" "$b"; nr=$((nr + 1)); tot=$((tot + kb)); continue
    fi
    f=""; [ -n "$(git -C "$p" status --porcelain 2> /dev/null)" ] && f=--force   # only ignorable entries remain
    if git -C "$repo" worktree remove $f "$p"; then
      rmdir "$(dirname "$p")" 2> /dev/null   # e.g. an emptied ~/.codex/worktrees/<id>
      printf "  removed  %5s GB  %s  [%s]\n" "$(gb "$kb")" "$s" "$b"; nr=$((nr + 1)); tot=$((tot + kb))
    else
      echo "  FAILED            $s"
    fi
  done < "$TMP/wts"
  [ $APPLY = 1 ] && git -C "$repo" worktree prune
done
echo
[ $APPLY = 1 ] && echo "removed $nr worktrees ($(gb $tot) GB), kept $nk" || echo "$nr removable ($(gb $tot) GB), $nk kept"
