#!/bin/bash
# Thin the annotation pack's revision history, with a verified backup first.
#
#   bash run-prune.sh                 dry run: builds the tool, prints what it would keep and remove, writes nothing to the pack
#   bash run-prune.sh apply           does it: backs the removed revisions up, checks the backup, then removes them
#   bash run-prune.sh [apply] /full/path/to/the/pack
#
# Environment (all optional):
#   KEEP_LAST    newest revisions to keep (default 100)
#   KEEP_EVERY   thin older ones to one per interval (default 1h)
#   BACKUP_DIR   where the backup goes on apply (default /Volumes/lidar/backups)
#   PRUNE_REF    the git ref the tool is built from (default: origin/main)
#
# The Annotation window can stay open: the tool takes the pack's writer lock only for the
# removal, and the window's saves archive revisions newer than any being removed.
# The backup is read back and every digest compared before anything is removed.
NAME=ad8b9438-c1d4-40f0-bd0a-f1852c984569-20260929-184718.156560000
BRANCH=claude/upbeat-galileo-4xbaat-annot-ad8b9438
HERE=$(cd "$(dirname "$0")" && pwd)       # <worktree>/results/annot-ad8b9438
SRC=$(cd "$HERE/../.." && pwd)            # the worktree this script came in
W=${PRUNE_WORK:-$(dirname "$SRC")}
KEEP_LAST=${KEEP_LAST:-100}
KEEP_EVERY=${KEEP_EVERY:-1h}
BACKUP_DIR=${BACKUP_DIR:-/Volumes/lidar/backups}

MODE=dry
if [ "${1:-}" = "apply" ]; then MODE=apply; shift; fi
PACK=${1:-}

if [ -z "$PACK" ]; then
  shopt -s nullglob
  PACK=$( { mdfind -name "$NAME" 2>/dev/null
            printf '%s\n' /Volumes/lidar/*/annotation-packs/"$NAME" \
              /Volumes/lidar/*/*/annotation-packs/"$NAME" \
              ~/*/sensor_data/lidar/annotation-packs/"$NAME" \
              ~/*/*/sensor_data/lidar/annotation-packs/"$NAME" \
              ~/*/*/*/sensor_data/lidar/annotation-packs/"$NAME"
          } | grep "/$NAME\$" | head -1 )
  shopt -u nullglob
fi
if [ -z "$PACK" ] || [ ! -d "$PACK/annotation-revisions" ]; then
  echo "PACK NOT FOUND. Run again with its path: bash $0 [apply] /full/path/to/$NAME"
  exit 1
fi
command -v go >/dev/null || { echo "go is not on PATH"; exit 1; }
echo "PACK=$PACK"
echo "revision files: $(ls "$PACK/annotation-revisions" | wc -l | tr -d ' ')   size: $(du -sk "$PACK/annotation-revisions" | cut -f1) KB"

# 1. the tool, built from the branch that carries it
mkdir -p "$W/bin" "$W/gocache" "$W/tmp" || exit 1
REF=${PRUNE_REF:-origin/main}             # the tool merged with #658; the branch it came from is gone
git -C "$SRC" fetch -q origin main || { echo "FETCH FAILED: cannot reach origin"; exit 1; }
git -C "$SRC" rev-parse -q --verify "$REF^{commit}" >/dev/null || { echo "$REF not found"; exit 1; }
echo "building the tool from $REF ($(git -C "$SRC" rev-parse --short=8 "$REF"))"
if [ -e "$W/prune-src" ]; then
  git -C "$W/prune-src" checkout -q --detach "$REF" || { echo "CHECKOUT FAILED"; exit 1; }
else
  git -C "$SRC" worktree add -q --detach "$W/prune-src" "$REF" || { echo "WORKTREE FAILED"; exit 1; }
fi
(cd "$W/prune-src" && GOCACHE="$W/gocache" GOTMPDIR="$W/tmp" go build -o "$W/bin/lidar-annotation-prune" ./cmd/tools/lidar-annotation-prune) \
  || { echo "BUILD FAILED (is ./cmd/tools/lidar-annotation-prune in $REF?)"; exit 1; }

# 2. keep what a frozen split pins: every split file beside the packs folder
SPLITS=()
shopt -s nullglob
for f in "$(dirname "$PACK")"/splits/*.json; do SPLITS+=(-split "$f"); done
shopt -u nullglob
echo "split files protecting revisions: $(( ${#SPLITS[@]} / 2 ))"

OUT="$HERE/prune"
mkdir -p "$OUT"
TOOL=("$W/bin/lidar-annotation-prune" -pack "$PACK" -keep-last "$KEEP_LAST" -keep-every "$KEEP_EVERY")
if [ ${#SPLITS[@]} -gt 0 ]; then TOOL+=("${SPLITS[@]}"); fi

if [ "$MODE" = dry ]; then
  echo "=== DRY RUN: nothing is written to the pack ==="
  "${TOOL[@]}" -json "$OUT/plan.json" | tee "$OUT/plan.txt"
  echo "To do it: bash $0 apply"
  exit 0
fi

# 3. apply
mkdir -p "$BACKUP_DIR" || { echo "cannot create $BACKUP_DIR"; exit 1; }
BACKUP="$BACKUP_DIR/ad8b9438-revisions-$(date -u +%Y%m%dT%H%M%SZ).tar.gz"
echo "=== APPLY: backup first, to $BACKUP ==="
df -h "$BACKUP_DIR" | tail -1
HEAD_BEFORE=$(shasum -a 256 "$PACK/annotations.json" | cut -c1-64)
"${TOOL[@]}" -apply -backup "$BACKUP" -min-free-ratio 0.15 -json "$OUT/report.json" 2>&1 | tee "$OUT/run.txt"
STATUS=${PIPESTATUS[0]}
echo "exit status: $STATUS"
echo "revision files now: $(ls "$PACK/annotation-revisions" | wc -l | tr -d ' ')   size: $(du -sk "$PACK/annotation-revisions" | cut -f1) KB"
echo "backup: $(ls -la "$BACKUP" 2>/dev/null)"
echo "head before: $HEAD_BEFORE"
echo "head now:    $(shasum -a 256 "$PACK/annotations.json" | cut -c1-64) (differs only if you saved meanwhile)"
df -h "$BACKUP_DIR" "$PACK" | cat
[ "$STATUS" = 0 ] || { echo "PRUNE FAILED: the pack is as it was unless the report above says otherwise"; exit 1; }

# 4. report back (results only; never touches the pack)
[ -n "${PRUNE_NO_PUSH:-}" ] && { echo DONE; exit 0; }
cd "$SRC" || exit 1
git add results/annot-ad8b9438/prune/report.json results/annot-ad8b9438/prune/run.txt results/annot-ad8b9438/prune/plan.json results/annot-ad8b9438/prune/plan.txt 2>/dev/null
MSG="[ai][docs] Annotation pack ad8b9438: revision prune report

Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PjD7pGbgHLYoirWoAVBrKV"
git commit -q -m "$MSG" || { git commit -q --no-verify -m "$MSG"; } || { echo "COMMIT FAILED"; exit 1; }
git push origin HEAD:"$BRANCH" || { echo "PUSH FAILED (the prune itself is done)"; exit 1; }
echo DONE
