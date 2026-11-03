# A constrained road plane and a kerb split against the regional ground surface

<!-- ignore-style-length -->

The regional ground surface fits a least-squares plane to each 10 m region's lower quartile, so a
kerb, an island or a parked van's floor in the region pulls it. This report asks whether PCL's
perpendicular-plane MSAC, with kerb faces split out by a difference of normals, is the better road
prior on the 23 corpus sites. It answers [ground0006](../../../data/experiments/ground/ground0006.md),
whose rule's reading was fixed (`5cf1b0ce`) before the oracle ran.

- **Status:** Complete, 2026-11-03. The rule fails both clauses; the estimator is not proposed and
  no default changed
- **Layers:** L3 (the settled background the surfaces are fitted from), the P11 regional surface
  that the replay clip and `box_ground` read
- **Related:** [ground0001's surface read](ground-surface-corpus-read-2026-10.md),
  [PCL algorithm review, Q0](pcl-algorithm-review.md#q0-can-pcl-improve-road-and-kerb-geometry-enough-to-make-safety-metrics-more-reliable),
  [OQ-03 segmentation contracts](pcl-segmentation-contracts.md),
  [PCL oracle tooling](pcl-oracle-tooling.md)

## Summary

- **The constrained plane is worse than the regional surface at p95, at every site.** In the
  kerb-holding regions, p95 of the held-back road returns rises from 0.582 to 1.169 m at
  Marina at Webster Beach and from 0.403 to 0.638 m at Columbus at Broadway. Clause 1 needed a
  fall of 0.020 m at both, and both rise instead (−0.587 and −0.235 m). All 23 sites rise, by
  0.050 to 0.589 m.
- **Clause 2 fails in 413 of the 566 scored regions,** where the constrained p95 is more than
  0.010 m above the regional one. The worst is 1.95 m at Broadway at Franklin. The constrained p95
  is lower in only 84 regions.
- **At the median the two are close.** The median over regions of the median |residual| is
  0.065 m for the regional surface and 0.071 m for the constrained plane. The constrained plane
  has the lower median in 279 of 566 regions. MSAC fits the densest 0.05 m slab of the lowest
  returns. Least squares fits a compromise through all of them. So the constrained plane's tail
  is longer wherever the region's lowest quartile holds more than one surface.
- **The difference of normals does not find kerbs on these backgrounds.** It flags 26 to 41 % of
  every site's background points as candidates, 31 to 44 % between 0 and 30 m. Against the DataSF
  kerbs at the five sites with an operator-registered pose, 3.6 to 14.6 % of the candidates lie
  within 0.3 m of a kerb, against 3.2 to 7.6 % of all points. That is at most twice chance, and at
  ashbury-downey it is chance.
- **Levelling the frame does not rescue it.** Rerun with each background turned by its accepted
  pose's roll and pitch, so the 5° cone is about the vertical, the rule still fails: −0.368 and
  −0.290 m at Marina and Columbus, and 396 of 571 regions rise by more than 0.010 m. This rerun is
  descriptive and outside the rule.
- **The oracle meets its contract.** At all 1,938 regions, the returned inliers equal the points
  within 0.05 m of the returned plane, with no mismatches. The regional surface's C++ port matches
  `l3grid` to 0.9 mm at every region centre, with the same regions and supports.

## Question

On the 23 first-segment sites, does a perpendicular-plane MSAC fit lower the held-back road
residual? It is fitted to each region's lowest returns, with difference-of-normals kerb candidates
withheld. Marina and Columbus, the graded sites, decide. And do its kerb lines agree with
Gerardus's kerbs?

## Method

**Backgrounds.** Each placement's first 300 s was replayed in analysis mode on a scratch server
(`velocity-report-local` at `84492f9e`, built with `make build-radar-local`), with the defaults,
from the whole-placement captures on the NAS. The preserved grid was persisted and exported with
`bgasc -format pcd`, new in this PR. It writes the ASC export's points as ASCII PCD, in the same
order and with the same values. At 16 sites the window is the first five-minute segment. At the
seven 2026-09-01 sites, the first segment holds 32 to 262 s of the placement, and the window reads
on into the next. The exports hold 59,501 to 71,785 points. Each replay took 156 to 360 s beside
another lane's replays. A pilot replay of marina-webster-beach, made before the spec was approved,
gave the same oracle counts as the run's own replay.

**The oracle,** `tools/pcl-review/ground-oracle`, takes every parameter from the spec's reading:

1. **Hold-back.** Every fifth point by ordinal is held back; the other four fifths are the fit
   points.
2. **The regional surface.** It is refitted on the fit points by a port of
   `l3grid.FitRegionalGroundSurface`. The fit uses 10 m cells, the lower quartile, a 3 × median
   refit and a global fallback.
3. **Kerb candidates.** Normals at 0.3 and 1.2 m on every point use the fit points as the search
   surface and are flipped towards the sensor. A point is a candidate when its
   `DifferenceOfNormalsEstimation` magnitude exceeds 0.25.
4. **The constrained fit.** For each region with its own regional plane: the fit points at or
   below the region's lower-quartile height, less candidates. `SACSegmentation` runs with
   `SACMODEL_PERPENDICULAR_PLANE`, `SAC_MSAC`, axis +z, eps 5° in radians, threshold 0.05 m and
   1,000 iterations, with coefficient optimisation enabled. The returned mask is checked against
   the returned plane. Fewer than 12 fit points, or an empty mask, is unsupported, and the region
   keeps its regional plane.
5. **Residuals.** Held-back points at or below the lower-quartile height, less candidates, are
   the road returns. A region with 20 or more is scored. Residuals are vertical.
6. **Kerb lines.** The candidates within 0.3 m of a non-candidate, and no more than 0.30 m above
   the lower-quartile height, go to `SACMODEL_LINE` RANSAC (0.10 m). A line is kept with
   10 inliers or more, within 20° of horizontal and 2 m long or more.

**The frame** is the sensor's: the captures carry no gravity reference. **The Gerardus
comparison** puts each candidate into the site frame by the operator's accepted pose on the
Gerardus data (nightly snapshot of 2026-11-02, read only), and measures the horizontal distance to
the bundle's DataSF Curbs and Islands lines. The pose was registered at five sites and placed by
hand at the other 18.

## Results

### Against the rule

| Clause                                                 | Required    | Marina at Webster Beach | Columbus at Broadway | All 23 sites | Result |
| ------------------------------------------------------ | ----------- | ----------------------: | -------------------: | -----------: | ------ |
| 1. Kerb regions' pooled p95, regional less constrained | ≥ 0.020 m   |                −0.587 m |             −0.235 m |              | fails  |
| 2. Scored regions with constrained p95 > 0.010 m above | none of 566 |                 6 of 13 |             24 of 26 |   413 of 566 | fails  |

### Per site

Kerb candidates are the share of the site's background points. The p95s pool the held-back road
returns of the site's scored regions that hold a kerb line.

| Site                  | Regions | Unsupported | Scored | Scored with a kerb | Kerb candidates | Kerb p95, regional | Kerb p95, constrained |   Fall | Regions rising > 1 cm | Largest rise |
| --------------------- | ------: | ----------: | -----: | -----------------: | --------------: | -----------------: | --------------------: | -----: | --------------------: | -----------: |
| marina-webster-beach  |     121 |          11 |     13 |                 11 |            41 % |              0.582 |                 1.169 | −0.587 |               6 of 13 |        1.114 |
| columbus-broadway     |      88 |           1 |     26 |                 20 |            30 % |              0.403 |                 0.638 | −0.235 |              24 of 26 |        0.479 |
| 1st-mission           |     107 |           3 |     32 |                 20 |            34 % |              0.225 |                 0.383 | −0.157 |              18 of 32 |        0.536 |
| 3rd-folsom            |      84 |           1 |     26 |                 22 |            31 % |              0.491 |                 0.832 | −0.341 |              17 of 26 |        0.521 |
| ashbury-downey        |      53 |           2 |     21 |                 14 |            34 % |              0.510 |                 0.740 | −0.230 |              13 of 21 |        0.879 |
| broadway-franklin     |      67 |           6 |     28 |                 12 |            28 % |              0.554 |                 0.827 | −0.273 |              26 of 28 |        1.950 |
| bush-powell           |      66 |           3 |     24 |                 21 |            34 % |              0.653 |                 1.113 | −0.461 |              20 of 24 |        0.903 |
| california-leon-baker |      51 |           5 |     20 |                 17 |            41 % |              0.998 |                 1.344 | −0.346 |              19 of 20 |        1.229 |
| columbus-north-point  |      78 |           2 |     24 |                 14 |            33 % |              0.660 |                 0.905 | −0.245 |              19 of 24 |        0.662 |
| embarcadero-bay       |     126 |           1 |     35 |                 28 |            29 % |              0.330 |                 0.454 | −0.125 |              22 of 35 |        0.696 |
| embarcadero-broadway  |     135 |           0 |     24 |                 21 |            28 % |              0.176 |                 0.244 | −0.068 |              13 of 24 |        0.577 |
| embarcadero-bryant    |     162 |           4 |     26 |                 20 |            32 % |              0.269 |                 0.319 | −0.050 |              14 of 26 |        1.678 |
| franklin-mcallister   |      96 |           1 |     32 |                 23 |            36 % |              0.396 |                 0.635 | −0.238 |              28 of 32 |        0.998 |
| fulton-divisadero     |      58 |           1 |     25 |                 17 |            32 % |              0.493 |                 0.815 | −0.322 |              20 of 25 |        1.123 |
| haight-divisadero     |      41 |           1 |     16 |                 10 |            37 % |              0.614 |                 0.846 | −0.233 |              15 of 16 |        1.797 |
| howard-6th            |      65 |           0 |     26 |                 15 |            28 % |              0.677 |                 0.982 | −0.305 |              18 of 26 |        0.877 |
| hyde-ofarrell         |      51 |           3 |     21 |                 16 |            30 % |              0.628 |                 1.028 | −0.400 |              18 of 21 |        0.671 |
| laguna-eddy           |      50 |           3 |     21 |                 18 |            32 % |              0.767 |                 1.355 | −0.589 |              18 of 21 |        0.861 |
| lombard-broderick     |      93 |           4 |     27 |                 22 |            33 % |              0.782 |                 1.360 | −0.578 |              18 of 27 |        0.861 |
| lombard-laguna        |      96 |           1 |     27 |                 14 |            26 % |              0.672 |                 1.158 | −0.485 |              18 of 27 |        0.641 |
| marina-broderick      |      90 |           3 |     17 |                 16 |            36 % |              0.441 |                 0.714 | −0.273 |              10 of 17 |        0.979 |
| pierce-haight         |      51 |           2 |     20 |                 17 |            36 % |              0.585 |                 0.707 | −0.123 |              17 of 20 |        1.061 |
| van-ness-sacramento   |     109 |           3 |     35 |                 24 |            29 % |              0.297 |                 0.459 | −0.162 |              22 of 35 |        0.644 |

Of the 1,938 regions, 61 were unsupported and 643 hold a kerb line. 566 had 20 or more held-back
road returns and were scored.

### Pooled over the 566 scored regions

| Measure (median over regions) | Regional | Constrained |
| ----------------------------- | -------: | ----------: |
| Median \|r\|                  |  0.065 m |     0.071 m |
| MAD of r                      |  0.061 m |     0.067 m |
| p95 \|r\|                     |  0.268 m |     0.402 m |

The constrained median is lower in 279 regions and its p95 in 84. In 149 of the scored regions
the constrained plane's tilt sits at 4.5° or more, at the edge of the 5° cone.

### Kerb candidates by range

All 23 sites, points inside regions.

| Range (m)            |  0–5 | 5–10 | 10–15 | 15–20 | 20–30 | 30–45 | 45 or more |
| -------------------- | ---: | ---: | ----: | ----: | ----: | ----: | ---------: |
| Share are candidates | 31 % | 34 % |  40 % |  44 % |  40 % |  27 % |        8 % |

### Against Gerardus's kerbs

At the five sites with a registered pose, the share of each set within 0.3 m (horizontal) of a
DataSF kerb line in the site bundle:

| Site                 |   All candidates | Boundary cells | Kept lines' inliers | Every background point |
| -------------------- | ---------------: | -------------: | ------------------: | ---------------------: |
| ashbury-downey       |  3.6 % of 24,101 | 4.8 % of 7,435 |        7.5 % of 746 |        3.2 % of 71,364 |
| columbus-broadway    |  7.0 % of 21,309 | 6.9 % of 8,171 |      7.7 % of 1,137 |        5.1 % of 70,256 |
| columbus-north-point | 14.6 % of 23,389 | 3.2 % of 6,662 |      0.4 % of 1,023 |        7.6 % of 70,677 |
| laguna-eddy          |  6.4 % of 22,986 | 6.2 % of 6,310 |        4.7 % of 812 |        4.8 % of 71,785 |
| lombard-laguna       | 10.9 % of 18,686 | 6.4 % of 6,372 |        0.6 % of 897 |        6.2 % of 70,803 |

At the 18 sites placed by hand, the kept lines' inliers lie within 0.3 m of a kerb at 0.5 % to
16.4 %. Every point's share there is 1.2 % to 16.3 %.

## Interpretation

1. **The verdict stands: the constrained plane is not proposed.** It fails clause 1 at both
   deciding sites and clause 2 at 413 regions. No reading of the rule is close.
2. **The lower quartile is not a road.** Both surfaces read the same held-back returns at or below
   each region's lower-quartile height. At these backgrounds that set is a stack of surfaces:
   road, gutter, sidewalk, steps, and returns below the road, for example 4 m below at
   haight-divisadero (−2, −5). Least squares lands between them and keeps the tail short. MSAC
   picks the densest 5 cm slab and leaves the rest a long way off. That is what the
   pooled table shows: equal medians, a p95 half again as large. A road prior needs a road
   selection before either fit, not a different fit on the same selection.
3. **The difference of normals at 0.3 m needs more than one return per cell.** A settled
   background has one point per ring and azimuth bin, about 3.5 cm apart along a ring at 10 m and
   0.5 to 1 m apart between rings. A 0.3 m neighbourhood is then a single arc, whose normal is
   free to turn about the arc. So a third or more of every site flags as a kerb, and the kept
   lines meet the DataSF kerbs no more often than any background point does. The kerb split
   needs a denser input, such as accumulated frames or the site model, or a scale set to the ring
   spacing.
4. **The cone is not the cause.** The levelled rerun turns each cloud by the operator pose's roll
   and pitch. It moves the tilt off the cone's edge in about half the regions (80 at 4.5° or more
   against 149), and still fails both clauses. The tilts it removes reach 10 to 13° at
   california-leon-baker and haight-divisadero, which is more than a tripod's lean. They are
   worth checking in the poses themselves before any frame is built on them.

## What it does not establish

- Whether any surface's clip keeps more lower-body returns (ground0002), or anything about
  tracks or safety metrics.
- Physical road height: residuals are against held-back background points, not a survey.
- PCL 1.15.1: Debian's 1.15.0 package ran, not the reviewed source build. The OQ-03 refined-mask
  contract held here at every region, but the build is not the reviewed one.
- Kerbs at 18 sites rest on hand-placed poses. Those poses are an operator's alignment, not a
  registration's.

## Provenance

| Item        | Value                                                                                                                                                                                                           |
| ----------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Rule        | spec reading `5cf1b0ce` on `exp/ground0006-constrained-plane`, from `main` `84492f9e`; the oracle and `bgasc -format pcd` in `bb75c99e`                                                                         |
| Backgrounds | `velocity-report-local` 0.5.1-pre50 at `84492f9e`, SHA-256 `cbf534c5…`; `config/tuning.defaults.json` SHA-256 `9ad7377f…`, the server's fingerprint `ab0c4035780d`; analysis mode, 0 to 300 s of each placement |
| Captures    | the 23 first-segment sites' whole placements, `sf-street-speeds/raw/lidar/` on the NAS; embarcadero-folsom made but not read                                                                                    |
| Oracle      | `tools/pcl-review/ground-oracle` (source SHA-256 `e55549f2…`), Debian `libpcl-dev` 1.15.0+dfsg-2, Eigen 3.4.0, g++ Release build; executable SHA-256 `1a045782…`                                                |
| Port check  | `l3grid.FitRegionalGroundSurface` on the same fit points (a scratch Go program, not committed): same regions and supports at all 23 sites, heights within 0.9 mm at region centres                              |
| Gerardus    | data copy from the nightly snapshot `20261102T122145Z`, accepted poses decoded from `registration_records`, kerbs from each site's current bundle; read only                                                    |
| Host        | arrow-worker, 4 vCPU, beside another experiment lane                                                                                                                                                            |
| Raw outputs | backgrounds, oracle CSVs and kerb lines under the run's scratch and `velocity-campaign/exp-20261103/laneB` on the results share; not in Git (D-36 for the kerb lines)                                           |

## Recommendations

1. **Do not port the constrained fit.** Change what is fitted before how. A road selection
   could take the Gerardus street polygons where a pose exists, or the cells moving tracks cross
   (ground0001's departure positions). Either would make a residual of "road returns" mean road.
   Then rerun this oracle's two fits on it.
2. **Run the kerb split on accumulated returns, not on the settled background.** The
   one-point-per-cell background cannot support a 0.3 m normal. Try the Gerardus site model's
   points or a few seconds of accumulated frames, keeping the 0.3 and 1.2 m scales, and repeat the
   comparison with the DataSF kerbs.
3. **Check the roll and pitch of the hand-placed poses.** Levelling by them shows 10 to 13° at two
   sites, which ground0003's frame would inherit.
4. **For Gerardus, a note rather than lines.** The kept kerb lines here carry no signal above
   chance, so offering them would add noise. The note for Gerardus is the measurement itself: a
   DoN at 0.3/1.2 m on one-return-per-cell backgrounds flags 26 to 41 % of points.
