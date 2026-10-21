# Dong2026: lane-constrained identity links through occlusion

- **Status:** Source and implementation review; no new replay measurements
- **Scope:** Motor-vehicle track fragments within one fixed LiDAR placement
- **Source:** Dong et al., Sensors 26(12):3755, [publisher](https://www.mdpi.com/1424-8220/26/12/3755), BibTeX `Dong2026`
- **Related:** [Delivery plan](../../plans/lidar-lane-stitching-plan.md), [identity experiments](../../../data/experiments/l5id/README.md)

## Answer

The paper has already had a full-text literature review, a bibliographic audit, and an explicit
adoption boundary: identity links may be useful, but interpolated gaps are not observations.
There is no recorded Dong-specific reproduction or lane-based fragment stitcher in this checkout.
The closest queued experiment, `l5id0006`, measures a 50 m count alongside simultaneous duplicate
suppression. It does not reconnect successive, disjoint tracks.

The requested workflow is a useful next experiment: draw directed lane segments, project vehicle
observations into distance along each segment, and associate compatible fragment endpoints.
Permit transitions only through declared successors and bounded merge regions. Keep raw tracks
and missing intervals intact. A derived identity can contain A, B, and C without pretending that
the two gaps between them were measured.

## What review already exists

| Record                                                                                                                                       | Work already done                                                                                                               | What it does not establish                                                        |
| -------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| [Research briefing, fragment stitching](brief/research-briefing-2025-01-to-2026-10.md#fragment-stitching-dong-et-al-2026-and-luo-et-al-2025) | Records Dong and Luo as read in full; reports thresholds, count errors, and the no-observation rule; points identity work to W5 | No Pandar40P reproduction, held-out identity result, or production adoption       |
| [Reference audit](brief/reference-audit.md) and [bibliography](../../../data/maths/references.bib)                                           | Identifies `Dong2026`, DOI, authors, and journal; added in the October reference audit                                          | Bibliographic verification is not experimental verification                       |
| [Behaviour plan §9.2](../../plans/lidar-behaviour-analytics-plan.md#92-coasting-is-not-observation)                                          | Coasting and interpolation cannot supply valid observed following time                                                          | Does not implement fragment association                                           |
| [l5id0006](../../../data/experiments/l5id/l5id0006.md)                                                                                       | Cites Dong; proposes a 50 m count measure alongside overlap-end, box inheritance, and tentative priority                        | Simultaneous duplicates and later reappearance are different association problems |

The briefing landed in `f1deb23e1` (#723), the bibliography audit in `a1512d63b` (#725),
and `l5id0006` in `39be9f13f` (#778). These are checked repository-history records.
The present review adds the algorithm audit, code mapping, merge extension, and queued tests.

## What the paper actually supplies

Sections 3.3–3.5, pages 8–12, describe a post-processing method:

1. Filter vehicle trajectories into rotated rectangular lane zones. Establish a longitudinal
   axis whose increasing coordinate follows the dominant traffic direction.
2. Estimate endpoint motion from five points. Identify likely internal disappearances and
   reappearances using predicted endpoints and a boundary tolerance.
3. Screen tail/head pairs using time, longitudinal separation, eligibility at both ends, and
   a limited temporal-overlap check.
4. Accept exclusive matches, retain the earlier identifier, and join the fragments. Display
   interpolation across the gap; the paper explicitly disclaims recovery of the hidden path.

The reported starting thresholds are 2.5 s, extended to 3.5 s when the five-point window contains
negative velocity; 20 m spatial separation; 0.2 m boundary tolerance; and up to three overlapping
samples within 5 m at 10 Hz. These are site-tuned values, not transferable sensor constants.
Negative velocity may reflect centroid movement or noise; it does not establish a hidden stop.

### Reported evidence

Table 1 uses four 30-minute periods at one Reno intersection. These are the paper's measurements,
not velocity.report results. The averages are the table's period averages, not a pooled score.

| Period           | Link precision | Link recall | F1    | Raw count error | Corrected count error |
| ---------------- | -------------- | ----------- | ----- | --------------- | --------------------- |
| Weekday AM       | 1.000          | 0.960       | 0.980 | 26.4%           | 5.7%                  |
| Weekday PM       | 0.955          | 0.955       | 0.955 | 18.5%           | 2.2%                  |
| Weekend AM       | 1.000          | 1.000       | 1.000 | 6.8%            | 0.0%                  |
| Weekend PM       | 1.000          | 0.750       | 0.857 | 6.2%            | 2.5%                  |
| Reported average | 0.989          | 0.916       | 0.948 | 14.5%           | 2.6%                  |

Table 3's single weekday-PM comparison is separate: 10 m zones improve from 2.5% to 1.7%,
50 m from 18.5% to 2.2%, and 150 m from 35.8% to 5.3%. Zone-specific manual vehicle counts
are 119, 135, and 151: these are not interchangeable denominators. Longer zones include more
opportunities for lane changes and identity ambiguity.

Table 2's 1.54 s runtime and 0.980 F1 describe one AM comparison with 139 fragments and
5,999 frames. They are not an online latency measurement or a Raspberry Pi budget.
Section 4.6 selects thresholds for best F1 on the validation data; it does not supply an untouched
external test. Validation is one site in fair weather, on motor vehicles. Data are available on
request; this review did not obtain the authors' data or code.

### Reproduction questions that must be settled first

Visual inspection of the PDF, rather than extracted text alone, confirms two printed issues:

- Equation 10, page 9, adds a time to a distance in one interval endpoint (`S_min + Δt`).
  That is dimensionally inconsistent. The prediction horizon and backward-head convention also
  need an explicit implementation contract.
- Equation 15, page 11, uses the same fragment index for the head and tail where the surrounding
  text describes two fragments. Equation 16 and the prose provide context, not a complete fix.
- The lower time/spatial bounds, candidate ordering, tie-breaking, and repeated chain merging
  are not fully specified. Section 3.5.4 says a segment participates in at most one matching
  pair, yet later discussion reports chains. Specify one predecessor and one successor for
  our chain model; do not present that interpretation as a verified author implementation.
- Equation 18 includes absolute separation in the rendered PDF. Text extraction can drop its
  bars. Implementing the extraction literally would admit a signed-distance error.

Record each interpretation in the implementation report and unit fixtures before scoring.
Call the result a **Dong-like baseline** until resolved against author code or clarification.
The method's boundary heuristic identifies candidate missing observations; it is not direct
proof that an occluding vehicle blocked a ray.

## Fit to the current code

| Existing component                                                                                                                      | Reuse                                                                                                              | Missing capability                                                                                        |
| --------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------- |
| [L5 continuity](../../../internal/lidar/l5tracks/continuity.go)                                                                         | Observation-support vocabulary, last-observed time, expiry reasons, optional absence explanation and bounded coast | Durable association of separately ended tracks                                                            |
| [L5 identity](../../../internal/lidar/l5tracks/identity.go) and [association](../../../internal/lidar/l5tracks/tracking_association.go) | Geometry/class checks, ambiguity refusal, Hungarian assignment concepts                                            | Candidate search over whole fragments, chain consistency, lane-transition gates                           |
| [Overlap end](../../../internal/lidar/l5tracks/overlap_end.go)                                                                          | Separate control for two tracks explaining one body at the same time                                               | Cannot be assumed to solve temporally separated fragments                                                 |
| [Local path](../../../internal/lidar/l8behaviour/localpath.go) and [path frame](../../../internal/lidar/l8behaviour/pathframe.go)       | Arc length, lateral offset, tangent, provenance, and finite supported extent                                       | Persistent lane identifiers, topology, merge handover; local-path fitting explicitly refuses forks/merges |
| [Following evaluator](../../../internal/lidar/l8behaviour/following.go)                                                                 | Support-aware suppression; unobserved samples remain ineligible                                                    | Adapter from linked identities that preserves every support interval and avoids self-pairing              |

L5 must not import L8 to obtain path geometry. The first experiment can use an offline adapter;
production reuse requires a small neutral geometry boundary or explicit inputs from composition.
The full L7 scene-map project is not a prerequisite for a manually reviewed local segment graph.

## Lane and merge workflow

The screenshot is a good review pattern: point cloud and selected zone on one side, synchronised
time–distance traces on the other, with wrong-link and missed-link controls. Its displayed totals
are an example of the tool, not independent validation or the paper's aggregate result.

Use a directed centreline plus a corridor for each segment. Project each observed body anchor to
`s` (metres along the centreline), `d` (signed lateral distance), and heading relative to the
local tangent. Keep the anchor's provenance consistent: a centroid shift can resemble acceleration.
Use overlapping membership with hysteresis near a lane boundary, and retain unknown membership.
The analysis view can restrict itself to chosen lanes; upstream returns and off-lane tracks must
remain available so that a merge, occluder, cyclist, or turn is not cropped out of the evidence.

At a merge, allow A→C and B→C only inside the declared transition region. Compare endpoints in
a common route distance, including the distance to and from the shared node; A's local `s=20`
and C's local `s=20` do not identify the same place. If two cars compete for C, exclusivity and
an ambiguity margin must leave the unresolved link open. A road graph may branch; one vehicle's
identity chain may not. Lane changes outside a reviewed transition stay unlinked in the first
version and appear as a measured recall limitation.

An occlusion can break one vehicle into A→B→C. Candidate links form a graph ordered in time.
Each fragment has at most one incoming and one outgoing accepted edge. Reject cycles, impossible
travel, conflicting simultaneous observations, and chains whose combined history contradicts an
earlier edge. Independent nearest-neighbour joins are insufficient when two cars disappear together.
Simultaneous shared clusters need upstream split/duplicate treatment; a linker cannot recover two
observations from one merged detection.

## Recommendation and limits

Queue the five experiments in the [plan](../../plans/lidar-lane-stitching-plan.md), starting with
endpoint truth and map feasibility. Keep the linker offline and in shadow. Add live fixed-lag
revision only after frozen held-out identity evidence, consumer semantics, and target timing pass.
Better counts must not be purchased with wrong identities, and link confidence is not a calibrated
probability until measured as such.

No replays, physical tests, parameter adoption, or algorithm implementation were performed for
this review. The new acceptance thresholds are proposed engineering rules, not paper findings.

## Provenance

- Source checkout: `a71d5fc4df1cc4141f6504f4766dea990f3cd5ba`, equal to the locally available
  `origin/main` ref when inspected; no claim of a fresh remote fetch.
- Local paper: `/Volumes/lidar/lidar/papers/pdf/Dong2026.pdf`, 24 pages, 27,897,636 bytes.
- Paper SHA-256: `1226105485f8c499ce198d8a60e0c146039be1169e2b2db4c52f296361ae9e71`.
- Methods checked against pages 8–12; evaluation and limits against pages 13–22. Equations on
  pages 9 and 11 checked visually. Publisher record and version notes checked on October 21,
  2026 UTC; the PDF creation metadata corresponds to the June 18 update.
- User-supplied screenshot: `Screenshot 2026-10-20 at 8.17.12 PM.png`; reviewed as workflow
  evidence, with no inference that its displayed perfect score establishes general accuracy.
- Extracted text and rendered inspection pages remain in `/tmp/dong2026-review/`, outside Git.
