# Phase 0/1 state-estimation corpus baseline

- **Status:** Recorded: deterministic medoid reference established, full speed-banded residual/association tables published, and association-failure causes attributed. Physical acceptance, G-PER-1, and the historical jump-track replacement remain open.
- **Scope:** Three-site, 16-capture offline replay corpus
- **Repository revision:** `0277a0dd47273e0c25a00a64e5f42c2851a0e744`
- **Related:** [State estimation](../../plans/lidar-state-estimation-plan.md), [lossless observation persistence batching](../../plans/lidar-lossless-observation-persistence-batching-plan.md), [deterministic track identity](../../plans/lidar-deterministic-track-identity-plan.md), [archive index](../../../tools/s2-archive/site-index.json)

## Result

The Phase 0/1 replay completed with a byte-identical first and repeat baseline
for every selected site. The first pass wrote immutable observations, linked
online estimates, and residuals. The repeat pass deliberately reopened no
observation database, so it tested the same evidence and pipeline without
changing the write-once source.

| Case                  | Captures | First-run frames | Repeat-run frames | Baseline  | Immutable source ID                                                          |
| --------------------- | -------: | ---------------: | ----------------: | --------- | ---------------------------------------------------------------------------- |
| Marina, Webster Beach |        4 |           11,320 |            11,320 | Identical | `source/v1/f7b8cdfe9c49c5b679476950a6732df7fa62422c38b76bdc7fe146df7dc1fe4c` |
| Columbus, Broadway    |        7 |           20,320 |            20,320 | Identical | `source/v1/23d962b75b38b8ab392a5846acd9469ff5620b81e03c325da488ac82c19f3034` |
| Embarcadero, Folsom   |        5 |           11,428 |            11,428 | Identical | `source/v1/cc52e0eafec652af4b2c2fa8d7a5e3feedc280ad4ed1efc040fc31ef4d7e2493` |

The corpus therefore establishes deterministic replay across **16 captures and
43,068 scored frames** for the `medoid_v0` reference measurement, unchanged
from the original September 13 rebase. It confirms that neither the frame
batching work nor the `creation_sequence` fix (see below) altered detection or
estimation output.

### A false positive found and fixed along the way

An independent replay comparison initially reported 99.2% of frames as
"changed" between this corpus and a later re-run. Investigation traced this
entirely to `track_id`: `l5tracks` assigns it a random UUID deliberately, so
it stays collision-free across tracker resets and restarts, but that means it
differs between any two replays of identical input. The comparison tool was
hashing it as content. Every one of 42,947 compared estimate and residual rows
was in fact byte-identical once `track_id` was excluded and the tracker's
deterministic `creation_sequence` (now persisted alongside each estimate) was
used to key the comparison instead. See
[the deterministic track identity plan](../../plans/lidar-deterministic-track-identity-plan.md)
for the fix and the wider identity-scheme proposal it motivated.

## Measured reference bands

These are tracker-internal reference measurements, not surveyed physical
truth. NIS includes accepted associations only, and the reference arm still
uses the medoid measurement. `speed_floor_mps=0` includes every accepted
association regardless of speed and cannot decompose lateral from
longitudinal error (`decomposed=0`); it is retained here as the aggregate row
against which the higher bands can be compared.

### Marina, Webster Beach

| Speed floor |      n | Lateral RMS | Longitudinal RMS | Lateral bias | Mean NIS | NIS exceedance | Matched | Missed | Association rate |
| ----------: | -----: | ----------: | ---------------: | -----------: | -------: | -------------: | ------: | -----: | ---------------: |
|       0 m/s | 28,683 |         n/a |          0.428 m |          n/a |    0.783 |           3.2% |  28,683 | 15,185 |            65.4% |
|       2 m/s |  4,877 |     0.208 m |          0.462 m |      0.003 m |    1.676 |           7.7% |   4,877 |  3,399 |            58.9% |
|       5 m/s |  5,455 |     0.220 m |          0.521 m |     -0.009 m |    2.306 |          10.2% |   5,455 |  2,843 |            65.7% |
|      10 m/s |  1,629 |     0.221 m |          0.406 m |      0.014 m |    1.940 |           7.2% |   1,629 |  1,149 |            58.6% |
|      15 m/s |     15 |     0.219 m |          0.427 m |     -0.016 m |    2.843 |           6.7% |      15 |     45 |            25.0% |

### Columbus, Broadway

| Speed floor |      n | Lateral RMS | Longitudinal RMS | Lateral bias | Mean NIS | NIS exceedance | Matched | Missed | Association rate |
| ----------: | -----: | ----------: | ---------------: | -----------: | -------: | -------------: | ------: | -----: | ---------------: |
|       0 m/s | 63,170 |         n/a |          0.566 m |          n/a |    0.842 |           3.9% |  63,170 | 50,020 |            55.8% |
|       2 m/s | 10,696 |     0.365 m |          0.681 m |     -0.045 m |    2.861 |          13.2% |  10,696 | 13,711 |            43.8% |
|       5 m/s |  9,514 |     0.295 m |          0.576 m |     -0.010 m |    2.408 |          10.2% |   9,514 | 12,761 |            42.7% |
|      10 m/s |    395 |     0.300 m |          0.515 m |     -0.004 m |    2.510 |          11.9% |     395 |    764 |            34.1% |

Columbus has no `15 m/s` band: nothing in this capture reached that speed.

### Embarcadero, Folsom

| Speed floor |      n | Lateral RMS | Longitudinal RMS | Lateral bias | Mean NIS | NIS exceedance | Matched | Missed | Association rate |
| ----------: | -----: | ----------: | ---------------: | -----------: | -------: | -------------: | ------: | -----: | ---------------: |
|       0 m/s | 32,719 |         n/a |          0.414 m |          n/a |    0.593 |           2.7% |  32,719 | 14,073 |            69.9% |
|       2 m/s |  4,159 |     0.393 m |          0.655 m |      0.090 m |    2.811 |          13.2% |   4,159 |  4,565 |            47.7% |
|       5 m/s |  4,131 |     0.279 m |          0.602 m |      0.035 m |    3.233 |          17.1% |   4,131 |  2,771 |            59.9% |
|      10 m/s |  1,288 |     0.351 m |          0.547 m |     -0.089 m |    3.100 |          15.2% |   1,288 |  2,100 |            38.0% |
|      15 m/s |      2 |     0.211 m |          0.275 m |     -0.164 m |    1.491 |           0.0% |       2 |     30 |             6.3% |

Columbus has the weakest 5 m/s+ association rate; Embarcadero has the highest
NIS and exceedance in that band. Those remain review leads, not a diagnosis:
association coverage, visible-surface bias, road grade, and measurement error
are still entangled.

## Association failures by cause

For each track, a gap between consecutive accepted associations longer than
500 ms (5 sensor periods) was classified by whether any detection existed
anywhere in the scene during the gap, regardless of which track it belonged
to:

- **`other_detections_present`**: the pipeline kept detecting objects in this
  frame span; the miss is specific to this track. Candidate causes are
  occlusion by another object, cluster fragmentation, or gating against a
  biased prediction — the P4 candidates already named in the state-estimation
  plan.
- **`zero_scene_detections`**: nothing was detected anywhere in the scene for
  the whole gap. Candidate causes are a genuinely empty moment on the road, or
  a pipeline-level frame drop independent of any one track.

| Case                  | Tail gaps (>500 ms) | `zero_scene_detections` | `other_detections_present` | Longest gap |
| --------------------- | ------------------: | ----------------------: | -------------------------: | ----------: |
| Marina, Webster Beach |                 628 |                0 (0.0%) |               628 (100.0%) |      2.30 s |
| Columbus, Broadway    |               1,582 |                6 (0.4%) |              1,576 (99.6%) |      4.90 s |
| Embarcadero, Folsom   |                 473 |                4 (0.8%) |                469 (99.2%) |      3.90 s |

The finding is one-sided: essentially every long gap (99.2–100% per site)
occurs while other detections continue elsewhere in the scene. This is
evidence against a whole-frame pipeline dropout as the dominant cause and
evidence for a per-track failure mode — occlusion, fragmentation, or gating —
consistent with defect P4 in the state-estimation plan ("association misses
about 56% of frames on moving tracks... candidates are gating against a
biased prediction, cluster fragmentation, and frame throttling"). It does not
by itself distinguish between those three candidates; that requires
inspecting specific gap episodes against frozen VRLOG scenes, which remains
open (see below).

Almost all tail gaps land in the 500 ms&ndash;1 s bucket (1,102 of 1,582 for
Columbus, 482 of 628 for Marina, 339 of 473 for Embarcadero), with a rapidly
thinning tail out to a handful of multi-second gaps and nothing beyond 5 s in
this corpus.

## Artefacts and verification

The completed run is stored outside Git at:

`/Users/david/code/sensor_data/lidar/vrlog/phase01-threesite-creationseq-retry-20260915`
(recorded VRLOG baselines) and
`/Users/david/code/sensor_data/lidar/evidence/phase01-threesite-creationseq-retry-20260915`
(the immutable observation/estimate/residual evidence database, now on local
disk rather than the external LiDAR volume — see the read/write separation
note below).

It contains the three first/repeat baseline pairs, `phase0-summary.json`, and
a full-fidelity SQLite evidence database covering all three sites
(distinguished internally by `source_id`). The database is 3.57 GB.

| Artefact                               | SHA-256                                                            |
| -------------------------------------- | ------------------------------------------------------------------ |
| `phase0-summary.json`                  | `6748096aef8f31782db984019c28f4db7219adb99ed5d098430eac5a3799e4d8` |
| Marina baseline, first and repeat      | `70eeca5a2b8607b2868d51d8e3001ea24d72ee8869cc5d4c97039cc841fe5d64` |
| Columbus baseline, first and repeat    | `456c5b9136ef5380e4c819e76e06fd0e91efb6cf5fa2ad2cfec910e5769f0387` |
| Embarcadero baseline, first and repeat | `c72c2902f9733cf74b4e19692f903101445a43798a0f07907f9dbd9a914b6af6` |

The run used the committed corpus declaration in
`tools/s2-archive/state-estimation-phase01-corpus.json`, the archive index in
`tools/s2-archive/site-index.json`, the default embedded tuning, a 70-second
warm-up, full capture duration, `require-settled=true`, and
`measurement-mode=medoid_v0`. It verified against the same immutable source
manifest as the original September 13 run
(`/Volumes/lidar/lidar/manifests/state-estimation-phase01-rebased-20260913.source-pcaps.json`,
digest `sha256:83c17249c48238abb748fb467ae6229c8b13ff7e5eea5f0cd57f5641c04252ba`),
so this is the same 16-capture corpus, not a re-derivation.

### Read/write separation

The original run wrote its evidence database to the same external LiDAR
volume it read source PCAPs from. Under concurrent load (a second replay and
an evidence-oracle export running at the same time) this produced severe I/O
contention: a comparable single-site replay took roughly five hours. This run
used `-evidence-dir` to write both the VRLOG and the evidence database to
local disk while still reading PCAPs from the external volume, and completed
all three sites, first and repeat, in about 21 minutes. The external volume
also disconnected once mid-run (a physical cable disconnect, not a drive
fault — confirmed by a clean re-read afterward); the run was restarted rather
than resumed, since the tool has no partial-corpus resume path.

## Preservation contract

Git preserves the small, reviewable control record. It must contain this
report, the corpus declaration, archive index revision, summary, baseline
JSON hashes, run configuration, source IDs, and the repository revision. The
three small baseline JSON files may also be checked in beside a future
compact manifest.

Git does **not** preserve the multi-gigabyte SQLite database or duplicate
PCAPs. Those are dataset artefacts. Preserve them as follows:

1. Keep the completed external directory immutable. Do not rerun into it or
   replace files in place.
2. Create and retain a source-PCAP manifest on the LiDAR volume before moving
   or uploading evidence. It names every PCAP in corpus order, its path
   relative to the LiDAR root, byte size, SHA-256, corpus case, and source ID.
   Keep its own SHA-256 in the compact result record. Place it beneath
   `/Volumes/lidar/lidar/manifests/`, not inside the completed directory. The
   manifest is the bridge between repository history and dataset storage, and
   the input guard for the batching oracle.
3. The compact external result manifest also records configuration,
   output-file SHA-256 values, database SHA-256, byte sizes, and the Git
   revision.
4. Store the evidence database as a derived corpus artefact in the Hugging
   Face dataset, not in Git LFS. It belongs with the raw PCAP corpus and must
   retain its original bytes and manifest checksum. A storage URI can be
   added to the compact Git manifest only after that upload is verified.
5. Preserve the first/repeat baseline pair, not merely a statement that it
   matched. A future verifier compares their hashes and may regenerate them
   from the named PCAPs.

This keeps the repository small while making a missing, altered, or
mismatched external artefact visible.

## What this closes, and what it does not

The run closes the three-site deterministic replication sub-gate for the
current medoid reference, publishes the full speed-banded residual and
association tables (not just the single `≥5 m/s` aggregate previously
recorded), and attributes association-failure gaps to per-track causes rather
than leaving them unexamined. It supplies a full-fidelity before measurement
for the batching work, now cross-validated against a second independent
replay with zero content drift.

It does not close Phase 0 physical acceptance, G-PER-1, or any corrected
measurement gate. Specifically still open:

- **Review representative failure episodes against frozen VRLOG scenes.** The
  cause breakdown above distinguishes "something else was detected" from
  "nothing was detected," but not which of occlusion, fragmentation, or
  gating explains the former — that needs individual episodes inspected
  against recorded scenes, not aggregate counts.
- **Freeze a reviewed replacement for the unavailable 33 historical jump
  tracks.** This needs the production database and `lidar-jump-candidates.py`,
  and is a separate, human-reviewed curation task rather than a replay
  measurement.
- **Pi per-stage timing, memory and throughput** — everything above ran on
  macOS M1, the acceptance platform for this plan's gates. Confirming the
  same result on the deployed Raspberry Pi is a v0.6.x+ hardware-validation
  pass, not a Phase 0 or G-PER-1 blocker.
- **P11 surface/clipping context** for near-edge geometry, and the reopened-
  evidence and remaining Phase 1 checks.
