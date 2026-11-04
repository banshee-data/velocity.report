# Gerardus's lane proposals against lanes drawn by hand, at kirk and columbus-broadway

This report answers [lane0002](../../../data/experiments/lane/lane0002.md): do Gerardus's `high`
lane proposals sit on the travel lanes a person draws without seeing them, and does each
direction get the right number of lanes? It is also the real check of Phase E of the
[lanes from tracks plan](../../plans/lidar-lane-proposals-from-tracks-plan.md).

- **Status:** Not run. The rule, the scoring tool and this skeleton are in place; the run waits on
  the owner's drawing (the `draw` step of the spec) and on lane fields uploaded to each site's
  Gerardus
- **Layers:** L8 (the lane field), Gerardus (the proposals), the painter (the drawn reference)
- **Related:** [lane snapshot and attribution](../../plans/lidar-lane-snapshot-and-attribution-plan.md#item-2-experiment-lane0002-gerarduss-proposals-against-drawn-lanes),
  [the first lane field](lane-field-columbus-broadway-2026-11.md),
  [Geometry priors: data purpose](../../geometry-priors-data-purpose.md)

The drawn lanes and the proposals are measured spatial data: the operator's reading of the street
and an inference from where vehicles were tracked, each at a stated time. Neither is a survey.

## Summary

To be written from the run.

## The rule

Written before the drawing (2026-11-04), in the spec. At each site separately: every `high`
proposal's lateral RMSE against its paired drawn lane under 0.30 m; per direction, `high` plus
`medium` proposals equal to drawn lanes; at least 80 % of drawn lanes found by a `high` proposal
(within 0.30 m over at least 70 % of the drawn lane); no `high` proposal on no drawn lane.

## Method

**Fields.** Each site's captures replayed in analysis mode with the defaults
([`tuning.defaults.json`](../../../config/tuning.defaults.json), fingerprint to be recorded), and
each run's three lane fields sent to the site's Gerardus against its accepted record.

**Drawing.** The owner draws every travel lane he can see in the painter with Proposals and Traffic
off, from the reference, the kerbs and the background only, tagged `reference=lane0002`, and
records the minutes each site took.

**Scoring.** `velocity lidar lanes score --gerardus <url> --site <l16> --reference-tag
reference=lane0002`: each proposal is paired with the drawn lane within 45° of its bearing whose
centreline it runs nearest; the lateral offset is sampled every metre of the proposal that projects
inside the drawn lane.

## Findings

| Site | Direction | Drawn | High | Medium | Pairs | Worst high RMSE (m) | Drawn found | False high |
| ---- | --------- | ----: | ---: | -----: | ----: | ------------------: | ----------: | ---------: |
|      |           |       |      |        |       |                     |             |            |

## Interpretation

To be written from the run.

## Limitations

- The drawn lanes are a reading of the street, not a survey: where paint is faint they can be off
  by a few decimetres themselves, which counts against the proposal.
- Two busy sites; a quiet site may produce fewer proposals.
- Gerardus's grade thresholds were set before this run, so this measures them rather than fitting
  them.

## Provenance

| Item     | Value |
| -------- | ----- |
| Captures |       |
| Build    |       |
| Tuning   |       |
| Gerardus |       |
| Records  |       |
