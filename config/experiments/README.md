# Experiment parameter sets

Each file here is a complete tuning config for one experiment arm: the
whole schema, loadable on its own, checked in so a later run can name
exactly what an earlier one ran. Each is
[tuning.defaults.json](../tuning.defaults.json) with the arm's changes, and
its row below names every key it changes. A test holds the files to their
rows, so a set copied from an older defaults file fails instead of running
as something other than its arm.

How to run one, build a new one and map replay experiment names to keys:
[CONFIG.md](../CONFIG.md#experiment-parameter-sets).

| File                                                                       | Changes from the defaults                                                                                                                                                                                                  | Report                                                                                                                      |
| -------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| [solid-body-shadow.json](solid-body-shadow.json)                           | `solid_body`: the estimator as a shadow beside the tracked filter, with face hysteresis (T1), course-aligned faces (T3) and the full members. The tracks are the defaults'.                                                | [Near-edge campaign](../../docs/lidar/operations/near-edge-campaign-2026-10.md), arm Shadow                                 |
| [solid-body-a2.json](solid-body-a2.json)                                   | `solid_body`: the shadow's options plus `near_edge_tracking`, so the near-edge measurement drives the tracked state.                                                                                                       | [Near-edge campaign](../../docs/lidar/operations/near-edge-campaign-2026-10.md), arm A2                                     |
| [alignment-candidate.json](alignment-candidate.json)                       | `solid_body`: A2 with course heading, extent growth admission, open-prior end-face centring, the vehicle extent floor and containment.                                                                                     | [Containment on the corpus](../../docs/lidar/operations/solid-body-containment-corpus-2026-10.md), arm `a2-allo-cont`       |
| [geometry-convergence-candidate.json](geometry-convergence-candidate.json) | `solid_body`: A2 with extent growth admission, open-prior end-face centring, the vehicle extent floor, containment, the rectangle heading with course fusion, and the rectangle fit's sigma at 1.5. The current candidate. | [Rectangle heading](../../docs/lidar/operations/solid-body-rectangle-heading-2026-10.md), the full candidate at sigma × 1.5 |
