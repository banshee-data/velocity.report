# Minnesota fixed-camera behavioral thresholds

This note assesses a Minnesota camera-based driving study as a source of attributed research screens, with its context kept separate from local safety claims.

- **Status:** Research assessment; no metric implementation or safety threshold adopted.
- **References:** `Shaer2026`; [canonical bibliography](../../../data/maths/references.bib).
- **Canonical:** [behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md).

## Evidence

Shaer, Fielbaum, and Levinson, [Detecting dangerous driving via computer vision: Linking video-based indicators to road crashes](https://doi.org/10.1080/19439962.2025.2608002), _Journal of Transportation Safety & Security_ (2026). The briefing describes 258 hours at 31 Minnesota road segments and historical crashes, using operational definitions: speeding ≥10 km/h above posted limit, short THW <2 s, and critical TTC <2 s. Bibliographic identity is verified; the full-text event table still needs independent verification before public use.

## Interpretation

These are _study-specific event screens_, not universally validated unsafe cutoffs. The reported study context is straight road segments, dry conditions and uncongested traffic; these inclusion rules materially constrain comparisons with urban San Francisco intersections.

## Measurement assessment

Speeding requires a time-valid posted speed limit and valid passage speed; THW requires correctly assigned same-path leader and follower plus bumper gap; TTC additionally requires reliable closing velocity. Distinguish events, time-at-risk, and eligible opportunity denominators. Do not label an episode aggressive merely because one threshold is crossed.

## Proposed validation

Reproduce all three screens on labelled fixed-sensor episodes; compare sensitivity to thresholds and confidence/occlusion filters. Store threshold profile identifier, citation, revision, units and exact eligibility. No production threshold changes in this PR.

## Open questions

Which full-text definitions and inclusion rules support the reported event screens? How sensitive are the comparisons to threshold choice, jointly observed exposure, and trajectory uncertainty?
