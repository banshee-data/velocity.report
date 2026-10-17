#!/usr/bin/env bash
# free-space-scan.sh - read-only survey of where disk space can be freed.
#
#   scripts/free-space-scan.sh            scan the internal disk, the lidar volume and the NAS shares
#   MIN_MB=500 scripts/free-space-scan.sh lower the size threshold (default 1000 MB)
#
# Never deletes, moves or writes anything. It prints candidates with a class, a suggested
# NAS share and whether a PASS record matching its name exists. Moving and deleting are
# separate, deliberate steps: see .claude/skills/free-space/SKILL.md.
#
# Overrides (environment): LIDAR_VOL, NAS_CAPTURES, NAS_RESULTS, NAS_MEDIA, NAS_HOST,
# CODE_DIR, MIN_MB, SCAN_TIMEOUT (seconds per directory, default 300).

set -u

LIDAR=${LIDAR_VOL:-/Volumes/lidar}
NAS_CAPTURES=${NAS_CAPTURES:-/Volumes/banshee-captures}
NAS_RESULTS=${NAS_RESULTS:-/Volumes/banshee-results}
NAS_MEDIA=${NAS_MEDIA:-/Volumes/media}
NAS_HOST=${NAS_HOST:-arrow.local}
CODE_DIR=${CODE_DIR:-$HOME/code}
MIN_MB=${MIN_MB:-1000}
SCAN_TIMEOUT=${SCAN_TIMEOUT:-300}
VALIDATE_DIR="$LIDAR/offload/_logs"

limited() { perl -e 'alarm shift; exec @ARGV' "$SCAN_TIMEOUT" "$@"; }

human() {
  awk -v k="$1" 'BEGIN { g = k / 1048576; if (g >= 1) printf "%7.1f GiB", g; else printf "%7.0f MiB", k / 1024 }'
}

# class: captures | results | media | regenerable | review
classify() {
  local n
  n=$(basename "$1" | tr '[:upper:]' '[:lower:]')
  case $n in
    *pcap* | *sensor_data* | *capture*) echo captures ;;
    *deriveddata* | *cache* | *node_modules* | .gocache | .tox | *.xcresult) echo regenerable ;;
    *result* | *campaign* | *output* | s2-* | *experiment* | *sweep* | *replay*) echo results ;;
    *video* | *photo* | *music* | *movie* | *fiction* | *.mov | *.mp4 | done) echo media ;;
    *) echo review ;;
  esac
}

destination() {
  case $1 in
    captures) echo "$NAS_CAPTURES" ;;
    results) echo "$NAS_RESULTS" ;;
    media) echo "$NAS_MEDIA" ;;
    regenerable) echo "delete (rebuilds itself)" ;;
    *) echo "decide" ;;
  esac
}

# A copy counts as verified only if a validate_*.txt naming the item says "VERDICT: PASS".
nas_status() {
  local base f
  base=$(basename "$1" | tr '[:upper:] ' '[:lower:]-')
  for f in "$VALIDATE_DIR"/validate_*.txt; do
    [ -e "$f" ] || continue
    case $(basename "$f" | tr '[:upper:]' '[:lower:]') in
      *"$base"*)
        if grep -q "VERDICT: PASS" "$f" 2> /dev/null; then
          echo "name-match PASS ($(basename "$f"))"
          return
        fi
        ;;
    esac
  done
  echo "no PASS record by name"
}

# scan_children DIR [skip-regex]: sizes of DIR's direct children above the threshold, largest first.
scan_children() {
  local dir=$1 skip=${2:-^$}
  [ -d "$dir" ] || return 0
  find "$dir" -mindepth 1 -maxdepth 1 ! -type l -print0 2> /dev/null \
    | while IFS= read -r -d '' p; do
      basename "$p" | grep -Eq "$skip" || printf '%s\0' "$p"
    done \
    | xargs -0 -P8 -n1 perl -e 'alarm shift; exec @ARGV' "$SCAN_TIMEOUT" du -sk 2> /dev/null \
    | awk -F'\t' -v min=$((MIN_MB * 1024)) '$1 >= min' \
    | sort -rn \
    | while IFS=$'\t' read -r kb path; do
      local cls
      cls=$(classify "$path")
      printf '%s  %-11s -> %-28s %-34s %s\n' "$(human "$kb")" "$cls" "$(destination "$cls")" "$(nas_status "$path")" "$path"
    done
}

echo "== Free space"
df -h / "$LIDAR" "$NAS_CAPTURES" "$NAS_RESULTS" "$NAS_MEDIA" 2> /dev/null | awk 'NR == 1 || !seen[$1 $9]++'

echo
echo "== NAS mounts"
for m in "$NAS_CAPTURES" "$NAS_RESULTS" "$NAS_MEDIA"; do
  if mount | grep -q " on $m "; then
    echo "mounted   $m"
  else
    echo "MISSING   $m   remount: osascript -e 'mount volume \"smb://$USER@$NAS_HOST._smb._tcp.local/$(basename "$m")\"'"
  fi
done
[ -d "$LIDAR" ] || echo "NOTE: $LIDAR is not mounted; skipping lidar sections"

echo
echo "== Internal disk: home folder (>= ${MIN_MB} MB)"
scan_children "$HOME" '^(Library|\.Trash)$'

echo
echo "== Internal disk: ~/Library (>= ${MIN_MB} MB; Mobile Documents skipped)"
scan_children "$HOME/Library" '^(Mobile Documents)$'
scan_children "$HOME/Library/Developer"
scan_children "$HOME/Library/Application Support"

echo
echo "== Internal disk: repos in $CODE_DIR (>= ${MIN_MB} MB)"
scan_children "$CODE_DIR"

echo
echo "== Internal disk: Trash"
[ -d "$HOME/.Trash" ] && du -sk "$HOME/.Trash" 2> /dev/null | awk '{print $1}' | while read -r kb; do echo "$(human "$kb")  $HOME/.Trash (empty it yourself)"; done

echo
echo "== Internal disk: idle git worktrees (dry run)"
SWEEP="$(dirname "$0")/worktree-sweep.sh"
if [ -x "$SWEEP" ]; then
  "$SWEEP" 2>&1 | tail -n 40
else
  echo "scripts/worktree-sweep.sh not found"
fi

echo
echo "== Internal disk: single files >= 1 GiB outside Library"
limited find "$HOME" -xdev -type f -size +1G -not -path "$HOME/Library/*" -not -path "*/.Trash/*" -print0 2> /dev/null \
  | xargs -0 -n1 du -sk 2> /dev/null | sort -rn | head -n 25 \
  | while IFS=$'\t' read -r kb path; do printf '%s  %s\n' "$(human "$kb")" "$path"; done

if [ -d "$LIDAR" ]; then
  echo
  echo "== lidar volume: top level (>= ${MIN_MB} MB)"
  scan_children "$LIDAR"
  echo
  echo "== lidar volume: offload/ and lidar/ (>= ${MIN_MB} MB)"
  scan_children "$LIDAR/offload" '^_(logs|manifests)$'
  scan_children "$LIDAR/lidar"
fi

echo
echo "== Reading this output"
echo "Class 'captures'/'results'/'media' items are NAS candidates. 'verified' means a validate_*.txt"
echo "matched a PASS record by name only: confirm it is the same tree before the user deletes."
echo "'no PASS record' means unknown, not absent: check the NAS, then copy and verify. Class 'review' needs a human decision. Nothing was changed."
