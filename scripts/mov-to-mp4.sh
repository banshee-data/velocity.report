#!/usr/bin/env bash
# mov-to-mp4.sh — Convert a .mov screen recording to .mp4 for web playback.
#
# Video-only (strips audio). Visually lossless H.264 with CRF 18.
# Optional speed-up for long recordings.
#
# Usage:
#   scripts/mov-to-mp4.sh [OPTIONS] <input.mov>
#
# Options:
#   --out <file>        Output path        (default: <input>.mp4)
#   --speed <N>         Speed multiplier   (default: 1, e.g. 2, 4, 0.5)
#   --crf <0-51>        Quality            (default: 18, lower = better)
#   -h, --help          Show this help
#
# Dependencies: ffmpeg (brew install ffmpeg)

set -euo pipefail

RED='\033[0;31m'
GRN='\033[0;32m'
CYN='\033[0;36m'
BLD='\033[1m'
RST='\033[0m'

info()  { printf "${CYN}▸ %s${RST}\n" "$*"; }
ok()    { printf "${GRN}✓ %s${RST}\n" "$*"; }
die()   { printf "${RED}error: %s${RST}\n" "$*" >&2; exit 1; }

OUTPUT=""
SPEED=1
CRF=18
INPUT=""

if [[ $# -eq 0 ]]; then
    sed -n '/^# Usage:/,/^$/p' "$0" | sed -E 's/^# ?//'
    exit 1
fi

while [[ $# -gt 0 ]]; do
    case "$1" in
        --out)    shift; OUTPUT="$1" ;;
        --speed)  shift; SPEED="$1" ;;
        --crf)    shift; CRF="$1" ;;
        -h|--help)
            sed -n '/^# mov-to-mp4/,/^[^#]/p' "$0" | sed -E 's/^# ?//' | sed '$d'
            exit 0
            ;;
        -*)  die "Unknown option: $1" ;;
        *)
            [[ -n "$INPUT" ]] && die "Unexpected argument: $1"
            INPUT="$1"
            ;;
    esac
    shift
done

[[ -z "$INPUT" ]] && die "No input file specified. Run with --help for usage."
[[ -f "$INPUT" ]] || die "Input file not found: $INPUT"
[[ -z "$OUTPUT" ]] && OUTPUT="${INPUT%.*}.mp4"
command -v ffmpeg >/dev/null 2>&1 || die "ffmpeg not found (brew install ffmpeg)"

IN_SIZE=$(du -sh "$INPUT" | cut -f1)
printf "${BLD}Source:${RST} %s  (%s)\n" "$INPUT" "$IN_SIZE"

# Build the PTS filter for speed adjustment.
# setpts=PTS/N speeds up by Nx (halves timestamps).
PTS_FILTER="setpts=PTS/${SPEED}"

info "ffmpeg: video-only, H.264 crf=${CRF}, speed=${SPEED}x → ${OUTPUT}"

ffmpeg -y -i "$INPUT" \
    -an \
    -vf "$PTS_FILTER" \
    -c:v libx264 -crf "$CRF" -preset slow \
    -pix_fmt yuv420p \
    -movflags +faststart \
    "$OUTPUT"

OUT_SIZE=$(du -sh "$OUTPUT" | cut -f1)
ok "Done: ${OUTPUT}  (${OUT_SIZE})"
