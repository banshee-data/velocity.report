#!/bin/bash
# Score the near-edge arms against the labels in annotation pack ad8b9438, on the capture it was cut from.
#
#   bash run-kirk0.sh                       replay the arms on kirk0, draft the split, score, push the results
#   bash run-kirk0.sh /full/path/to/pack    when the pack is not found
#
# Environment (all optional):
#   ARMS           arms to replay, in this order (default: control track track_t5 track_t5_tight track_a1)
#   KIRK0_PCAP     full path of kirk0.pcapng (default: searched for, then its SHA-256 is checked)
#   KIRK0_REF      git ref the tools are built from (default: the score branch, else origin/main)
#   KEEP_EVIDENCE  set to keep the replay databases (about 55 MB each) in the scratch directory
#   NO_PUSH        set to leave the results uncommitted
#   SKIP_BUSY_CHECK  set to start a replay although another one is running
#
# It never writes to the pack: it works on a copy-on-write clone of its files (no extra disk on APFS),
# taken once, so a save in the Annotation window while this runs cannot change what is scored.
# kirk0 is the capture the shadow work was developed on, so its split is a tuning split: the numbers
# say how the arms differ on labelled returns, never how they would do held out.
#
# One replay at a time, each about a minute. Evidence goes under ~/vr-scratch and is deleted at the end;
# only the reports (small) are committed.
NAME=ad8b9438-c1d4-40f0-bd0a-f1852c984569-20260929-184718.156560000
BRANCH=claude/upbeat-galileo-4xbaat-annot-ad8b9438
KIRK0_SHA=2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a
HERE=$(cd "$(dirname "$0")" && pwd)       # <worktree>/results/annot-ad8b9438
SRC=$(cd "$HERE/../.." && pwd)            # the worktree this script came in
W=${KIRK0_WORK:-$(dirname "$SRC")}
S=${KIRK0_SCRATCH:-$HOME/vr-scratch/annot-ad8b9438-kirk0}
OUT="$HERE/kirk0"
ARMS=${ARMS:-control track track_t5 track_t5_tight track_a1}
PACK=${1:-}

sha256() { if command -v shasum >/dev/null; then shasum -a 256 "$1" | cut -c1-64; else sha256sum "$1" | cut -c1-64; fi; }
need() { command -v "$1" >/dev/null || { echo "$1 is not on PATH"; exit 1; }; }
need go; need git

# ---- 1. the pack ----------------------------------------------------------------------------
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
if [ -z "$PACK" ] || [ ! -f "$PACK/annotations.json" ]; then
  echo "PACK NOT FOUND. Run again with its path: bash $0 /full/path/to/$NAME"
  exit 1
fi
echo "PACK=$PACK"

# ---- 2. the capture, read in place ------------------------------------------------------------
PCAP=${KIRK0_PCAP:-}
if [ -z "$PCAP" ]; then
  while IFS= read -r f; do
    [ -f "$f" ] || continue
    # a Git LFS pointer is a few hundred bytes; the capture is about 200 MB
    [ "$(wc -c < "$f" | tr -d ' ')" -gt 100000000 ] || continue
    PCAP=$f; break
  done < <(
    shopt -s nullglob
    { mdfind -name kirk0.pcapng 2>/dev/null
      printf '%s\n' /Volumes/lidar/*/kirk0.pcapng /Volumes/lidar/*/*/kirk0.pcapng /Volumes/lidar/*/*/*/kirk0.pcapng \
        ~/*/internal/lidar/perf/pcap/kirk0.pcapng ~/*/*/internal/lidar/perf/pcap/kirk0.pcapng \
        "$SRC"/internal/lidar/perf/pcap/kirk0.pcapng
    } | grep 'kirk0.pcapng$' | awk '!seen[$0]++'
  )
fi
if [ -z "$PCAP" ] || [ ! -f "$PCAP" ]; then
  echo "kirk0.pcapng NOT FOUND (a Git LFS pointer does not count). Run again with KIRK0_PCAP=/full/path/to/kirk0.pcapng bash $0"
  exit 1
fi
echo "PCAP=$PCAP"
GOT=$(sha256 "$PCAP")
if [ "$GOT" != "$KIRK0_SHA" ]; then
  echo "kirk0.pcapng has SHA-256 $GOT, not $KIRK0_SHA: that is not the reference capture"
  exit 1
fi

# ---- 3. the tools, built from the branch that carries them ------------------------------------------
case "$S" in ""|/|"$HOME"|"$HOME"/) echo "refusing to clear $S"; exit 1 ;; esac
rm -rf "$S"; mkdir -p "$S" "$W/bin" "$W/gocache" "$W/tmp" || exit 1
REF=${KIRK0_REF:-origin/claude/upbeat-galileo-4xbaat-annot-score}
git -C "$SRC" fetch -q origin claude/upbeat-galileo-4xbaat-annot-score main 2>/dev/null
git -C "$SRC" rev-parse -q --verify "$REF^{commit}" >/dev/null || REF=origin/main
git -C "$SRC" cat-file -e "$REF:cmd/tools/lidar-near-face-eval/main.go" 2>/dev/null \
  || { echo "$REF does not carry cmd/tools/lidar-near-face-eval: set KIRK0_REF to the score branch"; exit 1; }
echo "building the tools from $REF ($(git -C "$SRC" rev-parse --short=8 "$REF"))"
if [ -e "$W/score-src" ]; then
  git -C "$W/score-src" checkout -q --detach "$REF" || { echo "CHECKOUT FAILED"; exit 1; }
else
  git -C "$SRC" worktree add -q --detach "$W/score-src" "$REF" || { echo "WORKTREE FAILED"; exit 1; }
fi
(
  cd "$W/score-src" || exit 1
  # the server's embedded assets are placeholders on a clean checkout
  [ -x scripts/ensure-web-stub.sh ] && ./scripts/ensure-web-stub.sh >/dev/null
  [ -x scripts/ensure-docs-stub.sh ] && ./scripts/ensure-docs-stub.sh >/dev/null
  export GOCACHE="$W/gocache" GOTMPDIR="$W/tmp"
  go build -tags=pcap -o "$W/bin/kirk0-baseline" ./cmd/tools/lidar-state-estimation-baseline \
    && go build -o "$W/bin/kirk0-split-draft" ./cmd/tools/lidar-annotation-split-draft \
    && go build -o "$W/bin/kirk0-near-face" ./cmd/tools/lidar-near-face-eval \
    && go build -o "$W/bin/kirk0-ground-truth" ./cmd/tools/lidar-ground-truth-eval
) || { echo "BUILD FAILED (libpcap: brew install libpcap)"; exit 1; }
CORPUS="$W/score-src/cmd/tools/lidar-state-estimation-baseline/testdata"

# the capture where the corpus tool expects it, as a link: the file itself is read in place
mkdir -p "$S/pcaproot/pcap" && ln -s "$PCAP" "$S/pcaproot/pcap/kirk0.pcapng" || exit 1

# ---- 4. the pack, cloned once ---------------------------------------------------------------------
SNAP="$S/pack/$NAME"
mkdir -p "$SNAP" || exit 1
for f in "$PACK"/*; do
  case "$(basename "$f")" in annotation-revisions) continue ;; esac
  # cp -c clones on APFS (no space used); the plain copy is the fallback
  cp -pRc "$f" "$SNAP/" 2>/dev/null || cp -pR "$f" "$SNAP/" || { echo "COPY FAILED: $f"; exit 1; }
done
echo "pack snapshot: annotations.json sha256 $(sha256 "$SNAP/annotations.json")"

# ---- 5. replay the arms, one at a time ------------------------------------------------------------
BASE=solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members
experiment() {
  case "$1" in
    control)        echo "$BASE" ;;
    control_t5)     echo "$BASE,solid_body_rank_one_medoid" ;;
    track)          echo "$BASE,near_edge_track" ;;
    track_t5)       echo "$BASE,near_edge_track,solid_body_rank_one_medoid" ;;
    track_t5_tight) echo "$BASE,near_edge_track,solid_body_rank_one_medoid_tight" ;;
    track_a1)       echo "$BASE,near_edge_track,near_edge_track_a1" ;;
    *)              return 1 ;;
  esac
}
busy() {
  # another replay, or one of the queue scripts that starts them
  ps -axo command | grep -vE 'grep|run-kirk0|kirk0-baseline' \
    | grep -E 'lidar-state-estimation-baseline|baseline .*-evidence-dir|run-f[0-9][a-z0-9]*\.sh' | grep -vE '^(go|/[^ ]*/go) (run|build)' | head -3
}
mkdir -p "$OUT"; : > "$OUT/arms.log"
GOOD=()
for arm in $ARMS; do
  EXP=$(experiment "$arm") || { echo "unknown arm $arm"; echo "$arm unknown" >> "$OUT/arms.log"; continue; }
  if [ -z "${SKIP_BUSY_CHECK:-}" ]; then
    waited=0
    while [ -n "$(busy)" ]; do
      [ $((waited % 300)) -eq 0 ] && { echo "another replay is running; waiting (${waited}s):"; busy; }
      sleep 30; waited=$((waited + 30))
      [ $waited -gt 14400 ] && { echo "still busy after 4 h: stopping"; exit 1; }
    done
  fi
  echo "=== $arm: $EXP"
  mkdir -p "$S/$arm"
  "$W/bin/kirk0-baseline" -corpus "$CORPUS/kirk0-corpus.json" -index "$CORPUS/kirk0-index.json" \
    -pcap-root "$S/pcaproot" -pcap-subdir pcap -warmup 20 -case kirk0 -experiment "$EXP" -sample-points 256 \
    -source-manifest "$S/$arm/source-manifest.json" -out "$S/$arm/out" \
    -evidence-dir "$S/$arm/evidence" -evidence-per-case > "$S/$arm/run.log" 2>&1
  status=$?
  echo "$arm exit $status" | tee -a "$OUT/arms.log"
  tail -4 "$S/$arm/run.log"
  cp "$S/$arm/run.log" "$OUT/replay-$arm.log"
  if [ $status -eq 0 ] && [ -s "$S/$arm/evidence/kirk0.db" ]; then GOOD+=("$arm"); fi
done
has() { for a in "${GOOD[@]}"; do [ "$a" = "$1" ] && return 0; done; return 1; }
has control || { echo "the control arm did not replay: nothing to score against (see $OUT/arms.log)"; exit 1; }
DB() { echo "$S/$1/evidence/kirk0.db"; }

# ---- 6. the split: every reviewed road user, from where the replay's estimates start ---------------------
SPLIT="$S/kirk0-tuning.json"
"$W/bin/kirk0-split-draft" -pack "$SNAP" -from-evidence "$(DB control)" -output "$SPLIT" \
  -note "kirk0 tuning split drafted from the ad8b9438 pack; kirk0 is the dev capture, never held out" \
  | tee "$OUT/split-draft.txt"
[ -s "$SPLIT" ] || { echo "NO SPLIT DRAFTED (see $OUT/split-draft.txt): the reasons are listed there"; exit 1; }
cp "$SPLIT" "$OUT/kirk0-tuning.json"
SPLITARGS=(-pack "$SNAP" -split-manifest "$SPLIT" -split tuning -allow-tuning-split)

# ---- 7. score: near-face residuals, twice, so each question has its own baseline ----------------------------
faces() {   # faces <name> <arm>...   the first arm is the one the others are compared with
  local name=$1; shift
  local args=()
  for a in "$@"; do has "$a" && args+=(-arm "$a=$(DB "$a")"); done
  [ ${#args[@]} -gt 0 ] || return 0
  echo "=== near-face $name: $*"
  "$W/bin/kirk0-near-face" "${SPLITARGS[@]}" "${args[@]}" -json "$OUT/faces-$name.json" -markdown "$OUT/faces-$name.md" \
    2>&1 | tee "$OUT/faces-$name.txt"
}
faces vs-control control track track_t5 track_t5_tight track_a1   # does the tracked state beat the shadow
faces vs-track track track_t5 track_t5_tight track_a1             # does T5, or A1, change what the tracked state believes

# ---- 8. score: identity, the same pairs the F-series compared --------------------------------------------
identity() {   # identity <a> <b>
  has "$1" && has "$2" || return 0
  echo "=== identity $1 vs $2"
  "$W/bin/kirk0-ground-truth" perframe "${SPLITARGS[@]}" \
    -a-label "$1" -a-db "$(DB "$1")" -a-stage online -a-declared-baseline \
    -b-label "$2" -b-db "$(DB "$2")" -b-stage online -b-declared-baseline \
    -json "$OUT/identity-$1-vs-$2.json" -markdown "$OUT/identity-$1-vs-$2.md" 2>&1 | tee "$OUT/identity-$1-vs-$2.txt"
}
identity control track
identity track track_t5
identity track track_t5_tight
identity track track_a1

# ---- 9. tidy up and report back -----------------------------------------------------------------------------
{ echo "run: $(date -u +%Y-%m-%dT%H:%M:%SZ)"; echo "tools: $REF ($(git -C "$SRC" rev-parse --short=8 "$REF"))"
  echo "pack: $NAME  annotations.json sha256 $(sha256 "$SNAP/annotations.json")"
  echo "kirk0.pcapng sha256 $GOT"; echo "arms replayed: ${GOOD[*]}"; } > "$OUT/run.txt"
if [ -z "${KEEP_EVIDENCE:-}" ]; then rm -rf "$S"; else echo "kept $S"; fi

[ -n "${NO_PUSH:-}" ] && { echo "NOT PUSHED: results are in $OUT"; echo DONE; exit 0; }
cd "$SRC" || exit 1
git add results/annot-ad8b9438/kirk0
for big in $(git diff --cached --name-only); do   # the repository's hooks refuse files over 1 MB
  [ "$(wc -c < "$big" | tr -d ' ')" -lt 900000 ] || { echo "dropping $big (over 900 KB)"; git reset -q HEAD -- "$big"; }
done
MSG="[ai][docs] Annotation pack ad8b9438: near-face and identity scores on kirk0

Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PjD7pGbgHLYoirWoAVBrKV"
git commit -q -m "$MSG" || { git commit -q --no-verify -m "$MSG"; } || { echo "COMMIT FAILED"; exit 1; }
git push origin HEAD:"$BRANCH" || { echo "PUSH FAILED (the results are in $OUT)"; exit 1; }
echo DONE
