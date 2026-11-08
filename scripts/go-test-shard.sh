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
# past the 25-minute package timeout under -race, so it is cut in three by
# test name. replay-b skips the union of replay-a and replay-c, so each
# top-level test runs once. Re-balance from a CI log when a shard drifts past
# the others: `go test -json` per-test Elapsed is the evidence.

set -euo pipefail

MODULE="github.com/banshee-data/velocity.report"
SHARDS="core replay-a replay-b replay-c lidar-a lidar-b lidar-rest api other"

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

# The original long-running replay tests stay in replay-a. Move observation
# frame/tap, likelihood and fragment-join replays to replay-c: replay-b timed
# out after 25 minutes on #986 while replay-a took 9.4 minutes. These tests
# compile under -race; the full solid-body evidence tests explicitly do not.
REPLAY_A_TESTS='^(TestWarmupPrefixAndScoringManifest|TestObservationStoragePreservesReplayBaseline|TestCampaignMetricsPreserveTrackingBaseline)$'
REPLAY_C_TESTS='^(TestObservationFrames.*|TestFrameTapReadsWithoutChanging|TestLikelihoodCostsOnKirk0|TestFragmentJoinTrackMembershipOnKirk0)$'
REPLAY_NAMED_TESTS="$REPLAY_A_TESTS|$REPLAY_C_TESTS"

replay_args() {
	case "$1" in
	replay-a) printf '%s\n' -run "$REPLAY_A_TESTS" ;;
	replay-b) printf '%s\n' -skip "$REPLAY_NAMED_TESTS" ;;
	replay-c) printf '%s\n' -run "$REPLAY_C_TESTS" ;;
	esac
}

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
	replay-a | replay-b | replay-c) grep -E "$REPLAY" <<<"$all" ;;
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
		# The replay shards repeat the package with different test filters.
		[[ "$s" = replay-b || "$s" = replay-c ]] && continue
		packages "$s"
	done | sort)
	if [ "$all" != "$assigned" ]; then
		echo "shard assignment is not a partition of the module's packages:" >&2
		diff <(echo "$all") <(echo "$assigned") >&2 || true
		exit 1
	fi
	echo "ok: $(wc -l <<<"$all" | tr -d ' ') packages, each in exactly one shard"
	check_replay
	check_replay -race
}

check_replay() {
	# Check the replay filter patterns too: no test may be duplicated or
	# omitted when a named group changes. -list ignores -run and -skip,
	# so use the run pattern as -list's pattern and filter the skip pattern.
	local tests selected shard
	local -a filters
	tests=$(go test "$@" -tags=pcap -list '^Test' ./internal/lidar/replayeval | grep '^Test' | sort)
	selected=$(for shard in replay-a replay-b replay-c; do
		mapfile -t filters < <(replay_args "$shard")
		if [ "${filters[0]}" = -run ]; then
			go test "$@" -tags=pcap -list "${filters[1]}" ./internal/lidar/replayeval | grep '^Test'
		else
			grep -vE "${filters[1]}" <<<"$tests"
		fi
	done | sort)
	if [ "$tests" != "$selected" ]; then
		echo "replay test assignment is not a partition:" >&2
		diff <(echo "$tests") <(echo "$selected") >&2 || true
		exit 1
	fi
	echo "ok: $(wc -l <<<"$tests" | tr -d ' ') replay tests (${*:-without race}), each in exactly one shard"
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
	core | replay-a | replay-b | replay-c | lidar-a | lidar-b | lidar-rest) args+=(-race) ;;
	esac
	case "$shard" in
	lidar-a | lidar-b | lidar-rest) args+=(-p 2) ;;
	esac
	if [[ "$shard" = replay-* ]]; then
		local -a filters
		mapfile -t filters < <(replay_args "$shard")
		# Include per-test durations so future rebalancing can use CI logs.
		args+=(-v "${filters[@]}")
	fi
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
