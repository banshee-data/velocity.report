#!/usr/bin/env bash
# free-space-scan.sh - read-only survey of where disk space can be freed.
#
#   scripts/free-space-scan.sh            scan the internal disk, the local lidar volume
#   scripts/free-space-scan.sh --min-mb 500   lower the size threshold (default 1000 MB)
#
# Never deletes, moves or writes anything. It prints candidates with a class, a suggested
# local destination. A file-name class never establishes retention. Moving and deleting are
# separate, deliberate steps: see .claude/skills/free-space/SKILL.md.
#
# Overrides (environment): LIDAR_VOL,
# CODE_DIR, MIN_MB, SCAN_TIMEOUT (seconds per directory, default 300).

set -u

LIDAR=${LIDAR_VOL:-/Volumes/lidar}
CODE_DIR=${CODE_DIR:-$HOME/code}
MIN_MB=${MIN_MB:-1000}
SCAN_TIMEOUT=${SCAN_TIMEOUT:-300}

limited() { perl -e 'alarm shift; exec @ARGV' "$SCAN_TIMEOUT" "$@"; }

human() {
  awk -v k="$1" 'BEGIN { g = k / 1048576; if (g >= 1) printf "%7.1f GiB", g; else printf "%7.0f MiB", k / 1024 }'
}

# class: captures | results | media | regenerable | review
classify() {
  local n
  n=$(basename "$1" | tr '[:upper:]' '[:lower:]')
  case $n in
    *pcap* | *sensor_data* | *capture* | *vrlog*) echo captures ;;
    *evidence* | *gerardus* | *roof* | *reference* | *label*) echo evidence ;;
    *deriveddata* | *cache* | *node_modules* | .gocache | .tox | *.xcresult) echo cache-review ;;
    *result* | *campaign* | *output* | s2-* | *experiment* | *sweep* | *replay*) echo results ;;
    *video* | *photo* | *music* | *movie* | *fiction* | *.mov | *.mp4 | done) echo media ;;
    *) echo review ;;
  esac
}

# Policy recommendations only: verify dependencies before any deletion.
destination() {
  local cls=$1 path=$2
  case "$path/" in
    "$LIDAR/"*) echo "KEEP local copy; review retention"; return ;;
  esac
  case $cls in
    captures|results|evidence) echo "$LIDAR/evidence (verify dependencies)" ;;
    media) echo "$LIDAR (review retention)" ;;
    cache-review) echo "review local rebuild inputs before deletion" ;;
    *) echo "review ownership and dependencies" ;;
  esac
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
      printf '%s  %-11s -> %-28s %-34s %s\n' "$(human "$kb")" "$cls" "$(destination "$cls" "$path")" "retention unverified" "$path"
    done
}

main() {
  while [ $# -gt 0 ]; do
    case $1 in
      --min-mb) MIN_MB=${2:?--min-mb needs a number}; shift ;;
      -h | --help) sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
      *) echo "unknown argument: $1" >&2; exit 2 ;;
    esac
    shift
  done
  case $MIN_MB in '' | *[!0-9]*) echo "--min-mb must be a whole number of MB" >&2; exit 2 ;; esac
  echo "== Free space"
  df -h / "$LIDAR" 2> /dev/null | awk 'NR == 1 || !seen[$1 $9]++'

  echo
  echo "== Local evidence policy"
  echo "Required evidence stays on lidar. NAS copies are optional final archives."
  [ -d "$LIDAR" ] || echo "MISSING: $LIDAR; keep originals and restore the local volume before moving files."

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
  echo "Required large files belong on lidar. Items already there are kept by default."
  echo "Name-based classes do not establish disposability; trace services, symlinks, indexes and blobs."
  echo "An archive PASS receipt never permits deleting required local evidence."
  echo "Verify complete local copies and test without NAS access before proposing deletion. Nothing was changed."
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
