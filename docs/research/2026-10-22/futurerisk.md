# FutureRisk — experimental composite surrogate

This note assesses FutureRisk as an experimental trajectory-safety surrogate and identifies the evidence needed before considering a fixed-LiDAR comparison.

- **Status:** Research assessment; no metric implementation or safety threshold adopted.
- **References:** `Luo2026`; [canonical bibliography](../../../data/maths/references.bib).
- **Canonical:** [behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md).

## Evidence

Luo et al., _FutureRisk_, arXiv preprint (October 2026), https://arxiv.org/abs/2610.17806. Briefing-reported benchmark results: collision AUROC 0.793 versus forward-TTC 0.685, AUPRC 0.490 versus a learned baseline 0.452, on DeepAccident. Verify comparator identity, split protocol, class balance and calibration in the paper. These are **classification benchmark metrics**, not real-world crash-risk probabilities.

## Interpretation

The method reportedly combines physical safety-time indicators with learned context and uses context-specific high-percentile acceleration-severity cutoffs. The learned composite is not an explainable, cross-city calibrated public benchmark without external validation.

## Research option

Evaluate in a sandbox against TTC, PET, DRAC and physically interpretable event distributions on held-out fixed-LiDAR interactions. Require calibration, class/geometry strata, failure cases and domain-shift checks. No implementation or public score change recommended.

## Open questions

Which benchmark splits, comparator definitions, and calibration results are confirmed by the paper? Does performance transfer to qualified fixed-LiDAR encounters and their missingness patterns?
