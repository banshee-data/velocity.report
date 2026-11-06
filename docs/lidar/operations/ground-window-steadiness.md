# Ground-screen steadiness over the scoring window

- **Status:** Engineering prerequisite implemented; ground0002 screen remains open
- **Related:** [ground0002](../../../data/experiments/ground/ground0002.md),
  [live surface-ground lifecycle](ground-surface-live-cloud-2026-11.md),
  [Phase 0/1 baseline](state-estimation-phase01-corpus-baseline.md)

ground0002 needs heading jitter, speed jitter and course alignment in its baseline summaries.
The live tracker metrics accumulated lifetime observations of surviving tracks. Copying those
totals at the end would include warm-up and lose tracks that ended during the scored interval.

## Population and measures

The baseline now accumulates these observations directly while its scoring window is open.
A track must already be confirmed at update entry; the update that first confirms it is not
included. Tentative observations and warm-up are excluded. Contributions survive track expiry
and pruning. Starting a new baseline window clears only window counters; tracker reset also
clears them. Per-track lifetime metrics and the existing live aggregate retain their meaning.

| Measure                                                | Definition                                                                                                         |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| `heading_jitter_deg`                                   | RMS of the existing wrapped raw-to-smoothed OBB heading innovations on eligible accepted updates, in degrees       |
| `speed_jitter_mps`                                     | RMS of the existing scalar Kalman-speed changes between accepted observations, in m/s                              |
| `course_alignment_p50_deg`, `course_alignment_p90_deg` | Existing histogram percentiles of the folded box-axis/course difference, 0–90 degrees, at speeds of at least 2 m/s |

Each measure carries its eligible sample count. A locked heading may add no heading-innovation
sample. Course is sampled after the existing heading guards, as before. Coasting adds no
observed steadiness sample; a returning speed comparison can span missed frames, matching the
existing diagnostic. Zero eligible samples yields null values, never an apparent zero-error
measurement. These are consistency diagnostics, not independently referenced position, yaw or
speed accuracy. Lower jitter alone cannot qualify a ground clip or a smoother.

## Replay contract

`tracking_baseline.json` schema 3 adds a `steadiness` object with its own explicit population:
`confirmed_at_update_entry_scoring_window_including_terminated_tracks`. Existing residual and
association populations remain unchanged. Replay results and the corpus runner's per-case
summary carry the same object, including the summary saved before a later case can fail.
Historical schema-2 evidence has no such measurement and must not be read as zero jitter.

Floating values use the baseline's existing six-decimal publication precision. Canonicalisation
copies the summary and pointed values; it cannot mutate tracker state. The existing byte-for-byte
repeat check now covers the new counters and values too. Sampling uses constant memory and
changes no filter estimate, heading decision, association or tuning fingerprint.

## Validation

Tests compare scoring-window counters and RMS values with actual accepted-update increments,
check that opening the window leaves estimates unchanged, and retain observations after
coasting, expiry and pruning. Separate cases cover closed windows, tentative observations,
resets, empty populations, pooled percentiles, detached snapshots, JSON nulls, source immutability
and canonical precision. The corpus summary test verifies the values survive publication.

A real kirk0 PCAP integration test processed warm-up, scored a ten-second window and replayed
it. The schema-3 baseline reproduced byte-for-byte, contained observed speed samples and agreed
with the replay result. This is an execution contract check using the existing test's short
warm-up configuration, not a scientific ground0002 arm or a physical acceptance result.

## Cross-site execution remains blocked here

The current cloud environment's enforced network policy does not allow `us.aws.cdn.hf.co`.
A HEAD request for the pinned Hugging Face Marina capture still failed with proxy 403 on
2026-11-06 UTC. Its manifest size is 2,899,225,140 bytes; Columbus is 4,903,944,004 bytes, with
about 2 GiB free on this host at preflight. No graded-site capture or reserved second segment
was read, and no substitute corpus was scored.

Run the predeclared five arms on the first-segment corpus where the captures and working space
are available, then score kirk0's reviewed tuning and physical references and the cost rule.
The new eligibility manifest and use ledger must authorise development reads before replay;
do not relabel protected or TBD data to make a run proceed. Second-segment qualification,
independent reference review and Pi timing remain separate requirements. No ground default
changes or pass of ground0002 is claimed by this implementation.
