# Background cleanup does not yet justify another registration arm

- **Status:** OQ-09 answered; blanket MLS/SOR export cleanup dropped from Item 6
- **Related:** [Gerardus audit](pcl-gerardus-registration-gap-review.md), [candidate plan](../../plans/lidar-pcl-algorithm-candidates-plan.md#item-6-q3-recommendations-to-gerardus), [pose0001](gerardus-static-registration-2026-10.md), [pose0002](gerardus-background-window-2026-10.md)
- **PCL Git SHA:** `3346843ea6b97697a1577293aa9cdf236b5c7ffb`, 1.15.1
- **Gerardus Git SHA:** `a2c2ac10e6497c26d973488e38aba394c93ec345`, main verified during OQ-08
- **Local baseline:** velocity.report `e1db67ddc`, the OQ-08 checkpoint

## Disposition

Drop moving least squares (MLS) and blanket statistical outlier removal (SOR) as background-export
recommendations in this plan. The existing evidence does not establish that either improves
registration more than the track guard, or that either improves the guarded input at all. This is
a source-based scope decision, not a negative replay result: no cleanup arm was run.

The guard uses temporal track evidence to keep vehicle returns out of the background. SOR uses
spatial neighbour distances; dense unwanted vehicle surfaces need not be outliers, while valid
sparse structure can be. MLS projects measured points onto locally fitted surfaces, so it changes
the evidence whose fit is scored. Neither addresses the already identified definition of a
distinct registration basin. Retain the guarded export and Gerardus's current confidence/range
preparation. Reopen cleanup only with reviewed contamination that survives those stages and a
comparison that holds evaluation evidence fixed.

No new pose spec, tuning set, harness, default, or upload is introduced. l3bg0028's separate
border-label/retention question remains proposed; this decision neither adopts nor deletes it.
OQ-02 still blocks using the native ring/azimuth grid as a camera-compatible PCL image, and a veil
trait still does not prove a mixed return.

## What the earlier experiments establish

The [guard comparison](gerardus-static-registration-2026-10.md#the-track-guard-and-the-backgrounds-length)
in pose0001 produced six CANDIDATE sites with the guard and four without. Relative to guard-off,
guard-on gained lombard-laguna, van-ness-sacramento, franklin-mcallister, and embarcadero-bryant,
but lost marina-broderick and marina-webster-beach. At van-ness-sacramento the two poses differed
by 10.46 m and 96.10 degrees; at lombard-laguna separation changed from 1.21 without the guard to
1.26 with it. These are historical consistency-gate measurements, not surveyed accuracy or a
claim that the guard helps every site. The run accepted no poses.

[pose0002](gerardus-background-window-2026-10.md#against-the-rule) then found no shorter window
that passed its improvement rule against 300 s. At 60 s, two wrong-peak sites recovered, but two
controls were costed; 90 s recovered none; 180 s recovered one and costed a control. At fixed
poses, ground p95 could move across its gate. The negative verdict does not show that background
content is irrelevant: embarcadero-folsom's shorter backgrounds found R1's heading but still
failed facade p75. It motivates preserving the evaluation population, not selecting a cleaner
looking background after inspecting its registration score.

No MLS/SOR figures exist in either report. The guard comparison and window comparison cannot be
relabelled as a cleanup trial or used to rank unrun filters.

## Claim-level implementation evidence

| Claim                                                               | Reviewed source                                                                                                                                                                                                                                                                             | Consequence                                                                                                                                                                                                                    |
| ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| SOR rejects large mean neighbour distances                          | [SOR 68–123][sor] averages Euclidean distances to neighbours other than the query, then rejects above the population mean plus multiplier times sample standard deviation                                                                                                                   | It is a density statistic, not a mixed-return, moving-object, or physical-surface classifier                                                                                                                                   |
| Restricting query indices does not isolate the neighbour population | [SOR 49–89][sor-search] sets its search input to the complete cloud and searches each selected query in that cloud                                                                                                                                                                          | A fit/held split needs separate search input; setIndices alone still exposes held-out geometry to filter decisions                                                                                                             |
| SOR can be applied without the organised-camera adapter             | [SOR 49–57][sor-search] chooses a kd-tree for an unorganised cloud                                                                                                                                                                                                                          | Use original Cartesian XYZ with explicit indexing if reconsidered. OQ-02 blocks native-grid camera reuse, not every spatial filter                                                                                             |
| MLS with upsampling NONE still changes coordinates                  | [MLS 176–194][mls-none] fits a local surface and projects the query; [661–685][mls-project] returns a polynomial projection or the fitted-plane projection                                                                                                                                  | NONE means no upsampling, not no smoothing. Even a plane fallback can move a measured point                                                                                                                                    |
| MLS also changes which points survive                               | [MLS 306–350][mls-neighbours] processes a query only with at least three radius neighbours; [695–751][mls-fit] fits a covariance plane and, with enough points, a polynomial                                                                                                                | A sparse edge can disappear, or points near a crease can be projected onto a surface spanning both faces                                                                                                                       |
| The exported input already carries temporal support                 | [Snapshot export](../../../internal/lidar/l3grid/export_bg_snapshot.go) selects elevations; [ToASCPoints](../../../internal/lidar/l3grid/background_export.go) emits nonzero mean-range cells with TimesSeenCount. [Gerardus ASC parser 11–34][asc] reads that sixth column into confidence | A generic XYZ rewrite must not silently drop confidence. The fallback export without valid ring elevations projects to z = 0; such an input is invalid for a 3-D registration comparison and must be refused by the experiment |
| Gerardus already screens and weights input                          | [prepare_scan 547–567][prepare] cuts range, rejects confidence below 3 when enabled, keeps the first point per voxel, and weights by count/20 capped at 1                                                                                                                                   | This is not equivalent to SOR, but it is the existing control. Missing confidence defaults to 20, so losing that column changes the treatment                                                                                  |
| Filtering changes Gerardus's evaluation split                       | [prepare_scan 571–596][split] derives ground/structure from the prepared points and assigns every fifth retained point to held-out; [quality 887–930][quality] scores held-out matched residuals                                                                                            | Deletion, reordering, or moving points across voxels can change both fit and evaluation populations. An easier residual distribution cannot establish improved pose accuracy                                                   |

The exporter path used by pose0001 is `cmd/tools/bgasc` through ExportBgSnapshotToASC and
ToASCPoints. Its mean-range cells are accumulated background estimates, not individual raw
returns. An MLS projection is another inference on top of them. Preserve both their provenance
and the original coordinates rather than publishing a filtered cloud as if it were the same
measurement.

## Counterexamples

These are analytical examples derived from the source, not executed PCL fixtures or site runs.

1. **SOR removes valid isolated support.** Five collinear points at x = 0, 0.01, 0.02, 0.03,
   and 1 m, with two neighbours per query and multiplier 1, have mean neighbour distances
   0.015, 0.010, 0.010, 0.015, and 0.975 m. Their mean is 0.205 m and sample standard deviation
   about 0.43045 m, so the last point exceeds the 0.63545 m threshold. The same coordinates could
   describe a valid isolated return or an artefact. The filter cannot distinguish the labels.
2. **Dense contamination survives.** A vehicle surface with local spacing comparable to a nearby
   wall has comparable neighbour distances. SOR has no timestamp, identity, or motion input
   with which to reject it. Increasing its aggressiveness can remove the wall's sparse corners
   before the vehicle surface, losing the very normals that constrain along-street motion.
3. **Smoothing improves a changed target.** A radius neighbourhood straddling a kerb or two facade
   planes can fit a surface between them. MLS then moves queries toward that fit. Evaluating the
   transformed coordinates can lower residuals while the original measured geometry or physical
   pose is no better. Its non-upsampling mode does not prevent this.
4. **Deleting one point changes who is held out.** After an early deletion in the prepared list,
   later ordinals shift and the modulo-five split changes. Removed hard points also disappear
   from the residual distribution. Comparing each arm's own p95 without fixed evaluation support
   can reward deletion rather than registration; dropping TimesSeenCount adds another confound.

## Conditions for reconsideration

A new proposal needs reviewed residual/return evidence identifying the nuisance, including its
range, ring, surface, temporal support, and whether it is isolated. First show what the proposed
filter would remove or move, and what valid geometry it would lose, at a fixed pose. This is a
prerequisite for a new experiment, not a diagnostic campaign authorised by this checkpoint.

Freeze original source identities, an unmodified evaluation set, reference revision, and baseline
preparation before filtering. Let cleanup act only on the fitting input; prevent held-out points
from entering its neighbour search or surface fit. Score each fitted pose on the same original
evaluation set, with unmatched coverage and abstentions reported as well as residual percentiles.
The stock per-file preparation/split is insufficient for that comparison. Normal estimation is a
separate contract; MLS output normals are not calibrated merely because they exist.

A marginal-benefit question needs guarded input versus the same guarded input plus cleanup.
A claim of doing better than the guard needs matched guard-on/off × cleanup-on/off controls and
separate interaction reporting. Preserve pose0001's control-pose and residual protections,
reviewed Kirkham checks, and an independent held-out check before any adoption. Put all replay
parameters in checked-in experiment tuning files and filter parameters in a versioned harness
configuration before running; this checkpoint chooses no values and runs no arm.

## Validation and next checkpoint

PCL implementation bodies, Gerardus's reviewed preparation/scoring path, the local exporter, and
prior reports were inspected. Source paths and line ranges were checked. The OQ-08 full repository
test run covers the unchanged code; OQ-09 adds documentation only and passed final lint, links,
source-range checks, counterexample arithmetic, and the no-results guard. No cleanup comparison,
registration, physical validation, or performance measurement ran, and no raw outputs enter Git.

OQ-10 is next: inspect the classification scorecard and available labelled corpus, then record
minimum independent-object coverage by class, aspect, and range. Repeated frames from one object
must not be counted as independent class examples.

[sor]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/filters/include/pcl/filters/impl/statistical_outlier_removal.hpp#L68-L123
[sor-search]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/filters/include/pcl/filters/impl/statistical_outlier_removal.hpp#L49-L89
[mls-none]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/surface/include/pcl/surface/impl/mls.hpp#L176-L194
[mls-project]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/surface/include/pcl/surface/impl/mls.hpp#L661-L685
[mls-neighbours]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/surface/include/pcl/surface/impl/mls.hpp#L306-L350
[mls-fit]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/surface/include/pcl/surface/impl/mls.hpp#L695-L751
[asc]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/scan.rs#L11-L34
[prepare]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L547-L567
[split]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L571-L596
[quality]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L887-L930
