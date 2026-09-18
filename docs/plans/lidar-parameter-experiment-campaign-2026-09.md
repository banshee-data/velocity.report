# LiDAR parameter/algorithm experiment campaign, 2026-09

- **Status:** Batch 1 running (started 2026-09-17 23:12, operator-launched, unattended).
  An unattended supervisor (below) is prepared to wait for it, then run Batches 2 and
  5 and attempt Batch 3, for up to a configurable wall-clock budget (default 12h).
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
| No multi-site ground truth                                | `GroundTruthEvaluator` scores stored `RunTrack` rows with a human-set `UserLabel`. Checked `sensor_data.db`: 9 analysis runs have any labelled tracks at all (largest has 64), and all predate this campaign by months, with no stored `run_config_id`. Confirmed via `lidar_run_records.source_path` (not assumed): 2 are kirk0.pcapng, 5 are kirk1.pcapng, 1 is clar0.pcapng, 1 is an S2 site (`s2_sf_4_20260902153250_00003.pcap`) — none of the 21 new S2 corpus sites. The L3/L4/L5/multi-key/velocity-coherent docs all gate their real acceptance criteria on this data. It doesn't exist for the new corpus and manual labelling wasn't in scope for the 44-hour window.                         |
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
| **3** | L5 ground-truth-scored noise sweep on kirk1 (replacing the circular alignment metric with the real `GroundTruthEvaluator`, using the 64-track labelled reference run that already exists) | CLI built and tested; sweep itself blocked on a confirmed UDP port conflict (below)                              | Self-blocks with a written reason via a preflight check; not attempted unattended until resolved                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| **4** | L3 closeness-audit pass: one dedicated low-`snapshot_interval` replay per site, then `lidar-closeness-audit` against the resulting snapshots                                              | Yes                                                                                                              | Batches 1–2 (don't burn a second full pass across all 24 sites on the closeness key if Batch 1 already shows it's insensitive)                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| **5** | Multi-key interaction grid (L3 × L4 × L5, top 3–4 sensitive keys)                                                                                                                         | Yes, but small — only worth running if Batches 1–4 found ≥2 keys with real, consistent, non-circular sensitivity | Batches 1–4                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| —     | L4 clustering sweep                                                                                                                                                                       | Not scheduled                                                                                                    | No offline, non-circular L4 metric exists yet (no equivalent of `settling-eval` for L4; GroundTruthEvaluator needs labels this corpus doesn't have). Options to unblock, for a future check-in to choose between: (a) reuse `lidar-e1-analysis`'s cross-measurement disagreement as a weaker proxy, (b) build a small offline L4 metric (cluster count/size stability across repeat runs), (c) wait for HINT with an operator present. Not attempted unattended because every option either isn't built or is weaker evidence than what Batches 1–4 give for free. |
| —     | HINT-driven optimisation                                                                                                                                                                  | Not scheduled unattended                                                                                         | Needs a human labelling round; propose when you're at the keyboard, not as an overnight batch                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| —     | velocity-coherent baseline comparison                                                                                                                                                     | Blocked                                                                                                          | Extractor not implemented                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |

Batches 3–5 are not launched today; they're written up now so a later
check-in can approve or redirect them without re-deriving this analysis.

---

## Unattended supervisor (2026-09-18)

Batch 1 was launched by hand (below) and finished on its own after ~76
minutes (408/408 rows, zero errors) while Batches 2–5 were being built. To
avoid needing an operator to launch each subsequent batch by hand,
[data/experiments/try/campaign/](../../data/experiments/try/campaign/) adds
a small supervisor that reuses the exact scripts above rather than a new
framework:

- **`manifest.json`** — the ordered stage graph (wait → analyze → narrowed
  sweep → replicate sweep → analyze consistency → L5 GT sweep → interaction
  grid → finalize), each with a `status`, `depends_on`, and `params`. This is
  the "future-work manifest": `supervisor.py` rereads it fresh at the start
  of every pass, so any still-`pending` stage's params can be edited between
  check-ins without touching stages that already reached a terminal status
  (`done`/`done_no_op`/`blocked`/`failed`/`skipped`).
- **`supervisor.py`** — tries every currently-eligible pending stage once
  per pass (so, e.g., the three Go builds run immediately rather than
  waiting behind a slow sweep), sleeps and retries when nothing is eligible,
  and stops once every stage is terminal or the wall-clock budget
  (`budget_hours`, default 12) is spent.
- **`status.json`** — compact rollup written after every stage transition:
  per-stage status plus a one-line result summary, small enough to read at
  a check-in without opening the manifest or any raw results.

**Safety properties, verified by hand before handoff, not just assumed:**

- `wait_process` only ever polls for the operator-launched Batch 1 process;
  it never starts, restarts, or signals it. Verified against the real
  running process (correctly returned "not ready yet" while it was running,
  correctly reported the true final row count once it exited on its own).
- Every stage that itself launches a `run_sweep.py` follow-up (narrowed,
  replicate) first checks whether another `run_sweep.py` of any kind is
  already active and backs off (stays `pending`) rather than starting a
  second one. This was **not** in the first draft — it was added after a
  test pass legitimately triggered a real replicate sweep (its dependencies
  were already satisfied) that briefly ran concurrently with nothing, but
  would have raced a second copy of itself under the original
  `wait_process`-only guard. Caught and fixed before handoff, not left as a
  known gap.
- The L5 stage's port-conflict preflight check (see Batch 3 below) was
  exercised for real against the actual live server and correctly
  self-blocked with a written reason, twice, without touching anything.

### Second pass (2026-09-18 check-in)

Pass 1 finished cleanly in ~1.53h (11/11 stages `done`, `all stages terminal;
campaign complete` in the log) and then sat idle for several hours before
the operator noticed and checked in — not a hang, just no more eligible work
under the original manifest. Reviewing the accumulated evidence at that
check-in surfaced two real defects that a clean exit code had hidden:

- **`l3_interaction_grid` silently lost half its data.**
  `plan_interaction_levels.py` cast every level to `float`, so
  `neighbour_confirmation_count`'s worst value (an int field in the Go
  config) was written as `1.0`; settling-eval's strict JSON unmarshal
  rejected it, and all 48 rows at that level (24 sites × 2 `noise_relative`
  levels) errored while the other 48 (at `neighbour_confirmation_count=3`)
  succeeded. `run_interaction_grid.py` never crashes on a per-row error (by
  design — one bad site shouldn't sink the grid), so the stage still exited
  0 and was recorded `done`. Fixed at the root (an `IS_INT` cast in
  `plan_interaction_levels.py`, plus a defensive int-cast in
  `run_interaction_grid.py`'s `make_multi_config`) and in the supervisor:
  `handle_l3_interaction_grid` now fails the stage if any expected combo has
  zero non-error rows, instead of trusting the exit code alone. Verified the
  new guard against the real broken data before relying on it: it correctly
  flags the two broken combos from pass 1 and reports nothing wrong for the
  already-good ones.
- **The L5 sweep's numbers were a windowing artifact, not a real accuracy
  signal.** `l5_gt_sweep` used `duration_seconds=60` against kirk1.pcapng,
  but the reference run's capture is 177.8s
  (`lidar_run_records.duration_secs`) and `EvaluateGroundTruth`'s reference
  set (49 positive-labelled tracks) spans the whole capture regardless of
  how much of it the candidate replay covers — matched_count was only 4-5
  out of 49 reference tracks on every combo, because most reference tracks'
  time windows simply fell outside the 60s replay.

Two follow-ups were added to the manifest (not run automatically by any
LLM-in-the-loop judgement — both are mechanical extensions of already-proven
code, triggered by fixed, documented rules, same as every other stage here):

- `l3_interaction_grid_v2` (rerun with the type fix) →
  `analyze_l3_interaction_grid`, a new deterministic
  additive-vs-compounding verdict (`analyze_interaction_grid.py`): does
  pushing both sensitive keys to their worst values together compound,
  cancel, or stay additive relative to their single-key effects?
- `l5_gt_sweep_full_duration` (same reference run, `duration_seconds=190`,
  separate `out_dir` so the original 60s-window run is preserved) plus two
  **new reference sites** — `l5_gt_sweep_kirk0_dd98` and
  `l5_gt_sweep_kirk1_f069` — so the campaign isn't drawing L5 conclusions
  from a single labelled recording. Sites were selected by querying
  `sensor_data.db` for `lidar_run_records` with the most positive-labelled
  tracks and applying a `MIN_POSITIVE_LABELS=8` floor (same
  conservative-evidence philosophy as `MIN_SITE_FRACTION` elsewhere in this
  campaign); candidates below the floor (6, 3, 2, 2, 1, 1 positive tracks)
  were excluded as too noisy to be defensible. All three feed
  `analyze_l5_multisite` (`analyze_l5_results.py`), which ranks each site's
  27 combos by `composite_score` and reports whether any combo lands in
  every site's top-5 — a cross-site agreement check, not a single-site
  "winner."

Both `handle_l5_gt_sweep` and `run_l5_gt_sweep.py` already supported a
configurable `--out-dir`; the supervisor just didn't plumb it through until
this pass (needed so the three new L5 stages don't collide with each other's
`results.csv`, whose dedup keys only on the three noise params).

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

**Result (completed 2026-09-18, ~76 minutes, 408/408 rows, zero errors):**
deterministic analysis
([analyze_sensitivity.py](l3-settling-sweep/analyze_sensitivity.py), rule
documented in its own docstring) flags 2 of the 4 keys as sensitive across
the corpus, 2 as robust:

| Key                            | Verdict       | Evidence                                                                                                                                                                                                                                                                                                     |
| ------------------------------ | ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `neighbour_confirmation_count` | **Sensitive** | At `1` (doc's own stated minimum): 21/24 sites regress, mean settling delay +237 frames, one site hits the 1200-frame window ceiling without ever converging. At `2`: only 4/24 sites regress, no site failed to converge. At `4`/`5`: zero sites regress. Already at the safety floor at `1` — see Batch 2. |
| `noise_relative`               | **Sensitive** | At `0.05` (doc's stated maximum): 6/24 sites regress (25%). At `0.035`: 1/24. At `0.005`/`0.01`: none. Still moving at the edge of the tested range — Batch 2 extends past it.                                                                                                                               |
| `closeness_multiplier`         | Insensitive   | No tested value (1.5–5.0) regressed ≥ 20% of sites. Current default is robust; no further sweeping planned.                                                                                                                                                                                                  |
| `safety_margin_metres`         | Insensitive   | No tested value (0.05–0.30) regressed ≥ 20% of sites. Same conclusion.                                                                                                                                                                                                                                       |

Full per-value numbers: `sensitivity-analysis.json` in the same directory.

## Batch 2 — confirmation pass (in progress, started automatically 2026-09-18)

Two independent sub-passes, both using the same `run_sweep.py` driver (see
[plan_narrowed_sweep.py](l3-settling-sweep/plan_narrowed_sweep.py) for the
narrowing rule):

- **Narrowed sweep** (ordinal 0, `--sweep-json narrowed-sweep.json`):
  `closeness_multiplier`/`safety_margin_metres` dropped entirely (Batch 1
  conclusive — no further sweeping planned for either). `noise_relative`
  gets one new point, `0.065`, extending past the doc's original 0.05 upper
  bound since the effect was still visible right at that edge.
  `neighbour_confirmation_count` gets no new point: its worst value (`1`) is
  already the doc's stated floor and the physically-meaningful minimum (a
  confirmation count below 1 isn't meaningful), so there's nowhere lower to
  extend to — recorded as "already at floor," not a gap.
- **Replicate pass** (ordinal 1, full original 17-config sweep, no
  narrowing): checks Batch 1's findings against a second, independent
  120s window per site. 23/24 sites have a second capture segment; the
  missing one is recorded as a coverage gap, not an error.
  [analyze_replicate_consistency.py](l3-settling-sweep/analyze_replicate_consistency.py)
  compares the sensitive/insensitive verdict between ordinals per key and
  flags any disagreement explicitly rather than averaging it away.

Both are driven by the supervisor below rather than run by hand.

## Batch 3 — L5 noise sweep, ground-truth-scored, kirk1 (built; blocked on a port conflict)

**Update, 2026-09-18:** the CLI is built, tested, and works against real data:
[cmd/tools/lidar-ground-truth-eval](../../cmd/tools/lidar-ground-truth-eval/main.go)
takes `-reference-run-id`/`-candidate-run-id` (each with independent
`-reference-db`/`-candidate-db`, since a candidate produced by a throwaway
isolated server never shares a database with the real one) and calls
`adapters.EvaluateGroundTruth` directly, printing the `GroundTruthScore` as
JSON. Unit tests plus a manual run against real `sensor_data.db` data both
pass. Two corrections to what this section said before actually checking:

- The richest labelled reference run (`60a4774c-db3e-4008-9b7e-d1059ec27319`,
  64 labelled tracks of 141) is **not** kirk0 — `lidar_run_records` says its
  source is `kirk1.pcapng`. That file no longer exists under the live
  server's local pcap dir; it's only on `/Volumes/lidar/lidar/kirk1.pcapng`
  (4.9 GB — replays for this batch must use a short `-duration-seconds`
  window, not the full file, exactly like Batch 1 does for the corpus PCAPs).
- Generating new candidate runs to score requires a live server (nothing
  offline writes to the `AnalysisRunStore`/`lidar_run_tracks` schema
  `GroundTruthEvaluator` reads — `replayeval`/`lidar-state-estimation-baseline`
  write a completely different "immutable observations" schema). So this
  batch needs its own throwaway, isolated server instance — never the
  operator's live one.

**Confirmed blocker (not a config mistake — verified by hand):**
`internal/lidar/server/server.go`'s `Start()` tries to bind the live UDP
listener before it ever starts the monitor HTTP API, and returns immediately
if that bind fails — so a failed bind doesn't just disable live ingest, it
prevents the whole monitor server (and therefore PCAP replay control) from
starting at all. The PCAP-replay BPF filter uses that same configured port
with no per-request override
(`internal/lidar/server/datasource_handlers.go`: `ws.udpPort`), so the
isolated instance can't just pick a free port instead — it has to match
whatever port `kirk1.pcapng`'s packets were captured on (2369), which the
operator's live dev server already holds. Stopping that live server to free
the port is out of scope (never do this unattended); the real fix is a small
future change to auto-detect the replay port per capture the way
`settling-eval` already does (`network.DetectUDPPort`), decoupling it from
the live-listen bind port.
[data/experiments/try/l5-gt-sweep/run_l5_gt_sweep.py](../../data/experiments/try/l5-gt-sweep/run_l5_gt_sweep.py)
checks this before doing anything else and exits with a distinct "blocked"
code and a written reason rather than guessing or faking a result. Retry
once that's no longer true. Single-site only (kirk1) even once unblocked —
no labelled data exists elsewhere — so this will confirm or refute the
preliminary pass's `measurement_noise` finding but won't generalise across
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

Deliberately narrower than
[multi-key-interaction-grid.md](../../data/experiments/try/multi-key-interaction-grid.md)'s
original 3-level (low/default/high) design:
[plan_interaction_levels.py](l3-settling-sweep/plan_interaction_levels.py)
takes up to the top 3 sensitive keys and just 2 levels each (default, worst
flagged value), so N sensitive keys cost 2^N joint runs instead of 3^N. That
answers the actual question this experiment asks — do the single-key worst
cases compound, cancel, or plateau when combined — at a fraction of the
cost; a finer 3-level grid is the natural follow-up only if this one finds a
real interaction. With 2 sensitive keys from Batch 1
(`neighbour_confirmation_count`, `noise_relative`), this is a 4-combo grid
across all 24 sites via
[run_interaction_grid.py](l3-settling-sweep/run_interaction_grid.py), which
varies multiple L3 keys per config and records combos in its own
`interaction-results.csv` (a different row shape than the per-key
`results.csv`, so they're kept separate rather than overloading one schema).
