# Return fates and border traits: reuse the frame tap, keep separate labels

- **Status:** OQ-12 answered; l3bg0028 remains proposed
- **Local baseline:** velocity.report `7ec1a599fddb488c274db1839315bd8d53a29642`
- **PCL Git SHA:** `3346843ea6b97697a1577293aa9cdf236b5c7ffb`, 1.15.1
- **Related:** [review](pcl-algorithm-review.md), [handover](pcl-astera-handover.md), [l3bg0028](../../../data/experiments/l3bg/l3bg0028.md), [lower-body report](vehicle-lower-body-by-range-2026-10.md)

All thirteen review questions and the final branch pass are complete. Source findings and
validation below describe this report's recorded checkpoint; subsequent full-suite validation
and remaining implementation prerequisites are recorded in the
[current handover](pcl-astera-handover.md#resume-point). Closing a review question does not
complete or authorise its proposed campaign.

## Decision

Reuse `replayeval.Config.FrameTap` and `FrameTapFrame` for offline frame export and diagnostic
comparison. The existing `pcl-frame-export` already uses that path. Do not build another
publication tap, and do not rename the lower-body reader's `occluded` and `dropout` counts into
PCL shadow and veil labels. They answer different questions over different populations.

The tap is an observer at publication. On the normal path, L4 and the tracker have already run.
Withholding veil candidates from that frame's L4 input therefore needs an earlier replay-only
stage. Coasting decisions need evidence at the track lifecycle decision, before expiry. Those
integration points remain work for l3bg0028; mutating the tap's borrowed data is not an adapter.

## Claim-level evidence

Local line ranges below refer to the baseline above. They are implementation evidence, not new
replay measurements. PCL links refer to the verified Git SHA.

| ID     | Claim tested                                                                    | Verdict and evidence                                                                                                                                                                                                                                                                                                                                                      | Consequence                                                                                                                                                                            |
| ------ | ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C12-01 | The existing tap supplies reusable frame data                                   | Confirmed. [frame_tap.go](../../../internal/lidar/replayeval/frame_tap.go), lines 14–38 and 67–88, exposes returns, published and L3 masks, confirmed tracks, optional surface, background parameters, azimuth bins, and the fixed height floor. [replayeval.go](../../../internal/lidar/replayeval/replayeval.go), lines 1048–1064, installs the adapter and L3 observer | Reuse the read-only callback; copy data retained after it returns. Surface availability depends on an enabled fitter                                                                   |
| C12-02 | This tap is sufficient to implement same-frame withholding and coasting changes | Corrected. [tracking_pipeline.go](../../../internal/lidar/pipeline/tracking_pipeline.go), lines 856–1057 and 1180–1194, transforms/clusters points and updates tracks before publication. [frame_tap.go](../../../internal/lidar/replayeval/frame_tap.go), lines 91–103, observes normal and masked-empty publication but not `AdaptEmptyFrame`                           | Use earlier replay stages for interventions. Audit missing/early-return frames separately; do not promise every decoded frame or pre-expiry hypotheses from this observer              |
| C12-03 | A PCL export needs another frame acquisition path                               | Rejected. [pcl-frame-export/main.go](../../../cmd/tools/pcl-frame-export/main.go), lines 65–80, exports the first complete scored frame through this tap                                                                                                                                                                                                                  | Extend the export contract, not acquisition. The OQ-02 calibrated geometry and source-membership prerequisites still apply                                                             |
| C12-04 | Lower-body fates cover the coasting population                                  | Rejected. [lower_body.go](../../../cmd/tools/lidar-box-eval/lower_body.go), lines 86–117 and 157–202, requires no misses, speed at least 2 m/s, box length at least 3 m, a surface, and beams between that surface and the latest lowest return                                                                                                                           | Coasting tracks, slow objects, pedestrians, and full-height extent ends need their own denominators. Do not generalise the historical percentages                                      |
| C12-05 | `occluded` means the same thing as a PCL shadow border                          | Rejected. [lower_body.go](../../../cmd/tools/lidar-box-eval/lower_body.go), lines 203–230, first separates foreground/clipped returns, then compares only background horizontal range to the track footprint plus a settled-window tolerance                                                                                                                              | A nearer foreground vehicle can obscure a track while receiving the fate `foreground`. A PCL shadow trait belongs to image geometry, without a track footprint or L3 mask              |
| C12-06 | Dropout and guard-held counts supply complete visibility and provenance         | Rejected. [lower_body.go](../../../cmd/tools/lidar-box-eval/lower_body.go), lines 123–151 and 194–209, skips entirely unseen rings, selects one nearest return per slot, and counts `!L3Mask && Mask` as guard-held. [tracking_pipeline.go](../../../internal/lidar/pipeline/tracking_pipeline.go), lines 1184–1192, can also add edge-follow returns                     | Preserve absent-ring/coverage state and source ordinals. The mask difference alone does not split pre-L4 held returns from post-cluster edge-follow; l3bg0027 owns that proposed split |
| C12-07 | Border classes are a reduction of the six fate classes                          | Rejected. PCL [classifyBorders](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/src/range_image_border_extractor.cpp#L365-L458) computes surface structure, scores, neighbour updates, and shadow associations, then sets directional obstacle/shadow bits and veil bits between paired indices                           | Keep separate data columns and preserve all bits. An unset trait is not automatically a reviewed interior, physical endpoint, or known free ray                                        |
| C12-08 | Missing returns can safely be encoded as maximum range                          | Rejected. PCL [changeScoreAccordingToShadowBorderValue](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/impl/range_image_border_extractor.hpp#L180-L223) has a distinct maximum-range-neighbour branch; subsequent search uses opposing scores within the configured border radius                   | Preserve unknown, missing, and explicit maximum-range semantics through the adapter. Absence alone supplies no free-space evidence                                                     |
| C12-09 | A veil flag proves a mixed return that should be removed                        | Rejected as physical truth. [classifyBorders](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/src/range_image_border_extractor.cpp#L408-L458) marks intervening image indices; it does not inspect a sensor waveform or a reviewed body label                                                                             | Withholding remains an experiment on veil candidates, assessed against retained reviewed returns and endpoint truth                                                                    |
| C12-10 | Existing tests establish border or coasting correctness                         | Rejected. [lower_body_test.go](../../../cmd/tools/lidar-box-eval/lower_body_test.go), lines 71–171, checks the six fates, eligibility, footprint geometry, and L3/published distinction. [replay_frame_tap_pcap_test.go](../../../internal/lidar/replayeval/replay_frame_tap_pcap_test.go), lines 16–64, compares baseline bytes and mask/surface availability            | Retain these contracts. They do not test calibrated border labels, unseen-ring coverage, coasting decisions, or a zero-allocation border implementation                                |

## Counterexamples

These are analytical consequences of the code, not newly executed PCL fixtures.

1. **Moving foreground occluder:** put a foreground vehicle in front of the predicted body. The
   lower-body switch calls its return `foreground` (or `clipped`), whereas a geometric visibility
   test must consider whether it blocks the body. Filtering to the `occluded` fate loses it.
2. **Broad nearer wall:** all relevant rays hit a smooth nearer wall. The lower-body test can
   count occluded slots across its interior, even though a border extractor marks discontinuities
   at boundaries. Conversely, a range-image border can exist where no eligible track exists.
3. **Absent ring:** remove every return on one ring. The lower-body reader never marks the ring
   as seen and adds no slots for it. A missing slot on an otherwise seen ring is dropout. These
   are different coverage outcomes, neither of which establishes an empty footprint.
4. **Coasting selection:** change only a track's misses from zero to one. It leaves the lower-body
   population, precisely when the proposed coast arm needs evidence. Post-update publication can
   also be too late to recover a track that has already expired.
5. **Real thin structure between borders:** a genuine narrow return can lie between associated
   obstacle and shadow indices. A veil bit describes that image relationship; removing it can
   discard real reviewed support. The unchanged retention gate must detect that loss.
6. **Identical range, different masks:** two returns in one slot have equal distance but different
   classifications. The nearest-return loop replaces only on strictly smaller distance, so its
   chosen fate can depend on input order. Reuse does not confer the spec's proposed order
   independence; a deterministic membership/tie rule and tests remain necessary.

## Implementation boundary for l3bg0028

Keep three records distinct: lower-body fates relative to an eligible observed track; PCL
directional traits on an explicitly constructed range image; and visibility evidence relative to
a predicted body. Compare them by frame, source ordinal, and track identity where applicable,
without treating one as reference truth for another.

The offline observer may share frame acquisition with the exporter and lower-body diagnostic.
It must record which publication paths it saw, the scored/complete state, source membership,
mask provenance, and coverage. The existing reader infers ring elevation from returns present in
that frame; a border/visibility adapter needs calibrated ray information for missing rings too.
The optional surface and scalar height floor do not constitute exact per-return clipping evidence
for every replay mode. A retained pointer to pipeline state is not a frozen snapshot.

For the veil treatment, compute the candidate labels before selecting L4 input, preserving the
unmodified baseline data for comparison. For the coast treatment, evaluate the predicted body
before the relevant continuation/expiry decision, including frames without detections and
foreground occluders. Record ray coverage, expected detection, and pose uncertainty before using
a return beyond the body as negative evidence. Missing support remains unknown. Wire both as
declared replay experiments; do not move a production default or feed decisions back through the
publication observer.

Before replay, the implementation owner must demonstrate synthetic foreground and background
occluders, smooth occluder interiors, missing slots and rings, explicit maximum-range samples,
genuine thin returns, duplicate-slot ties, coasting/expiry boundaries, and early-return frames.
Test the calibrated adapter's membership and all directional traits separately from reviewed
endpoint labels. Preserve l3bg0028's existing numerical gates and measure the proposed allocation
and frame-time bounds; neither is established here. The OQ-11 development/cohort boundary applies.

## Validation and limits

The existing lower-body and footprint unit tests passed for this checkpoint. Source ranges were
checked against the frozen local baseline and PCL Git SHA. No new border implementation, sensor
adapter, replay campaign, PCL fixture, physical measurement, or Pi timing was performed. The
existing tap's PCAP baseline-equivalence test was inspected, not rerun. Fresh documentation lint,
links, experiment validation, and the no-results guard cover the contract edits; at OQ-12, the latest full-suite run
was OQ-08 and it was not repeated for that documentation-only checkpoint. The later final
branch pass reran the full suite.

The historical l3bg0024 report and its denominators remain intact. l3bg0027 still owns the held
versus edge-follow split and L4 experiment. OQ-12 closes the reuse/semantics question only. OQ-13 subsequently [answered the merged-detection partition question](pcl-merged-detection-partition-disposition.md).
