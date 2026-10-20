# What the published box model cost: an A/B of #764 to #777 on a quiet host

The box model went on by default in #765, and the same pull request recaptured the Mac perf
baselines. A tuning change moves the fingerprint, the comparator refuses the old baseline, and the
new numbers become the reference, so the change's cost was never compared with the code before it.
CI had no baseline at all until #779, so the nightly could only check the frame budget. The one
measurement is the devlog's: 1 to 3 % of wall time and 0.1 ms of mean frame time on kirk0, from the
box model's own development runs. This plan measures it with the A/B gate from #779, on a host
doing nothing else, and records the answer.

- **Status:** Parked (D-32, October 20, 2026): its rows are queued or in v0.5.7 and later, or it has none, and nothing in v0.5.2 to v0.5.6 needs it; revisit when the slot holding its rows becomes next. Was: Planned (2026-10-18)
- **Layers:** L5 Tracks (the box model), perf harness
- **Target:** one sitting on hydra, about 40 minutes of machine time
- **Depends on:** #779 (`scripts/perf-ab.sh`, `make test-perf-ab`)
- **Canonical:** [performance regression testing](../lidar/operations/performance-regression-testing.md#ab-the-cost-of-a-change)

## 1. The comparisons

Each is a pull request's head against the commit before it; the commits are linear squash merges.
Every row runs both gated profiles. `l3-only` stops before L5, so it is the control: a box-model
cost that shows there is noise, not cost.

| #   | Base       | Head       | What it measures                                        | Expected (full)          |
| --- | ---------- | ---------- | ------------------------------------------------------- | ------------------------ |
| A0  | `0504248c` | `0504248c` | The host's noise: identical code on both sides          | 1.00                     |
| A1  | `21c7d4b4` | `fd48d1b4` | #764: the box model built, off by default               | 1.00: the off path       |
| A2  | `fd48d1b4` | `79ac3fee` | #765: the box model on by default                       | 1.01 to 1.03, per devlog |
| A3  | `79ac3fee` | `3acd80aa` | #766: an option added, off; the fingerprint moved again | 1.00                     |
| A4  | `fd48d1b4` | `0504248c` | Everything since the box model went on, to today's main | A2, plus #767 to #777    |

A0 comes first: its spread is what every other row is read against, and it sets the threshold
the A/B gate should use (section 4).

## 2. Running it

On hydra, with nothing else running: no scene batch, no replay server, no other benchmark.
Paced replays are insensitive to a shared host until the pipeline falls behind; a benchmark is not.

```bash
# A clean clone; perf-ab.sh makes its own worktree for the base.
git clone https://github.com/banshee-data/velocity.report.git ~/perf-box-model
cd ~/perf-box-model && git lfs pull --include internal/lidar/perf/pcap/kirk0.pcapng

# The script and comparator come from main (#779); the commits being measured predate them.
cp scripts/perf-ab.sh scripts/perf_ab_compare.py /tmp/

run() { # name base head
	git checkout -q "$3"
	OUT="$HOME/perf-box-model-runs/$1" ROUNDS=9 /tmp/perf-ab.sh "$2"
}
run A0 0504248c 0504248c
run A1 21c7d4b4 fd48d1b4
run A2 fd48d1b4 79ac3fee
run A3 79ac3fee 3acd80aa
run A4 fd48d1b4 0504248c
```

Where Git LFS is not installed, set `PCAP=` to a copy of kirk0 whose digest matches the pointer
(`sha256:2864ebde…`, 200,657,872 bytes). Nine rounds is the CI capture's repeat count; with both
profiles and the warm-up pair a row is 40 replays, about 8 minutes on a CI runner.

`perf-ab.sh` exits 1 when a row exceeds its threshold. That is a finding here, not a failure:
record it and carry on to the next row.

## 3. What to record

For each row, `summary.md` from its output directory: the median per-round head ÷ base ratio for
wall clock, mean and p95 frame time, and both sides' work counters. Also record hydra's CPU, core
count, Go version, and whether the host was otherwise idle.

A2's work counters should be equal on both sides: the box model reads the tracker's output and
changes no track (#764 measured the tracks byte-identical). If `clusters` or
`distinct_confirmed_tracks` differ between A2's sides, the box model is not the only thing #765
changed, and the cost belongs to whatever else moved.

## 4. Decisions, written before the runs

- **The box model's cost.** If A2's full-profile wall-clock ratio is 1.05 or under and l3-only's
  is within A0's spread, the cost is accepted as measured and the devlog's figure is confirmed or
  corrected. Above 1.05, open a `perf` experiment for the Pi timing that #765 left owed, with A2's
  ratio as the reason; the Pi is the target, and the Mac's figure was never evidence for it.
- **The off path.** A1 and A3 above A0's spread mean an option that is off still costs something,
  a defect in its own right; record which and open an issue.
- **The A/B threshold.** The gate ships at 1.10. If A0's per-round ratios on hydra and the first
  CI run (0.999 full, 1.000 l3-only, 5 rounds) stay within ±1 %, tighten the gate to 1.05 in the
  report's pull request. The knobs hold the fraction above 1, so that is `0.05`, not `1.05`: the
  Makefile's `PERF_AB_THRESHOLD`, the workflow's `THRESHOLD` env (which `perf-ab.sh` reads), and
  `perf_ab_compare.py`'s `--threshold` default. At 1.10 the gate would pass A2 even at the top of
  the devlog's range.

## 5. Output

A report at `docs/lidar/operations/box-model-perf-cost-2026-10.md` with the five rows, A0's
spread, the decisions above, and the host; a devlog line; and, if section 4 says so, the threshold
change and the follow-up experiment. Raw run directories stay on hydra: they are evidence of one
sitting, not something the repository keeps.
