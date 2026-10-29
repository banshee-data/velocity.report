# Human following behind autonomous vehicles

This note frames an observational comparison of human following around identifiable automated-vehicle fleets and records the measurement and inference limits.

- **Status:** Research assessment; no metric implementation or safety threshold adopted.
- **References:** `Mironenko2026`, `Chu2026`; [canonical bibliography](../../../data/maths/references.bib).
- **Canonical:** [behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md).

## Evidence

Mironenko, Banaee, and Loutfi, [A taxonomy of human driver errors and unsafe adaptations in mixed traffic](https://doi.org/10.1186/s12544-026-00851-z), _European Transport Research Review_ (2026), described in the briefing as a synthesis of 37 studies and 49 evidence cases. Verify study coverage and coding against the full review. Related primary study: _Automated vehicles drive safer but induce riskier behavior in human drivers in car-following scenarios_, _Communications Engineering_ (2026), https://doi.org/10.1038/s44172-026-00793-3. <!-- spelling-ignore -->

## Observable hypothesis

Under comparable conditions, do human followers exhibit different THW distributions when the leader is a confidently classified Waymo/Zoox fleet vehicle rather than a conventional vehicle? Distinguish **vehicle fleet/model identification** from **verified automated-driving engagement**. LiDAR alone cannot establish human motive, trust, or strategic exploitation.

## Study design

Use same-site/lane/time comparisons, predeclared episode-level estimand, site-specific effects and hierarchical model with site heterogeneity. Consider leader speed and acceleration variability, traffic density, road enclosure, geometry, occlusion and matching/mediator bias. Use pilot variance to simulate power; 3,000 episodes is not an established requirement.

## Current measurement blockers

The [October 20 opportunity report](../../lidar/operations/following-opportunity-corpus-2026-10.md) reports median track-pair duration 0.5 s, centroid-based gap and substantial identity fragmentation. These are **not yet** validated independent following episodes. First validate episode continuity, physical bumper gaps, vehicle classification and selection denominators.

## Proposed disposition

Preregister a noncausal observational comparison with equivalence analysis and cross-site sensitivity; do not publish an AV safety or aggression score.

## Open questions

Can eligible following episodes and leader class be established independently of tracker fragmentation? What matching, equivalence margin, and exposure support a comparison without treating fleet identity as automated-driving engagement?
