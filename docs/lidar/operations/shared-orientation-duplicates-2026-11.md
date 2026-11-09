# Shared orientation duplicates: the mechanism, the fix and the heading calibration

<!-- ignore-style-length -->

The November benchmark expected `solid_body_shared_orientation` (#985) to be a shadow change. It
raised duplicates at both haight-divisadero segments (pairs 4 to 5 and 11 to 14; pair-frames 52 to
71 and 311 to 539), and pooled published widths over 2.2 m rose from 24 to 32 %. Its kirk0 heading
NIS had a median of 0.098, against 0.455 for a calibrated filter. This report reads the mechanism
from the code path and from the benchmark's own rows, fixes it, and reads the calibration against
smaller axis scales. It scores no identity, so it has no predeclared spec.

- **Status:** Mechanism found and fixed (default-off options only); calibration read, no key changed
- **Layers:** L5 (the box model, the shadow solid body), L3 (the track guard), read by `lidar-box-eval`
- **Related:** [state plan §9.2.4](../../plans/lidar-state-estimation-plan.md#924-extents-in-the-state-and-censored-extent-evidence-delivered-default-off),
  [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md),
  the benchmark l5id0019 and its report (`tracking-geometry-fixes-benchmark-2026-11.md`, on the
  open PR #994, not yet on main)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`, for every arm. The arms differ only in replay experiments, which are not tuning
  keys

## Answer

**Alone, the shared orientation is a shadow. The duplicates come from its combination with
`box_extent_solid_body`, which the benchmark arm stacked.** The box then draws the solid body's
extent. It refuses an extent measured along an axis more than 10° from its own heading. On a shared
frame the body's axis _is_ the box's heading, so that test could never refuse. On kirk0 the
refusals fell from 3,459 to 567. The extra extents changed the published box's length, width and
centre, never its heading. L3's track guard holds the returns inside a confirmed track's published
box as foreground, so a changed box changes later clusters and then the tracks. The duplicates rose
only after the tracks diverged.

**The fix keeps the axis test.** It now reads the axis the body would hold without the shared
orientation. On kirk0 the combination then refuses 3,660 frames (box extent alone: 3,459). Its
duplicate pair-frames are box extent's 445, not 527. Its tracks match box extent's for 679 of 770
frames, not 533. The earlier behaviour was deliberate in #985, which reported the fall to 567, so it
is kept under its own replay name, `box_extent_shared_axis`. That arm reproduces the benchmark's
`shared-orientation` arm on every frame.

**The axis scale is not the calibration's main cause.** The predicted `P_ψψ` is a median 41 % of
the innovation variance. Read against an `axis_sigma_scale` of 0.5 on the same innovations, the
median NIS is still 0.20, while the mean is already 1.52. No scale brings both near χ²₁.

## What reaches the published box

The three candidates the brief named were read in turn.

1. **The split `Observe`.** `advanceBoxHeading` (step 3c) and `finish` (step 3d) are `Observe`'s
   code in two halves on the same `BoxInput`. The heading half reads the tracked state, the merge
   flag and the frame's clusters. Step 3b settles all three, and the shadow solid body writes none
   of them. The heading half does not read the body.
2. **Coast first.** On a coast the box coasts before the solid body. `place` with no points returns
   before it reads `in.Body`, so the order is invisible to the box.
3. **The body's extent through `box_extent_solid_body`.** `boxBodyExtent` passed the body's
   `orientation.PsiRad` as the axis for `bodyExtents`' agreement test. On a shared frame that is
   `b.psi`, the box's own heading, so `foldHalf(e.psi − b.AxisRad)` is zero and the test passes. The
   frame then takes the body's extent whenever the extent has settled. The extent itself is also
   measured along a different axis.

So only the third reaches the box, and only beside `box_extent_solid_body`. A new synthetic test,
`TestSharedOrientationAloneLeavesThePublishedBox`, pins the first two. On the through-truck pass,
every frame's published box and tracked state are bit-identical with and without the option.

## The benchmark's rows

The benchmark's `lidar-box-eval` rows (build `fe6549b3`) hold every published box on every scored
frame. Track IDs are random per run, so rows were paired within a frame by tracked speed and point
count. A frame's tracks count as the same when both arms hold the same set of these. Until the
first frame where they differ, the arms hold one set of tracks drawn two ways.
`face-state` against `a0` is identical on all four segments: the face state alone is a shadow, as
its arm said.

`box-extent` against `shared-orientation`. "Moved" is the share of paired rows on which the field
differs, read only before the first divergence. Pair-frames are `lidar-box-eval`'s duplicate test
redone from the rows: two boxes with a longer side of 3 m or more, sharing over half the smaller
one's area. Redone, the test reproduces each summary's total exactly.

| Case                 | Same tracks for | Heading moved | Length moved | Width moved | Centre moved | Pair-frames before | Pair-frames after |
| -------------------- | --------------- | ------------- | ------------ | ----------- | ------------ | ------------------ | ----------------- |
| haight-divisadero s1 | 221 of 1,200    | 0 %           | 27.5 %       | 7.3 %       | 28.5 %       | 52 → 52            | 1 → 19            |
| haight-divisadero s2 | 433 of 1,200    | 0 %           | 24.9 %       | 6.9 %       | 26.9 %       | 0 → 0              | 245 → 539         |
| ashbury-downey s1    | 473 of 930      | 0 %           | 25.7 %       | 14.1 %      | 32.7 %       | 5 → 5              | 81 → 79           |
| ashbury-downey s2    | 134 of 1,096    | 0 %           | 13.6 %       | 17.1 %      | 30.7 %       | 0 → 0              | 0 → 1             |
| kirk0                | 533 of 770      | 0 %           | 18.2 %       | 17.4 %      | 25.1 %       | 325 → 323          | 120 → 204         |

The heading never moves, which rules out the split and the coast order on real captures. While the
tracks are the same, a fifth to a third of the boxes change, and the duplicate count does not.
Every added pair-frame comes after the tracks diverge. The duplicates are the track guard's
feedback on a changed box, not the box test reading wider boxes. Widths moved most at
haight-divisadero s2: before divergence its length change reached +3.75 m at the p90, its centre
shift 1.86 m at the p90, and vehicle-wide rows over 2.2 m went from 2 to 27.

## The fix, on kirk0

`solidBodyTrack.own` carries the orientation the body would hold without the option. It is
`solidBodyOrientation` with the previous frame's own orientation as its prior, as `orientation` is
when the option is off. On a shared frame, `boxBodyExtent` hands the agreement test that axis.
`BoxModelConfig.BodyExtentSharedAxis` (replay `box_extent_shared_axis`) restores the shared axis,
and needs `box_extent_solid_body` and `solid_body_shared_orientation`.

The `TestSolidBodySharedOrientationOnKirk0` counts, at `a034ef11`, over associated frames:

| Arm                                        | Took the extent | None to read | Refused on axis | Not settled |
| ------------------------------------------ | --------------- | ------------ | --------------- | ----------- |
| `box_extent_solid_body` alone              | 990             | 1,180        | 3,459           | 1,185       |
| with the shared orientation (own axis)     | 1,000           | 978          | 3,660           | 1,176       |
| with `box_extent_shared_axis` (as on main) | 2,588           | 988          | 567             | 2,689       |

The third row is main's figures exactly. `lidar-box-eval` on kirk0 at `a034ef11`, flags as
l5id0019's box step. Every arm carries `solid_body` and `solid_body_face_state`:

| Arm                               | Tracks as box extent's | Duplicate pairs | Pair-frames | Moving vehicle rows | Width median | Rows over 2.2 m |
| --------------------------------- | ---------------------- | --------------- | ----------- | ------------------- | ------------ | --------------- |
| face state                        |                        | 10              | 452         | 4,635               | 1.91 m       | 36.2 %          |
| face state + shared orientation   |                        | 10              | 452         | 4,635               | 1.91 m       | 36.2 %          |
| box extent                        | 770 of 770             | 9               | 445         | 4,635               | 2.02 m       | 44.8 %          |
| box extent + shared (own axis)    | 679 of 770             | 9               | 445         | 4,601               | 1.97 m       | 39.0 %          |
| box extent + shared + shared axis | 533 of 770             | 9               | 527         | 4,598               | 2.04 m       | 43.6 %          |

The shared orientation alone matches the face state on all 770 frames, boxes included. The
box-extent and shared-axis arms match the benchmark's on every frame. With the own axis, the box
still differs from box extent's on 7 to 13 % of rows before the tracks diverge, never in heading.
That residue is the extent itself, now measured along the box's heading. It is
`box_extent_solid_body`'s contract to publish the body, so it is reported, not gated. On the
synthetic through-truck pass the counts are 56 refusals and 28 taken, for box extent alone and for
the own axis, against 0 and 48 for the shared axis.

## Calibration

Each accepted axis innovation now carries `r²`, the predicted `P_ψψ` and the fit's unscaled
variance. `SolidBodyMeasurement.HeadingNISAtScale(k)` reads `r² / (P + k²·σ_fit²)`. That is the same
innovations against another scale, without changing the tuning key or the filter. kirk0, 3,933
accepted innovations (209, 5.0 %, gated at 1.5):

| `axis_sigma_scale` read | Mean NIS | Median | p90   | Over the 3σ gate |
| ----------------------- | -------- | ------ | ----- | ---------------- |
| 1.5 (the filter's)      | 0.725    | 0.098  | 2.268 | 0 % (by design)  |
| 1.0                     | 1.061    | 0.144  | 3.320 | 2.5 %            |
| 0.75                    | 1.282    | 0.173  | 3.976 | 3.7 %            |
| 0.5                     | 1.524    | 0.202  | 4.717 | 4.8 %            |
| χ²₁                     | 1        | 0.455  | 2.71  | 0.27 %           |

The predicted `P_ψψ` is a median 0.410 of `S` at 1.5 (p10 0.338, p90 0.559). With the observation
variance taken to zero the median could rise only to about 0.24. The mean passes one between 1.5
and 1.0, while the median stays at a third to a half of χ²₁'s. The distribution is a mixture: most
innovations are smaller than even the prediction allows, and a tail is wider than any scale admits.
The prediction's own variance (the yaw-acceleration noise and the seed) and the tail's frames need
reading before a scale is chosen.

## Limitations

- **The haight-divisadero reproduction is owed.** Its segments are not on this host and were not
  downloaded; the shared disk was held for other placements. The mechanism is shown on the
  benchmark's rows for all four segments, but the fix was run on kirk0 only. That it removes the
  haight duplicates is inferred.
- **kirk0's duplicates barely moved under the old combination** (pairs 9 to 9, pair-frames 445 to
  527), so kirk0 shows that the fix restores box extent's figures, not a large duplicate cost.
- **The calibration is a reading, not a re-run.** A filter at a smaller scale would carry another
  `P` and other innovations. Its NIS needs a parameter set with the key changed, and a spec if it
  is scored.
- **Not read:** identity (HOTA, IDF1), the solid body's widths against geom0015, and whether the
  own axis is the right test beyond restoring box extent's admission. Truck 1's 6.20 m solid-body
  width under the shared orientation is the benchmark's tracking fix 3, not this one.

## Recommendations

1. Reread haight-divisadero s1 and s2 for three arms: `box-extent`, the fixed combination and
   `box_extent_shared_axis`. Expected effect (inferred): duplicate pairs back to box extent's 4
   and 7. Falsified if the fixed combination's pairs stay at 5 and 14.
2. Before choosing an `axis_sigma_scale`, read the prediction's share of `S` and the tail's frames:
   speed, aspect, relabels and restarts. Then run any scale change from a parameter set.
3. Keep `box_extent_shared_axis` out of combined arms until fix 3 bounds width spans on an
   unconverged axis.

## Provenance

- Code: branch `claude/lucid-bardeen-ux375z-shared-orientation-dups`, commit `a034ef11` (on main
  `704b8e46`); `lidar-box-eval` built with the Makefile's LDFLAGS (`0.5.1-pre52`, git sha
  `a034ef11d04b`) and `-tags=pcap`.
- Capture: kirk0, `20251205T1417_80858-75c_kirkham-9th`, sha256 `2864ebde38e7…`.
- Box step: `-warmup 6 -duration 0 -require-settled=false -variants none -rows`. The disk filled
  during the first pass, so the box-extent arms were rerun without `-evidence-db`. With and
  without the database the rows are identical, as checked on `box-extent` and the own-axis arm.
- Benchmark rows: l5id0019's `lidar-box-eval` rows, build `fe6549b3`, kirk0 and the first two
  120 s segments of ashbury-downey and haight-divisadero. They are held locally, not in Git.
- Tests: `TestSolidBodySharedOrientationOnKirk0` (pcap) and the new
  `solid_body_shared_axis_test.go` in `internal/lidar/l5tracks`.
