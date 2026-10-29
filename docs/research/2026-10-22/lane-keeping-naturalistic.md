# Lane keeping, lane departures and lane changes

This note separates lane-position variation, boundary departures, and lane changes, and outlines the references needed to measure each with fixed roadside LiDAR.

- **Status:** Research assessment; no metric implementation or safety threshold adopted.
- **References:** `Ali2026`; [canonical bibliography](../../../data/maths/references.bib).
- **Canonical:** [behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md).

## Evidence

Ali et al., [Characterization of Lane Keeping and Departure Scenarios Using Large-Scale Naturalistic Driving Data](https://doi.org/10.4271/09-14-01-0022), _SAE International Journal of Transportation Safety_ (2026), described in the briefing as using over 36 million naturalistic miles from more than 3,800 participants. Reported approximately three million lane changes and 300,000 lane/road departures, with many routine departures at shallow 0–2° crossing angles. Confirm exact cohort, definitions and angle distributions against the full paper.

## Separate outcomes

(1) lateral lane-position variability when lane assignment is stable; (2) lane-boundary crossings/departures, with crossing angle and direction; (3) intentional lane changes with duration, lateral velocity and accepted front/rear gaps. A roadside sensor generally **cannot determine intention** from trajectory alone; label observed crossing versus inferred lane-change intent separately.

## Fixed-sensor constraints

A reliable lane map and stable pose are prerequisites; lane-position SD can be biased by curved lanes, sensor range and truncated tracks. Report opportunity and visibility denominators and avoid comparing lane-change rates without accounting for road geometry.

## Validation

Use labelled straight/curved roads, deliberate changes versus small boundary encroachments, pose perturbations, occlusions, lane-map offsets and sampling-window sensitivity. The 0–2° finding is a descriptive distribution, not a new safety cutoff.

## Open questions

Which cohort counts and departure-angle definitions are confirmed in the paper? How accurately can lane assignment, crossing geometry, and eligible exposure be established without inferring driver intention from motion alone?
