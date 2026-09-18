# LiDAR parameter/algorithm experiment campaign, 2026-09

- **Status:** Batch 1 prepared, not yet launched.
- **Scope:** Execute the backlog in [data/experiments/try/](../../data/experiments/try/) against the
  now-complete 24-site S2 corpus (312,315 frames; see
  [state-estimation-phase01-corpus-baseline.md](../lidar/operations/state-estimation-phase01-corpus-baseline.md))
  using the harness that already exists. This doc is the execution plan: what
  runs, in what order, why that order, and what each batch actually proves.
- **Related:** [lidar-parameter-tuning-optimisation-plan.md](lidar-parameter-tuning-optimisation-plan.md)
  (evergreen design doc this campaign executes against),
  [lidar-heading-coherence-sprint-plan.md §5.2](lidar-heading-coherence-sprint-plan.md)
  (harness notes, the disk-contention finding and its fix),
  [lidar-state-estimation-plan.md](lidar-state-estimation-plan.md) (Phase 0/1 status).

## Why this doc exists

We have ~44 hours of unattended compute and very little operator time. The
existing `try/*.md` experiment definitions were written against a tool
(`pcap-analyse`) that has since been removed and a `GroundTruthEvaluator` that
needs manually labelled reference tracks that mostly don't exist. Running the
docs as literally written would either fail immediately or produce results
that look rigorous but aren't. This doc reconciles the backlog against what
the codebase can actually do today, in priority order, and gives one
copy-pasteable command per batch.

## What changed since the `try/*.md` docs were written

| Finding                                                   | Detail                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `pcap-analyse` is gone                                    | Removed from the tree; only a stale gitignored binary from March remains (`./pcap-analyse`). All three per-layer sweep docs and the multi-key doc named it as the tool to use. Replaced references with the current equivalents: `settling-eval` (L3), `tune sweep -mode tracking` (L5, live-server only), `lidar-closeness-audit` (L3, needs bg snapshots that don't exist yet — see below).                                                                                                                                                                                                                                                                                                            |
| Config key spelling                                       | `l3-background-settling-sweep.md` named `safety_margin_meters` / `neighbor_confirmation_count`; the actual keys in [config/tuning.defaults.json](../../config/tuning.defaults.json) are `safety_margin_metres` / `neighbour_confirmation_count` (British spelling, matching the rest of the config). Fixed in the doc.                                                                                                                                                                                                                                                                                                                                                                                   |
| `Fragmentation` isn't implemented                         | `GroundTruthEvaluator.EvaluateGroundTruth` ([internal/lidar/adapters/ground_truth.go:287](../../internal/lidar/adapters/ground_truth.go)) hardcodes `Fragmentation = 0.0` with a comment marking it a future enhancement. Every `try/*.md` doc listing "fragmentation rate" as a gated metric available today is wrong; it isn't computed.                                                                                                                                                                                                                                                                                                                                                               |
| No multi-site ground truth                                | `GroundTruthEvaluator` scores stored `RunTrack` rows with a human-set `UserLabel`. Checked `sensor_data.db`: 9 analysis runs have any labelled tracks at all (largest has 64), and all predate this campaign — almost certainly all kirk0, none of the 21 new S2 sites. The L3/L4/L5/multi-key/velocity-coherent docs all gate their real acceptance criteria on this data. It doesn't exist for the new corpus and manual labelling wasn't in scope for the 44-hour window.                                                                                                                                                                                                                             |
| HINT is human-in-the-loop, not batchable                  | `internal/lidar/sweep/hint.go` alternates automated rounds with a live labelling window in the Runs/Tracks web UI (`POST /api/lidar/sweep/hint` / `/hint/continue`). It has no CLI entry point and cannot run unattended — a round needs someone to actually label tracks between automated passes. Real advantage over brute force (adaptive bound narrowing against real GT), but only usable when an operator is present, so it isn't part of the unattended batches below.                                                                                                                                                                                                                           |
| `tune sweep` always needs a live server                   | All 5 modes (`multi`/`noise`/`closeness`/`neighbour`/`tracking`) drive a running server over HTTP; none run standalone. `tracking` mode also only reaches whatever PCAPs are under the _running server's_ `-lidar-pcap-dir` — today that's `kirk0.pcapng`, not the S2 corpus on `/Volumes/lidar`. This is why the L5 preliminary pass (see below) is kirk0-only.                                                                                                                                                                                                                                                                                                                                         |
| L5's own preliminary pass is confirmed circular           | The pass already run and documented in [l5-tracking-noise-parameter-sweep.md](../../data/experiments/try/l5-tracking-noise-parameter-sweep.md#preliminary-pass-2026-09-17-tune-sweep--mode-tracking-on-kirk0) scores `measurement_noise` against the tracker's own smoothed heading — the same "locked heading has near-zero jitter" pathology as RC6. Its finding (raise `measurement_noise`) is explicitly not a recommendation.                                                                                                                                                                                                                                                                       |
| `settling-eval` is real, offline, and non-circular        | [internal/cmd/lidar/settling.go](../../internal/cmd/lidar/settling.go) replays one PCAP through a standalone `BackgroundManager` — no live server, no track labels — and reports convergence computed from the background grid's own measured state (coverage rate, spread-delta rate, region stability, mean confidence; see [SettlingReport](../../internal/lidar/l3grid/settling_report.go)). Smoke-tested against a real S2 corpus capture: 120s of replay (1200 frames) in 11.6s wall time, correctly detects convergence at frame 12 under current defaults. This is the one experiment in the backlog that can produce real, non-circular, 24-site evidence _today_ with zero new implementation. |
| `lidar-closeness-audit` needs data that doesn't exist yet | It audits `lidar_bg_snapshot` rows against the sensor's physical range-accuracy spec — genuinely independent evidence, better than settling-eval for the closeness threshold specifically. But it reads snapshots, and every observations.db checked (all 24-site runs plus five earlier state-estimation runs) has zero `lidar_bg_snapshot` rows: `snapshot_interval` defaults to 2h, far longer than any of these replay windows, so nothing ever gets flushed. Usable later with a short, dedicated capture pass (lower `snapshot_interval`, e.g. to a few seconds, for one deliberate run per site) — not free today, deferred to Batch 4.                                                           |
| `velocity-coherent` extractor doesn't exist               | No `velocity_coherent` engine anywhere in `internal/lidar/l4perception` or the config engine registry — only `dbscan_xy_v1`. [velocity-coherent-baseline-comparison.md](../../data/experiments/try/velocity-coherent-baseline-comparison.md) is blocked on an implementation that was never built, independent of the ground-truth gap. Not part of this campaign; flagged as out of scope until someone builds the extractor.                                                                                                                                                                                                                                                                           |

## Campaign ordering and rationale

Per the instruction this campaign follows: cheap sanity checks first, then
broad exploration, then multi-site confirmation of anything promising, then
narrowed/HINT-assisted search only where an operator is present, then
cross-layer interactions last (they're explicitly gated on per-layer results
in `multi-key-interaction-grid.md`). Each batch is sized 1.5–4 hours; every
individual (site, config) result is kept regardless of what later batches
show.

| Batch | What                                                                                                                                                                                      | Runs unattended?                                                                                                 | Depends on                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ----- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **1** | L3 background-settling broad sweep, all 24 sites, `settling-eval`                                                                                                                         | Yes                                                                                                              | Nothing — ready now                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| **2** | L3 confirmation pass: narrow to the ranges Batch 1 flags as sensitive, repeat on all 24 sites with tighter steps + a second capture segment per site (determinism/replicate check)        | Yes                                                                                                              | Batch 1 results                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| **3** | L5 ground-truth-scored noise sweep on kirk0 (replacing the circular alignment metric with the real `GroundTruthEvaluator`, using the 64-track labelled reference run that already exists) | Needs one small CLI extension (below); the sweep itself is unattended once built                                 | A ~20-line CLI wrapper around `adapters.EvaluateGroundTruth` — nothing existing exposes it outside the live HINT flow                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| **4** | L3 closeness-audit pass: one dedicated low-`snapshot_interval` replay per site, then `lidar-closeness-audit` against the resulting snapshots                                              | Yes                                                                                                              | Batches 1–2 (don't burn a second full pass across all 24 sites on the closeness key if Batch 1 already shows it's insensitive)                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| **5** | Multi-key interaction grid (L3 × L4 × L5, top 3–4 sensitive keys)                                                                                                                         | Yes, but small — only worth running if Batches 1–4 found ≥2 keys with real, consistent, non-circular sensitivity | Batches 1–4                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| —     | L4 clustering sweep                                                                                                                                                                       | Not scheduled                                                                                                    | No offline, non-circular L4 metric exists yet (no equivalent of `settling-eval` for L4; GroundTruthEvaluator needs labels this corpus doesn't have). Options to unblock, for a future check-in to choose between: (a) reuse `lidar-e1-analysis`'s cross-measurement disagreement as a weaker proxy, (b) build a small offline L4 metric (cluster count/size stability across repeat runs), (c) wait for HINT with an operator present. Not attempted unattended because every option either isn't built or is weaker evidence than what Batches 1–4 give for free. |
| —     | HINT-driven optimisation                                                                                                                                                                  | Not scheduled unattended                                                                                         | Needs a human labelling round; propose when you're at the keyboard, not as an overnight batch                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| —     | velocity-coherent baseline comparison                                                                                                                                                     | Blocked                                                                                                          | Extractor not implemented                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |

Batches 3–5 are not launched today; they're written up now so a later
check-in can approve or redirect them without re-deriving this analysis.

---

## Batch 1 — L3 background-settling broad sweep (ready now)

**Experiment:** [l3-background-settling-sweep.md](../../data/experiments/try/l3-background-settling-sweep.md)
(doc updated to match: correct key names, `settling-eval` in place of
`pcap-analyse`, settling metrics in place of the unimplemented
fragmentation/GT metrics).

**Harness:** [data/experiments/try/l3-settling-sweep/run_sweep.py](../../data/experiments/try/l3-settling-sweep/run_sweep.py) —
new, but it's a thin orchestration script, not a new experiment framework: it
shells out to the existing `settling-eval` binary once per (site, config)
combination, using the site→PCAP mapping already recorded in the corpus'
own manifests
(`/Volumes/lidar/lidar/manifests/state-estimation-phase01-24site-20260917.source-pcaps.json`
and `...-rebased-20260913.source-pcaps.json`, covering all 24 sites between
them). No new Go code, no new evaluator.

**What it sweeps:** the four keys named in the experiment doc —
`closeness_multiplier`, `safety_margin_metres`, `noise_relative`,
`neighbour_confirmation_count` — each at 4 non-default points spanning the
doc's stated range, plus one default-config baseline row per site. 24 sites ×
17 configs = 408 runs.

**What it measures (non-circular):** `settling-eval`'s report is computed
from the background grid's own measured per-cell statistics — coverage rate,
spread-delta rate, region stability, mean confidence, and the frame at which
all four cross the settling thresholds already defined in
`tuning.defaults.json`. None of this touches a downstream track or a
comparison against the algorithm's own smoothed output, so it doesn't have
the RC6 self-scoring problem the L5 pass has.

**Resumability:** re-running the same command skips any (site, key, value)
already present in `results.csv`. Safe to kill and restart. Progress is
readable from `summary.json` (rewritten every 10 rows) without touching the
full CSV or the per-run raw JSON reports.

**Outputs:**

- `data/experiments/try/l3-settling-sweep/results.csv` — one row per run:
  timestamp, exact git SHA, site ID, swept key/value, capture relative path +
  sha256, recommended settling frame, converged flag, final coverage/spread/
  stability/confidence, wall time. Committed to git (small, like the existing
  L5 sweep CSV).
- `data/experiments/try/l3-settling-sweep/summary.json` — rolling aggregate
  (mean/min/max settling frame and non-convergence count per key). Committed.
- `data/experiments/try/l3-settling-sweep/raw/*.json` — full per-run
  `settling-eval` report (full metrics history). Local only, gitignored:
  cheap and deterministic to regenerate from the row's git SHA + config +
  capture sha256, so not worth ~130MB in git.

**Validated:** dry-run on 2 sites (34 runs, 30s window) completed cleanly,
zero errors, and already shows a real, plausible, non-flat signal —
`neighbour_confirmation_count` at higher values pushed settling from 12
frames to as late as 301 and produced the run's only non-convergence, while
`safety_margin_metres` showed no movement at all across its swept range on
those 2 sites. That's exactly the kind of per-key sensitivity signal Batch 2
should confirm or refute across all 24.

## Batch 2 — confirmation pass (conditional on Batch 1)

Same harness, same sites, run again with:

- The swept range narrowed around whatever Batch 1 flags as sensitive
  (don't re-run a key that showed zero movement across its full doc-specified
  range on all 24 sites — that's itself a finding: current default is robust,
  record it and move on).
- A second capture segment (ordinal 1, where available) per site, to check
  the finding isn't an artifact of the specific 120s window chosen.

Exact scope will be written once Batch 1 completes — this section is a
placeholder for that write-up, not a command to run yet.

## Batch 3 — L5 noise sweep, ground-truth-scored, kirk0 (needs a small extension)

The existing preliminary pass measured the wrong thing. The right evaluator
(`adapters.GroundTruthEvaluator`) already exists and is already tested, and a
labelled reference run already exists in `sensor_data.db` (64 labelled tracks
in run `60a4774c-db3e-4008-9b7e-d1059ec27319`, likely kirk0 — confirm before
running). It's only reachable today through the live HINT HTTP flow. Needed:
a ~20-line standalone CLI (or a flag on an existing tool) that takes
`-reference-run-id` and `-candidate-run-id` and calls
`adapters.NewGroundTruthEvaluator(store, adapters.DefaultGroundTruthWeights()).Evaluate(...)`,
printing the `GroundTruthScore` as JSON. Combined with `tune sweep -mode
tracking`'s existing ability to apply a tracker config and replay a PCAP
against the live server, this closes the exact gap the preliminary pass
flagged: real detection-rate/false-positive/quality scoring instead of a
metric the tracker can win by smoothing. Single-site only (kirk0) — no
labelled data exists elsewhere — so this confirms or refutes the
preliminary pass's `measurement_noise` finding but doesn't generalise across
the corpus.

## Batch 4 — L3 closeness audit (needs one dedicated capture pass)

Rerun `lidar-state-estimation-baseline` (or a lighter equivalent) for a
handful of sites with `snapshot_interval` set to a few seconds instead of 2h,
just long enough to force `lidar_bg_snapshot` rows to exist, then run
`lidar-closeness-audit` against them. Only worth doing if Batch 1 shows
`closeness_multiplier` sensitivity worth cross-checking against the sensor's
physical range-accuracy spec; otherwise Batch 1's settling evidence plus a
"no snapshot data, deferred" note is enough.

## Batch 5 — multi-key interaction grid

As specified in [multi-key-interaction-grid.md](../../data/experiments/try/multi-key-interaction-grid.md)
(doc's `pcap-analyse` reference needs the same fix as Batch 1 once this
batch is actually scoped), restricted to whichever 3–4 keys Batches 1–4 show
the steepest, most consistent sensitivity. Not scoped further until that
evidence exists.
