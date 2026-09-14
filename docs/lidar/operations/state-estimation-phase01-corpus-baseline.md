# Phase 0/1 state-estimation corpus baseline

- **Status:** Recorded: deterministic medoid reference established; physical acceptance remains open
- **Scope:** Three-site, 16-capture offline replay corpus
- **Repository revision:** `261deac40dae070ed3d53af9b5f39f31d568059e`
- **Related:** [State estimation](../../plans/lidar-state-estimation-plan.md), [lossless observation persistence batching](../../plans/lidar-lossless-observation-persistence-batching-plan.md), [archive index](../../../tools/s2-archive/site-index.json)

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
43,068 scored frames** for the `medoid_v0` reference measurement. It confirms
that the rebase's frame-boundary and ordering changes did not reintroduce the
previous byte-level instability.

## Measured reference bands

These are tracker-internal reference measurements, not surveyed physical truth.
NIS includes accepted associations only, and the reference arm still uses the
medoid measurement. They are useful because the same evidence now reproduces
them exactly.

| Case                  | ≥5 m/s samples | Lateral RMS | Longitudinal RMS | Mean NIS | NIS exceedance | Association rate |
| --------------------- | -------------: | ----------: | ---------------: | -------: | -------------: | ---------------: |
| Marina, Webster Beach |          5,455 |     0.220 m |          0.521 m |    2.306 |          10.2% |            65.7% |
| Columbus, Broadway    |          9,514 |     0.295 m |          0.576 m |    2.408 |          10.2% |            42.7% |
| Embarcadero, Folsom   |          4,131 |     0.279 m |          0.602 m |    3.233 |          17.1% |            59.9% |

Columbus has the weakest association rate in this band. Embarcadero has the
highest NIS and exceedance rate. Those are review leads, not a diagnosis:
association coverage, visible-surface bias, road grade, and measurement error
are still entangled.

## Artefacts and verification

The completed run is stored outside Git at:

`/Volumes/lidar/lidar/state-estimation-phase01-rebased-20260913`

It contains the three first/repeat baseline pairs, `phase0-summary.json`, and a
full-fidelity SQLite evidence database. The directory is 5.5 GB; the database
is 3.0 GB. Its size is a Phase 1 performance finding, not a reason to remove
points from the record.

| Artefact                               | SHA-256                                                            |
| -------------------------------------- | ------------------------------------------------------------------ |
| `phase0-summary.json`                  | `cfc4003846365e21548f5000166b32bf764137ae8527b6b3fa71fd771ed7d99b` |
| Marina baseline, first and repeat      | `3a0efe8f3b332fd759b2b01a1b26e5028582a6d5a341bc75ccdefa8275c54418` |
| Columbus baseline, first and repeat    | `8dee47b45570468acb1f83e8fde4fce979e19af648f6231ab335e896626010af` |
| Embarcadero baseline, first and repeat | `22cc7b9869ee52cf779e8650823bbbcf1b93c26e5e2a25f652b35cf128cc3b31` |

The run used the committed corpus declaration in
`tools/s2-archive/state-estimation-phase01-corpus.json`, the archive index in
`tools/s2-archive/site-index.json`, the default embedded tuning, a 70-second
warm-up, full capture duration, `require-settled=true`, and
`measurement-mode=medoid_v0`.

## Preservation contract

Git preserves the small, reviewable control record. It must contain this report,
the corpus declaration, archive index revision, summary, baseline JSON hashes,
run configuration, source IDs, and the repository revision. The three small
baseline JSON files may also be checked in beside a future compact manifest.

Git does **not** preserve the multi-gigabyte SQLite database or duplicate PCAPs.
Those are dataset artefacts. Preserve them as follows:

1. Keep the completed external directory immutable. Do not rerun into it or
   replace files in place.
2. Create and retain a source-PCAP manifest on the LiDAR volume before moving
   or uploading evidence. It names every PCAP in corpus order, its path
   relative to the LiDAR root, byte size, SHA-256, corpus case, and source ID.
   Keep its own SHA-256 in the compact result record. For this immutable run,
   place it beneath `/Volumes/lidar/lidar/manifests/`, not inside the completed
   directory. The manifest is the bridge between repository history and dataset
   storage, and the input guard for the future batching oracle.
3. The compact external result manifest also records configuration,
   output-file SHA-256 values, database SHA-256, byte sizes, and the Git
   revision.
4. Store the evidence database as a derived corpus artefact in the Hugging Face
   dataset, not in Git LFS. It belongs with the raw PCAP corpus and must retain
   its original bytes and manifest checksum. A storage URI can be added to the
   compact Git manifest only after that upload is verified.
5. Preserve the first/repeat baseline pair, not merely a statement that it
   matched. A future verifier compares their hashes and may regenerate them
   from the named PCAPs.

This keeps the repository small while making a missing, altered, or mismatched
external artefact visible. A 5 GB database in Git would be neither convenient
nor particularly immortal; it would merely make everybody's clone day worse.

## What this closes, and what it does not

The run closes the three-site deterministic replication sub-gate for the
current medoid reference and supplies a full-fidelity before measurement for
the batching work.

It does not close Phase 0 physical acceptance, G-PER-1, or any corrected
measurement gate. The next evidence work is to review representative
Columbus association failures and Embarcadero high-NIS episodes with frozen
VRLOG scenes, freeze replacement jump cases, batch persistence without data
loss, add the P11 surface/clipping context, and then run the reopened-evidence
and Pi checks required by Phase 1.
