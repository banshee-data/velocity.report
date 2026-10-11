#!/usr/bin/env bash
# go-test-shard.sh - run one shard of the Go test suite, exactly as go-ci.yml does.
#
# Usage:
#   scripts/go-test-shard.sh <shard>          run the shard, writing coverage-<shard>.out
#   scripts/go-test-shard.sh --list <shard>   print the shard's packages and exit
#   scripts/go-test-shard.sh --check          prove every package lands in exactly one shard
#
# Every package lands in exactly one shard. The named shards claim packages by
# pattern and "other" takes whatever is left, so a new package is tested on the
# next PR without anyone editing a list. (The hand-kept lists this replaces had
# drifted: internal/report, internal/config, internal/scene, internal/tailscale
# and internal/cmd/lidar were tested only by the nightly run.)
#
# The split follows measured cost, not architecture. replayeval replays kirk0
# for about eighteen minutes under -race on a slow runner, so it is cut in two
# by test name; -run and -skip take the same pattern, so each top-level test
# runs in exactly one half. Re-balance from a CI log when a shard drifts past
# the others: `go test -json` per-test Elapsed is the evidence.

set -euo pipefail

MODULE="github.com/banshee-data/velocity.report"
SHARDS="core replay-a replay-b lidar-a lidar-b lidar-rest api other"

# Package patterns, matched against the import path with the module prefix removed.
CORE='^internal/(db|radar|serialmux|security|units|timeutil|httputil|testutil|fsutil|monitoring|ctl|cmd/device)(/|$)'
REPLAY='^internal/lidar/replayeval$'
# lidar-a and lidar-b hold the packages that take three minutes or more under
# -race. server is the largest, so it shares a shard with one package: with
# -p 2 the two start together rather than server waiting for a slot.
LIDAR_A='^internal/lidar/(server|l1packets/network)$'
LIDAR_B='^internal/lidar/(segments|lidarbench|storage/sqlite|pcapsplit)$'
LIDAR='^(internal/lidar|internal/cmd/(root|server|tune)|cmd/velocity)(/|$)'
API='^internal/api(/|$)'

# The replayeval tests that make up replay-a: about half the package's time.
REPLAY_A_TESTS='^(TestWarmupPrefixAndScoringManifest|TestObservationStoragePreservesReplayBaseline|TestCampaignMetricsPreserveTrackingBaseline)$'

all_packages() {
	# The module root itself becomes ".", so "./." names it on the command line.
	go list -tags=pcap ./... | sed -e "s#^${MODULE}\$#.#" -e "s#^${MODULE}/##"
}

# packages <shard>: the shard's packages, one per line, module-relative.
packages() {
	local all
	all=$(all_packages)
	case "$1" in
	core) grep -E "$CORE" <<<"$all" ;;
	replay-a | replay-b) grep -E "$REPLAY" <<<"$all" ;;
	lidar-a) grep -E "$LIDAR_A" <<<"$all" ;;
	lidar-b) grep -E "$LIDAR_B" <<<"$all" ;;
	lidar-rest) grep -E "$LIDAR" <<<"$all" | grep -vE "$REPLAY|$LIDAR_A|$LIDAR_B" ;;
	api) grep -E "$API" <<<"$all" ;;
	other) grep -vE "$CORE|$LIDAR|$API" <<<"$all" ;;
	*)
		echo "unknown shard: $1 (want one of: $SHARDS)" >&2
		exit 2
		;;
	esac || true
}

check() {
	local all assigned
	all=$(all_packages | sort)
	assigned=$(for s in $SHARDS; do
		# replay-b repeats replay-a's package with the complementary test filter.
		[ "$s" = replay-b ] && continue
		packages "$s"
	done | sort)
	if [ "$all" != "$assigned" ]; then
		echo "shard assignment is not a partition of the module's packages:" >&2
		diff <(echo "$all") <(echo "$assigned") >&2 || true
		exit 1
	fi
	echo "ok: $(wc -l <<<"$all" | tr -d ' ') packages, each in exactly one shard"
}

run() {
	local shard="$1" pkgs
	pkgs=$(packages "$shard" | sed "s#^#./#")
	if [ -z "$pkgs" ]; then
		echo "shard $shard has no packages" >&2
		exit 1
	fi

	# -race everywhere it ran before. api and other run without it: api's race
	# run lives in nightly-full-ci.yml, and other was not run on PRs at all.
	local -a args=(-tags=pcap -covermode=atomic -coverprofile="coverage-${shard}.out")
	case "$shard" in
	core | replay-a | replay-b | lidar-a | lidar-b | lidar-rest) args+=(-race) ;;
	esac
	case "$shard" in
	lidar-a | lidar-b | lidar-rest) args+=(-p 2) ;;
	esac
	case "$shard" in
	replay-a) args+=(-run "$REPLAY_A_TESTS") ;;
	replay-b) args+=(-skip "$REPLAY_A_TESTS") ;;
	esac
	# The replay shards each hold one long-running package; Go's default
	# 10-minute per-package timeout is too tight for them on a slow runner.
	args+=(-timeout 25m)

	echo "shard $shard: $(wc -l <<<"$pkgs" | tr -d ' ') packages"
	# shellcheck disable=SC2086 # one package per word
	go test "${args[@]}" $pkgs
}

case "${1:-}" in
--list)
	packages "${2:?usage: $0 --list <shard>}"
	;;
--check)
	check
	;;
"" | -h | --help)
	sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
	echo "shards: $SHARDS"
	;;
*)
	run "$1"
	;;
esac
