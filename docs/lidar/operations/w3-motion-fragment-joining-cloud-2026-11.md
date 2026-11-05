# W3 motion-supported fragment joining

- **Status:** Default-off implementation and cloud screen; W3 qualification remains open
- **Spec:** [l4clus0008](../../../data/experiments/l4clus/l4clus0008.md)
- **Layers:** L4 Perception, pipeline with prior L5 snapshots

## Question and implementation

Can a recent, well-constrained moving body's predicted footprint support joining
current fragments without enlarging DBSCAN's neighbourhood or recovering noise?
This follows the failed minimum-density and fixed-radius alternatives.

The optional `l4.dbscan_xy_v1.fragment_join` block is absent from defaults.
Startup and offline replay pass the same settings to the callback. Before current
association, a confirmed rigid vehicle with at least eight observations and
observed support is projected to one common capture time. The configured bounds
are 0.2 s age, 2 m/s minimum speed and 0.75 m maximum position standard deviation.
The position covariance uses the full CV projection, the tracker's current process-noise law (including the coupled option), and its largest eigenvalue;
weak prior directions are not replaced by a fixed isotropic certainty.

L4 considers only accepted components and the complete post-cap member stream.
Each member must be inside one predicted footprint, with 0.15 m margin. Any
intersection with a second eligible body makes ownership ambiguous. Components
within a 2 m gap may join under that one body. Existing shape filters still apply;
a rejected group restores all its original components, including their IDs.
Noise and already-rejected components are not recovered. The same partition
feeds detections, immutable observation membership and association. Source
ordinals and processed members survive; evidence samples remain capped separately.

`fragment_join` in the corpus summary counts clustered frames, frames with joins
and components joined, including warm-up. Counts are deterministic diagnostics,
not physical accuracy. No default, wider input cap, minimum density or foreground
classifier changes are made.

## Validation and evidence

Synthetic tests cover genuine separated fragments, ambiguous neighbouring bodies,
missing membership, excessive gaps, non-finite/duplicate priors and shape-filter
rejection. They verify no input mutation, canonical joined identity, full return
lineage, noise preservation and observation-tap consistency. Prior tests refuse
stale, tentative, coasting, stopped, pedestrian and uncertain tracks.

## Corrected-build kirk0 results

Both arms ran the whole capture with six seconds of warm-up and
`-require-settled=false`, matching the development screen rather than claiming
qualification of an unsettled background. Each scored recording held 771 frames;
the counters saw 773 clustered frames, with 39 component joins in 39 frames for
the candidate. First/repeat baseline bytes and joining counts agreed exactly.
The default-off control also matched the independently built identity screen's
control baseline byte for byte.

| Measure                                          |     a0 |   join |
| ------------------------------------------------ | -----: | -----: |
| Component joins                                  |      0 |     39 |
| Confirmed tracks in observed evidence            |     79 |     76 |
| Brief tracks under 1 s                           |     12 |     11 |
| Lasting tracks, at least 2 s                     |     58 |     58 |
| Near-static lasting tracks                       |      6 |      6 |
| Lasting vehicle-sized tracks                     |     36 |     36 |
| Approximate spurious clusters per observed frame | 0.6947 | 0.6326 |
| Published vehicle tracks, at least five frames   |     44 |     43 |
| Observed vehicle frames                          |  4,523 |  4,515 |
| Duplicate pairs                                  |     10 |     10 |
| Overlapping pair-frames                          |    452 |    431 |

The approximate spurious-cluster measure fell 8.94 %, brief tracks fell by one,
and observed vehicle frames fell 0.18 %. The duplicate pair count did not change.
The two vehicle-track definitions differ: the noise screen requires a lasting
track, while the published-box duplicate screen requires five vehicle-sized
frames and includes predictions. These are proxies, not object truth.
End-to-end counts include subsequent tracking/background feedback; they do not
attribute every changed detection to the 39 immediate component joins.

The W3 rule remains **insufficient evidence**. Reviewed fragmentation at 20–35 m,
retention, cross-object merges, HOTA, IDF1 and false negatives were not scored:
Hugging Face label/point payloads still redirected to a CDN blocked by the running
instance, whose network policy remained revision 1 after the saved update.
No target fragmentation denominator or merge truth was invented from these
unlabelled counts. The corpus, frozen held-out identity/merge population and Pi
cost gates remain open, and the candidate remains off by default.

## Execution and validation

The stamped corpus and duplicate-count tools used
`1acf836e` (the full SHA is in each replay manifest), Go 1.27.1 on Linux amd64.
The primary command used the committed `kirk0-corpus.json` and `kirk0-index.json`,
`-warmup 6 -require-settled=false -sample-points 1024`, one of the tuning files
below, an immutable source manifest and per-case evidence. The existing
`lidar_noise_screen.py` read those databases. A separate
`lidar-box-eval -pcap kirk0.pcapng -warmup 6 -variants none` replay per arm supplied
the duplicate/observed-vehicle counts, including instrumentation-wrapper wiring.
Primary first/repeat runs took 83.8 and 84.6 s; box-count runs 30.6 and 27.7 s.
These shared-host wall times do not qualify runtime cost or Pi throughput.

The full Go suite passed. After the process-noise correction, the pipeline,
replay and server suites passed again, as did the added covariance-bound,
mixed-shape and immutable-config properties. Both new internal implementation files passed the 98 % branch-added coverage gate at 100 % of statements. Go vet/lint, documentation lint,
experiment validation, config-order and raw-output guards passed.

Review replaced the full-membership all-pairs gap scan with a gap-sized spatial
grid and the same inclusive squared-distance check. A fixed-seed 500-case oracle
comparison and cell-boundary cases agree with the original scan. On this cloud
host, a separated 2,048-by-4,096-return benchmark took 1.014 ms with the grid
against 6.759 ms with the all-pairs scan (200 ms benchmark duration). The grid
allocated 389 KB per call against zero for the scan; dense neighbouring cells can
still require many comparisons. This microbenchmark is not a G12 or Pi result.

Baseline tuning SHA-256:
`9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`.
Candidate `config/experiments/motion-fragment-join.json` SHA-256:
`2e604040caebd0a8d41f9a4647f42249a0a60fb7c80698e10444413a69129e3d`.
Corpus tool executable SHA-256:
`329a23b1f51a93b6c2eb2c602bcd04e964211e212055aa2fce455d1e3ce90a94`.
The raw databases were compressed outside Git, with decompressed SHA-256 checked
before removing the uncompressed copies.

## Provenance

Base `1ec1ccea5947ec8f72d155b337ed86d066f342c4`.
The pre-run rule was committed before any candidate replay. Kirk0 source SHA-256
`2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`.
Dataset revision `37ef1befc1876ad860800beea1bbeaa6c9078da9`.
Raw evidence and logs stay outside Git.

The first development launch was stopped before reading candidate results when
review found that the hint covariance projection omitted process noise. The
implementation now reads the actual tracker noise configuration, including
through instrumentation, rejects priors beyond `MaxPredictDt`, and has a
boundary regression that the noise-free calculation would incorrectly admit.
The final screen uses fresh outputs and a single corrected build for both arms;
the stopped launch supplies no acceptance evidence.
