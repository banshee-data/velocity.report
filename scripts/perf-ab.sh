#!/usr/bin/env bash
# Benchmark a base commit against the working tree on this machine, alternately,
# and gate on what the change costs. See scripts/perf_ab_compare.py for why this
# exists alongside the committed baselines.
#
#   scripts/perf-ab.sh [BASE_REF]
#
# BASE_REF defaults to the merge base with origin/main. Environment:
#   ROUNDS      rounds per profile (default 5); each round runs base and head once
#   PROFILES    profiles to compare (default: "full l3-only", the gated ones)
#   NAME        capture under internal/lidar/perf/pcap (default kirk0)
#   PCAP        a path to that capture instead, where Git LFS is not installed
#   THRESHOLD   median per-round wall-clock ratio allowed above 1 (default 0.10)
#   OUT         where runs and the report go (default: a temporary directory)
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

BASE_REF=${1:-$(git merge-base HEAD origin/main)}
BASE_SHA=$(git rev-parse --verify "$BASE_REF^{commit}")
HEAD_SHA=$(git rev-parse HEAD)
ROUNDS=${ROUNDS:-5}
PROFILES=${PROFILES:-full l3-only}
NAME=${NAME:-kirk0}
THRESHOLD=${THRESHOLD:-0.10}
OUT=${OUT:-$(mktemp -d -t perf-ab.XXXXXX)}
mkdir -p "$OUT"

PCAP=${PCAP:-}
for ext in pcapng pcap; do
	[ -n "$PCAP" ] && break
	[ -f "internal/lidar/perf/pcap/$NAME.$ext" ] && PCAP="$ROOT/internal/lidar/perf/pcap/$NAME.$ext" && break
done
[ -n "$PCAP" ] || { echo "No capture internal/lidar/perf/pcap/$NAME.{pcapng,pcap}" >&2; exit 2; }
# A Git LFS pointer is a few hundred bytes; replaying one fails confusingly.
if [ "$(wc -c <"$PCAP")" -lt 4096 ]; then
	echo "$PCAP looks like an unfetched LFS pointer; run git lfs pull" >&2
	exit 2
fi

echo "perf A/B: base $BASE_SHA vs head $HEAD_SHA"
echo "capture $PCAP, profiles: $PROFILES, $ROUNDS rounds, threshold $THRESHOLD"
echo "runs in $OUT"

# The base is built from its own tree, so it runs its own code against its own
# tuning defaults; that is the comparison.
BASE_TREE="$OUT/base-tree"
# Only the head's capture is replayed, so the base tree skips LFS downloads.
GIT_LFS_SKIP_SMUDGE=1 git worktree add --detach --force "$BASE_TREE" "$BASE_SHA" >/dev/null
trap 'git worktree remove --force "$BASE_TREE" >/dev/null 2>&1 || true' EXIT

build() { # tree output
	(cd "$1" && ./scripts/ensure-web-stub.sh >/dev/null && ./scripts/ensure-docs-stub.sh >/dev/null &&
		go build -tags=pcap -o "$2" ./cmd/tools/lidar-bench)
}
echo "building base and head lidar-bench..."
build "$BASE_TREE" "$OUT/lidar-bench-base"
build "$ROOT" "$OUT/lidar-bench-head"

run() { # side profile round
	local tree=$ROOT
	[ "$1" = base ] && tree=$BASE_TREE
	(cd "$tree" && "$OUT/lidar-bench-$1" -pcap "$PCAP" -profile "$2" -config config/tuning.defaults.json \
		-max-frames-over-budget-pct 100 -progress 0 -quiet \
		-benchmark-output "$OUT/runs/$2/$(printf %02d "$3")-$1.json" >/dev/null)
}

for profile in $PROFILES; do
	mkdir -p "$OUT/runs/$profile"
	# One unrecorded run each warms the page cache and the CPU governor, so the
	# first measured round is not the one that paid for them.
	run base "$profile" 99 && run head "$profile" 99
	rm -f "$OUT/runs/$profile/99-"*.json
	for ((i = 0; i < ROUNDS; i++)); do
		# ABBA: alternate which side goes first, so neither always runs on a
		# host the other has just warmed.
		if ((i % 2 == 0)); then
			run base "$profile" "$i" && run head "$profile" "$i"
		else
			run head "$profile" "$i" && run base "$profile" "$i"
		fi
		echo "  $profile round $((i + 1))/$ROUNDS done"
	done
done

python3 "$(dirname "${BASH_SOURCE[0]}")/perf_ab_compare.py" --threshold "$THRESHOLD" \
	--summary "$OUT/summary.md" "$OUT/runs"
