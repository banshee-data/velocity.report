# End-on truck windows in the corpus and the archive: the W7 spike

<!-- ignore-style-length -->

The [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md) needs
a second capture with a truck approaching end on, reviewed and frozen, before any of its
candidates can be scored held out (W7). Its first step was a week-one spike: count how many
such windows the data already holds, so the capture is selected by a rule rather than found
by looking for the failure. This is that count.

- **Status:** Complete, 2026-10-09. A label-free count from existing replay evidence; no capture reviewed
- **Layers:** L5 solid body (as the instrument), offline evaluation
- **Related:** [convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md#6-work-to-scope-and-schedule) (W7), [alignment report](solid-body-physical-alignment-kirk0-2026-10.md), [kirk0 pilot](physical-reference-pilot-kirk0-2026-10.md), [physical-reference review plan](../../plans/lidar-physical-reference-review-plan.md)

## Summary

- The 23-site corpus (77 scored minutes) holds **33 clean end-on truck windows** of a second or
  more within 40 m, in 146 long tracks out of 7,183. Only **3** of them are end on in the sense
  kirk0's truck 2 was, the end face alone in the fix for most of the window; the rest show a
  side as well, at 7 to 25 m.
- The archive is 24 sites and 514 minutes, 6.7 times the corpus's scored time. At the corpus's
  rate it holds on the order of **200 oblique and 20 truly end-on windows**. A second end-on
  truck does not need fieldwork; it needs a selection rule over segments the corpus did not use.
- The three truly end-on corpus windows are at 1st-mission (track 775, 157 to 166 s into the
  segment, 9 s approaching, nearest 20 m), 3rd-folsom (track 730, 237 to 249 s, 12 s, 23 m)
  and 1st-mission (track 849, 175 to 186 s, 6 s, 10 m). They are tuning material: a window
  chosen because a truck is end on is chosen for the failure, and cannot be the held-out one.

## Method

The instrument is the solid body's own rows from the corpus passes of the alignment run (the
`a2-allo` arm, growth admission on, so lengths accumulate), read from the 23 site evidence
databases. No labels. A candidate is a track that:

1. reaches a believed length of 6.5 m or more from accumulated evidence (a long body; the
   class is unknown for nearly every row, so length stands in for "truck");
2. spends at least 1.0 s at 2 m/s or more with its course within 25° of the line to the sensor,
   approaching or receding (the end-on geometry);
3. is at most 3.2 m wide at any row of that window (a merge of two bodies is wider).

"Truly end on" is a candidate whose fix uses an end face alone for at least half the window's
rows, which is what a truck looks like to the near-edge model until its side comes into view.
The archive scale is the S2 site index's minutes per site; the corpus scored 200 s per site
after a 70 s warm-up.

Everything here inherits the instrument's errors: a merged pair can read as one long body
(1st-mission and lombard-laguna are the corpus's merge-heavy sites), an end-on truck whose
length never accumulated is missed, and the course is the tracker's. The count is a scale, not
a census.

## Findings

### The corpus

| Site                  | Tracks | Long (≥ 6.5 m) | End-on ≥ 1 s | Within 40 m |
| --------------------- | -----: | -------------: | -----------: | ----------: |
| 1st-mission           |    566 |             19 |            8 |           6 |
| van-ness-sacramento   |    463 |             13 |            5 |           5 |
| lombard-broderick     |    521 |             10 |            4 |           4 |
| lombard-laguna        |    525 |             10 |            4 |           4 |
| 3rd-folsom            |    365 |              4 |            3 |           3 |
| columbus-broadway     |    516 |              9 |            3 |           3 |
| embarcadero-broadway  |    442 |              6 |            3 |           3 |
| california-leon-baker |    115 |              3 |            2 |           2 |
| bush-powell           |    379 |             14 |            1 |           1 |
| columbus-north-point  |    264 |              7 |            1 |           1 |
| franklin-mcallister   |    407 |             10 |            1 |           1 |
| twelve other sites    |  2,620 |             41 |            0 |           0 |
| **Total**             |  7,183 |            146 |           35 |          33 |

Thirteen of the 35 have a long end-on stretch of 5 s or more. Of the 33 within 40 m and under
3.2 m wide, 20 approach the sensor and 13 recede; 3 are truly end on. With the `a2` arm, whose
lengths do not accumulate past a merge candidate, the count is 21: the instrument's own length
estimate decides what counts as long, which is the point the convergence plan makes about it.

The twelve sites with none are the cross-streets and the two marina placements, where traffic
passes the sensor broadside; the sites with several are the long straights (1st, 3rd, van Ness,
Lombard, the Embarcadero) where a lane runs at the sensor.

### The archive

| Measure                    | Corpus (scored) | Archive            |
| -------------------------- | --------------: | ------------------ |
| Sites                      |              23 | 24                 |
| Minutes                    |              77 | 514                |
| Clean end-on windows ≥ 1 s |              33 | about 220, by rate |
| Truly end on               |               3 | about 20, by rate  |
| Sites with end-on geometry |              11 | the same eleven    |

The archive's unscored time is the corpus's own sites, 15 to 35 further minutes each, with the
same geometry. embarcadero-folsom, held out of every corpus pass, is 20 minutes on a straight
with the Embarcadero's traffic and has never been read.

### Candidates for tuning review

The longest clean windows, for an operator who wants a truck end on in the annotation window
now. Offsets are seconds after the segment's first solid-body row.

| Site                 | Track | Length | Window       | Nearest |    Speed | End face alone | Direction   |
| -------------------- | ----: | -----: | ------------ | ------: | -------: | -------------: | ----------- |
| 1st-mission          |   775 |  8.1 m | 157 to 166 s |    20 m |  8.9 m/s |           79 % | approaching |
| 3rd-folsom           |   730 |  7.6 m | 237 to 249 s |    23 m | 10.2 m/s |           47 % | approaching |
| 1st-mission          |   849 |  7.4 m | 175 to 186 s |    10 m |  5.8 m/s |           45 % | approaching |
| 3rd-folsom           |   474 |  9.1 m | 156 to 166 s |     7 m |  9.9 m/s |           38 % | approaching |
| franklin-mcallister  |   605 |  7.9 m | 103 to 111 s |     7 m | 10.7 m/s |           31 % | approaching |
| embarcadero-broadway |   736 |  7.9 m | 191 to 207 s |    11 m |  8.6 m/s |           22 % | approaching |

## The capture request

The convergence plan asks for one capture request serving W7, the facet operator pilot's
side-to-end episode and the cars-beyond-50 m item. This spike turns it into a selection rule,
to be written down before any window is opened:

1. **Tuning.** 1st-mission track 775 and 3rd-folsom track 730, the two truly end-on windows with
   the most points, reviewed and frozen as a tuning split. These are chosen for the failure and
   say so; they let W1 and W2 be developed against an end-on truck that is not kirk0's.
2. **Held out.** One 5-minute segment the corpus did not use, from one of the eleven sites with
   end-on geometry, chosen by lot among those segments and reviewed whole: every long body in
   it, end on or not, with operator-keyframed yaw and extents so the references are not fits to
   the returns the tracker reads. By the corpus's rate a segment there holds one or two clean
   end-on windows and about one chance in four of a truly end-on one; draw a second if the first
   has none, and record both draws. embarcadero-folsom is the natural first lot: it is the one
   site nothing has read.
3. **The same segment serves the other two.** A 5-minute straight-road segment at 1st, 3rd or
   the Embarcadero carries cars beyond 50 m and side-to-end view changes as a matter of course;
   the facet pilot's episode and the range item draw from the held-out segment's review rather
   than from a capture of their own.

What this does not establish: that any candidate is a truck rather than a long merge, until an
operator opens it; the truly end-on rate beyond three cases; or anything about sites the
archive does not hold. The operator queue is the constraint the plan names, and the held-out
review is one segment, not several.

## Provenance

- Corpus evidence: the alignment run's pass 3 (`a2-allo` at be79fbd53) and pass 1 (`a2` at
  17f2719e9), 23 sites, under `work/velocity-campaign/physical-align-corpus-20261009` on the
  LiDAR volume, read from local copies.
- Archive scale: `tools/s2-archive/site-index.json`, 24 sites.
- Script: `endon-spike.py`, with the run's other scripts outside the repository.
