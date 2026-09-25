// Package perframeeval scores tracker output against independently reviewed,
// held-out annotation episodes, per frame, and compares two arms on the same
// episodes. It is the acceptance harness for gap-analysis row M5: the metric
// code lives in l8analytics; this package decides what the reference is,
// which episodes may be scored, and which estimates are being judged, and it
// refuses every comparison those decisions do not support.
//
// The composition, in order:
//
//  1. Reference: an annotation pack and its review sidecar, through a frozen
//     object-disjoint split manifest, under a recorded policy
//     (annotation.BuildReference). Each episode becomes reference series keyed
//     by the pack's sample timestamps; objects the episode does not score are
//     ignore points in it.
//  2. Hypotheses: one versioned estimate set (lidar_track_estimates by source,
//     estimator, observation model, parameter hash and stage, grouped by the
//     reproducible creation_sequence), or one analysis run's per-frame track
//     positions. Final estimates by default; anything else only as a declared
//     baseline, and the output says so.
//  3. Alignment: each hypothesis point is moved onto the nearest reference
//     frame within a tolerance, because two runs of one capture agree on frame
//     times only to a millisecond or so, and the matcher keys frames exactly.
//  4. Scoring: CLEAR MOT with MOT16 fragmentation, HOTA and the identity
//     family, per episode and pooled.
//  5. Comparison: two arms, paired per episode, with deltas; refused when the
//     reference, policy, gate, tolerance or episode set differ.
//
// It depends on the annotation package, which reaches into L9 for export, so
// it sits beside the layers rather than inside L8.
package perframeeval
