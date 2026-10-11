# Geometry convergence: predeclared levels (W0)

<!-- ignore-style-length -->

The levels the tracker's heading, box and identity must reach before the geometry convergence
candidates can be read as an improvement, pinned on the tuning references before any candidate
is scored; the measures every arm now reports so the levels can be read; and the values today.
This is the record the physical scorer's held-out acceptance is meant to cite; the scorer does
not cite it yet, and nothing here is a held-out result.

- **Status:** Predeclared, 2026-10-09, on kirk0's tuning references and the 23-site corpus. The three label-free measures are implemented and reported by every solid-body arm; the scorer-side citation and the held-out segment are outstanding
- **Layers:** L5 Tracks (solid body), L8 Analytics, offline replay and per-frame evaluation
- **Related:** [geometry convergence plan §3](../../plans/lidar-tracker-geometry-convergence-plan.md#3-what-acceptable-looks-like), [physical-reference review plan](../../plans/lidar-physical-reference-review-plan.md#scoring-and-inspection), [scoring against physical references](point-annotation-tool.md#scoring-against-physical-references), [alignment report](solid-body-physical-alignment-kirk0-2026-10.md), [W7 spike](end-on-truck-window-spike-2026-10.md), [G-UNC-1 criteria](adaptive-uncertainty-criteria.md) (the format this follows)
- **Code:** [containment and spans on the row](../../../internal/lidar/l5tracks/solid_body_nearedge.go), [summary guards](../../../internal/lidar/replayeval/solid_body_summary.go), [scorecard block](../../../cmd/tools/lidar-track-scorecard/main.go), [migration 000059](../../../internal/db/migrations/000059_lidar_solid_body_containment.up.sql)

## Declaration

These levels were fixed before W1 (`solid_body_rectangle_heading`) or W2
(`solid_body_containment`) existed. A level is amended only in writing, dated, with its reason,
under Amendments, and before the next held-out run; a result read against a level amended after
the run is not a result. The references' own bounds are the resolution of every physical row: a
level tighter than a bound cannot be read, and an error inside the bound is zero.

Two words are used exactly. The **axis** is the body's orientation modulo 90°, which no length
label touches. The **directed heading** is the yaw modulo 360°, which includes which axis is the
length and which end is the front. A right axis with the wrong label is a 90° directed error and
a 0° axis error, and the two are scored separately so that is seen as what it is.

## Arms

| Arm       | Experiments                                                                                                  | Role                                                                         |
| --------- | ------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| Shadow    | `solid_body`, `solid_body_face_hysteresis`, `solid_body_course_faces`, `solid_body_full_members`             | Baseline where identity cannot move; the first arm each candidate is read on |
| A2        | The shadow's set and `near_edge_track`                                                                       | Baseline for the tracked state; identity is its own row                      |
| Candidate | A2 or the shadow with `solid_body_rectangle_heading`, `solid_body_containment`, each alone and both together | What the levels are read against                                             |
| Alignment | A2 or the shadow with the four alignment experiments (open-prior centring)                                   | The comparison the candidate must beat to replace it                         |

Every arm is replayed from one stamped build of `lidar-state-estimation-baseline`; the A2
control's summary must be identical, timing aside, to the previous build's before any candidate
is read.

## Population

- **Tuning, physical.** kirk0, pack `ad8b9438-c1d4-40f0-bd0a-f1852c984569`, frozen split
  `kirk0-ad8b9438-tuning-r1`, episode `following-472-530`: 22 reviewed poses and 11 following gaps
  over three vehicles. Truck 2 is scored separately: its cab never reaches the tracker (W3), so
  no L5 change can place it, and a level over all poses would be decided by it.
- **Tuning, identity.** The same split, episode `whole-pack`.
- **Label-free.** The 23 tuning and screen sites of the D2 first-segment corpus, 200 s scored
  after a 70 s warm-up, as the alignment run's passes ran them; `embarcadero-folsom` stays held
  out of it.
- **Held out.** One segment the corpus did not use, drawn by lot as the W7 spike sets out and
  reviewed with operator-keyframed yaw and extents. No level below is read held out until that
  segment is frozen and the scorer cites this record.

## Evidence sets

| Set                   | Produced by                                                                      | Rows read here                                                                                                     |
| --------------------- | -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `physical.json`       | `lidar-ground-truth-eval perframe -physical-reference` on the frozen split       | Per-pose centre, yaw, length, width, box IoU and their reference bounds; per-gap error and bound                   |
| `identity.json`       | The same, episode `whole-pack`                                                   | HOTA, IDF1, ID switches                                                                                            |
| `phase0-summary.json` | `lidar-state-estimation-baseline` with `solid_body` named                        | The `solid_body` block: `containment`, `extent_shortfall`, `axis_by_age`, `anchor_solid_bodies_body_centre_frames` |
| `scorecard.json`      | `lidar-track-scorecard -observations <evidence.db>`                              | The `solid_body` block, the same guards without the strata, one per corpus site                                    |
| `tracker_timing.json` | The baseline tool with `-campaign-metrics`, balanced order, nothing else running | `Tracker.Update` p50 and p99                                                                                       |

The three guards are read from the persisted solid-body row, which since migration 000059
carries `containment_share` (the share of the frame's cluster points inside the reported box
widened by the 0.15 m face tolerance), `contained_points`, and the cluster's trimmed spans along
and across the reported orientation. A row written before the migration has no share and is left
out, never read as zero. Under `solid_body_full_members` the tracker measures the cluster's
members, so the share is against what the tracker saw, not the persisted sample.

## Criteria

| Id  | Measure                                 | Operational test                                                                                                                                                                                   | Level                                                                  |
| --- | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| G1  | Axis, established rows at 2 m/s or more | `physical.json` yaw error per scored pose, folded to [0°, 90°], less the pose's yaw bound, floored at zero; truck 2 separately                                                                     | Median 0°; p90 ≤ 10°                                                   |
| G2  | Directed heading                        | The same yaw error unfolded, less the bound                                                                                                                                                        | ≤ 10° on at least 90 % of poses                                        |
| G3  | Heading convergence, label-free         | `axis_by_age` rows `2-4s` and `4s+`, `axis_over_30_share`, per corpus site; the median over sites                                                                                                  | ≤ 0.05                                                                 |
| G4  | Heading ambiguity                       | A row whose label is unresolved is marked and reports its observed spans (a W1 field)                                                                                                              | No unresolved row reported as a labelled body                          |
| G5  | Containment                             | `containment.by_reference[body_centre].held_share`: rows holding at least 98 % of their points, kirk0 and the corpus median                                                                        | ≥ 0.95                                                                 |
| G6  | Reported extent                         | `extent_shortfall.length_short_share` and `width_short_share` of the reported box (observed span over the reported dimension by more than 0.25 m)                                                  | 0 for the reported box; the belief's shortfall reported beside it      |
| G7  | Centre                                  | `physical.json` centre error against the pose's centre bound; truck 2 separately                                                                                                                   | ≥ 80 % of scored poses within the bound                                |
| G8  | Length                                  | `physical.json` length against the reference, established poses; cars and trucks separately, truck 2 separately                                                                                    | Cars within 15 %, trucks within 20 %, on ≥ 80 % of poses               |
| G9  | Following gap                           | `physical.json` gap error against the gap's bound, both bodies established                                                                                                                         | Within the bound on every scored gap                                   |
| G10 | Lateral, G-GEO-1                        | Criteria 1 to 4 as the [state plan §9.3](../../plans/lidar-state-estimation-plan.md#93-decision-gate-g-geo-1) writes them; and `anchor_solid_bodies_body_centre_frames.p99_m` at the corpus median | Criterion 3 within 5 %; the candidate's p99 not above the baseline's   |
| G11 | Identity, A2 only                       | `identity.json` ID switches on `whole-pack` against the A2 baseline's; HOTA and IDF1 reported                                                                                                      | Within the larger of 10 % of the baseline and the measured no-op floor |
| G12 | Cost                                    | `tracker_timing.json` p99 against the arm's baseline, median of four balanced runs; the orientation fit's time stated per cluster on the Mac and on a Pi                                           | Within 10 %; the fit measured, not estimated                           |

**The no-op floor of G11.** On the frozen split, A2's identity moves with any change to the
solid body whether or not a scored pose changes. The one arm of the alignment run that changed
no scored pose, `solid_body_vehicle_extent_floor` alone, moved the switches from 100 to 81, so
the measured floor is 19 switches, 19 % of the baseline, from a single no-op. A second arm tried
for the floor, `solid_body_face_plane_spans`, moves scored poses at the current build and does
not qualify. The band is therefore the larger of 10 % and 19 switches until a second no-op is
measured; a candidate inside the band has not changed identity, one outside it has, and either
way HOTA and IDF1 are printed beside the count.

**What a level is not.** G5 and G6 are met by construction once W2 ships, since the constraint
makes the reported box hold its points; they then check that it ran, and the belief's shortfall
beside G6 is what still measures the estimator. G3 under a course-fed heading agrees with the
course by construction; the axis column of `axis_by_age`, which no label touches, is the
convergence row, and the directed column is a labelling check. G1 against kirk0's references
compares a fit with a fit, since the references were fitted to the returns the tracker reads;
W7's operator-keyframed references break that, and until then G1 is necessary, not sufficient.

## Running it

```bash
# One arm on kirk0, with the solid-body summary and an evidence database.
./baseline -corpus cmd/tools/lidar-state-estimation-baseline/testdata/kirk0-corpus.json \
  -index cmd/tools/lidar-state-estimation-baseline/testdata/kirk0-index.json \
  -pcap-root internal/lidar/perf -pcap-subdir pcap -warmup 20 -case kirk0 \
  -experiment solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members,near_edge_track \
  -sample-points 256 -out ./out -evidence-dir ./evidence -evidence-per-case
```

```bash
# The physical and identity scores against the frozen split.
./gt-eval perframe -pack "$PACK" -split-manifest "$SPLITS/kirk0-ad8b9438-tuning-r1.json" -split kirk0-tuning \
  -episodes following-472-530 -allow-tuning-split -physical-reference \
  -a-label a2 -a-db ./evidence/kirk0.db -a-stage online -a-declared-baseline -a-solid-body \
  -b-label candidate -b-db ./candidate/kirk0.db -b-stage online -b-declared-baseline -b-solid-body -json physical.json
```

```bash
# The label-free guards for one corpus site.
./scorecard -observations ./site/evidence/observations.db -scoring-start-seconds 70 -json scorecard.json
```

The `solid_body` block of the scorecard and of `phase0-summary.json` carries `containment`
(rows, `held_share`, `median_share`, by reference), `extent_shortfall` (rows, the length and
width short rows, shares and median excess) and `axis_by_age` (per bucket: rows, the axis
median, p90 and share over 30°, and the directed figures beside them).

## Evidence so far

kirk0 at `be79fbd53` with this build's measures; the corpus figures are the alignment run's
passes, read from the same rows by hand before the measures existed.

| Id  | A2                                                                     | Alignment candidate (A2, four experiments)                                    |
| --- | ---------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| G1  | Directed 7.5° median, 89° p90; axis not yet printed by the scorer      | 12° mean directed                                                             |
| G3  | kirk0 `2-4s` 0.56, `4s+` 0.41; corpus 0.23 to 0.27 (directed, by hand) | kirk0 0.06, 0.20; corpus 0.04 to 0.06, self-measured under the course heading |
| G5  | Body centre 0.18 (928 rows); medoid 0.66                               | 0.29 (940 rows); medoid 0.67                                                  |
| G6  | Length 0.45 (median excess 1.22 m), width 0.20                         | Length 0.24 (0.61 m), width 0.21                                              |
| G7  | 5 of 19 within the bound                                               | 11 of 21                                                                      |
| G8  | Car 3.88 of 4.26 m; truck 1 5.88 of 10.63; truck 2 0.62 of 9.63        | Car unchanged; truck 1 8.62; truck 2 unchanged                                |
| G9  | 3 of 7 gaps within the bound                                           | 3 of 7                                                                        |
| G10 | Corpus lateral p99 0.107 to 0.684 m by site                            | −0.024 m at the corpus median against A2                                      |
| G11 | 100 switches, HOTA 0.310, IDF1 0.285                                   | 89, 0.310, 0.297                                                              |
| G12 | `Tracker.Update` p99 4.07 ms, p50 0.109 ms                             | 3.07 ms (−25 %), 0.130 ms (+19 %)                                             |

None of these is a pass or a fail: they are where the baseline and the candidate the convergence
plan replaces stand against levels written after them, which is the one direction the record
allows.

**W2, `solid_body_containment`**, the first arm scored against the levels as written: kirk0 at
`a91db3862` with the Go measures, the corpus medians from pass 4 of the alignment run's harness,
all in the [containment corpus report](solid-body-containment-corpus-2026-10.md).

| Id  | A2 with containment                                                                                                                                | Alignment candidate with containment                   | Against the level                                                      |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ | ---------------------------------------------------------------------- |
| G1  | Unchanged: yaw is not W2's to move                                                                                                                 | Unchanged                                              | Not read                                                               |
| G3  | kirk0 `2-4s` 0.58, `4s+` 0.45; corpus 0.23, 0.26 (axis, from the rows)                                                                             | kirk0 0.05, 0.19; corpus 0.04, 0.05                    | Not W2's; the candidate's course heading meets it by construction      |
| G5  | Body centre 0.41 (1,108 rows); corpus median 0.60 (0.45 to 0.80 by site)                                                                           | 0.40 (1,103 rows); corpus 0.66 (0.50 to 0.81)          | Not met, and not met by construction (amendment 2)                     |
| G6  | Length 0.036, width 0.068; corpus 0.025, 0.058                                                                                                     | 0.037, 0.048; corpus 0.018, 0.040                      | Not met: the frame's span against a window-minimum floor (amendment 2) |
| G7  | 5 of 19                                                                                                                                            | 11 of 20                                               | Unchanged by W2                                                        |
| G8  | Car 4.05 of 4.26 m (7 of 7 within 15 %); truck 1 6.38; truck 2 1.96 reported                                                                       | Car 3.91; truck 1 8.88; truck 2 0.75                   | Unchanged in substance: the floor, not the belief, moved               |
| G9  | 3 of 7                                                                                                                                             | 3 of 7                                                 | Unchanged                                                              |
| G10 | kirk0 p99 0.291 to 0.156 m; corpus body centre +0.007 m (13 / 8), all windows −0.070 m (2 / 19)                                                    | 0.152 to 0.156 m; +0.017 m (17 / 4), −0.074 m (5 / 16) | Not met as written; met on the same windows (amendment 1)              |
| G11 | 93, 0.306, 0.275                                                                                                                                   | 118, 0.310, 0.272                                      | Met, both inside the 19-switch band                                    |
| G12 | As screened p99 7.30 ms (+71 %), shadow +102 %; with the floor's search capped (`c5bb9fcca`) 4.15 ms (−3 %), shadow +15 %, both medians about 2.5× | Not timed                                              | Met on A2 after the fix; not on the shadow                             |

**W1b, `solid_body_rectangle_heading`**, its first build, with `solid_body_rectangle_course_fusion`
built during the same run: kirk0 at `be8ace881` and `d5ff96fb6`, the corpus from passes 5 and 6,
all in the [rectangle heading report](solid-body-rectangle-heading-2026-10.md). `a2-new-cf` is A2
with the rectangle heading, fusion, containment, growth admission, open-prior centring and the
vehicle floor: the alignment candidate with the rectangle heading in the course heading's place.

| Id  | Rectangle heading + containment (`a2-rh-cont`)                                               | `a2-new-cf`                                                                                                   | Against the level                                                     |
| --- | -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| G1  | 0° median, 1.3° p90; truck 2 0.3°                                                            | 0°, 1.2°; truck 2 0.4°                                                                                        | Met on kirk0, a fit scored against fits                               |
| G2  | 13 of 15; truck 2 6 of 6                                                                     | 14 of 15; truck 2 6 of 6                                                                                      | Met; car 8 at 0.8 m/s on a 0.3 s-old track the one miss               |
| G3  | kirk0 0.04 / 0.04; corpus 0.060 / 0.067, better than A2 at all 23 sites                      | kirk0 0.02 / 0.03; corpus 0.040 / 0.043, 10 sites under 0.05 at both ages                                     | Met by `a2-new-cf`, whose course is fused into the axis (amendment 3) |
| G5  | kirk0 0.48; corpus 0.68                                                                      | kirk0 0.58; corpus 0.70                                                                                       | Not met (amendment 2)                                                 |
| G6  | 0.010 / 0.023                                                                                | 0.012 / 0.014                                                                                                 | Not met, nearer than W2 alone (amendment 2)                           |
| G7  | 5 of 9                                                                                       | 11 of 13                                                                                                      | Met by `a2-new-cf` on kirk0                                           |
| G8  | 6 of 15                                                                                      | 13 of 15                                                                                                      | Met by `a2-new-cf` on kirk0                                           |
| G9  | 0 of 7 (truck 1 matched to its cab fragment)                                                 | 3 of 7                                                                                                        | Not met: truck 2's cab never reaches the tracker (W3)                 |
| G10 | kirk0 0.291 to 0.200 m; corpus +0.029 m against A2 with containment (17 / 6), matched p99 up | kirk0 0.136 m; corpus +0.001 m against A2, +0.009 against the alignment candidate, matched p99 0.171 to 0.171 | Not met by the rectangle heading alone; level for `a2-new-cf`         |
| G11 | 108                                                                                          | 110                                                                                                           | Met, both                                                             |
| G12 | +10.9 % at p99 (rectangle heading alone, without containment), p50 2.8×; shadow +10.3 %      | −1.5 % at p99, p50 4.9×                                                                                       | Met by `a2-new-cf`; the heading alone just outside                    |

With the fit's σ at 1.5 (`solid_body_rectangle_sigma_mid`), the full arm reads G1 0° / 1.0°, G3
0.026 / 0.029 at the corpus median (the fit's own axis 0.061 / 0.093), G7 10 of 13, G8 13 of 15,
G10 a matched-window p99 of 0.162 m against the alignment candidate's 0.175 and the body-centre
p99 0.004 m below it at 15 of 23 sites, and G11 124 switches, five outside the band.

## Amendments

Proposed 2026-10-09 from the W2 and W1b scores, none adopted: each changes a level or its test,
which is a decision this record only reports. Until one is adopted the levels above stand as
written and W2 and W1b are scored against them.

1. **G10's population.** The body-centre p99 is read over windows of five consecutive
   body-centre rows, and a change that keeps a claim where it used to lapse moves rows into
   that population: under W2 it grew at 21 to 23 of 23 sites while the lateral on the windows
   both arms score was unchanged (2,976 matched windows, median paired difference 0.000 m).
   Proposed test: the p99 over every scored window (`anchor_point_estimates_all`) beside the
   body-centre figure, and the matched-window comparison as the operational form, with the
   windows an arm adds and loses reported apart. The level, not above the baseline's, would
   apply to the all-window figure and to the matched windows.
2. **G5 and G6's floor.** "Met by construction once W2 ships" was wrong as W2 is built: the
   reported box is floored at the window-minimum span (the smallest 1 %-trimmed extent over axes
   within 10° of the believed one), which understates a cluster growing or turning past the
   window and leaves the outermost 1 % of returns outside by design, and the position is held
   only on the axes a fix left open. The held share is 0.60 to 0.66 at the corpus median and
   rows that took a floor hold fewer of their points (0.88 to 0.93 at hyde-ofarrell) than rows
   that did not (0.99). Proposed: state the floor's definition in G6's test and read its
   shortfall against the window minimum, not the frame's span, which makes the reported box's
   shortfall zero by construction as intended; set G5's level on the share the trimmed floor can
   reach (0.98 of points within 0.15 m is above the trim), or change the floor to the frame's
   untrimmed span and re-score. Which of the three mechanisms accounts for most of the gap is
   not attributed; the per-row record distinguishes floored rows for that.
3. **G3 under a fused course.** With `solid_body_rectangle_course_fusion` the course is a
   second observation of the axis, so the axis column of `axis_by_age` is partly the course
   agreeing with itself, as the course heading's always was. The fit's own axis against the
   course is in the same table's rectangle columns, which this run found folded modulo 180
   degrees where the fit is defined modulo 90 (corrected at `cc28d59c6`). Proposed: read G3 on
   the rectangle columns whenever the course is fused into the axis, and on the axis columns
   otherwise.
