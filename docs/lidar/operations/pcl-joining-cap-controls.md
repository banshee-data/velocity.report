# PCL joining oracle: separate sampling from connectivity

- **Status:** OQ-05 answered by a pre-run design amendment; no new campaign executed
- **Spec:** [l4clus0005](../../../data/experiments/l4clus/l4clus0005.md)
- **Evidence:** [l4clus0004 report](l4-subsampled-density-2026-10.md)
- **Plan:** [Joining rule](../../plans/lidar-pcl-algorithm-candidates-plan.md#item-3-q1-the-joining-rule-l4clus0005)

All thirteen review questions and the final branch pass are complete. Source findings and
validation below describe this report's recorded checkpoint; subsequent full-suite validation
and remaining implementation prerequisites are recorded in the
[current handover](pcl-astera-handover.md#resume-point). Closing a review question does not
complete or authorise its proposed campaign.

## Decision and its evidence

Add the uncapped control and both uncapped graph arms to l4clus0005. Keep the capped arms.
The existing corpus result is a reason to separate the factors, not to conclude that removing
samples cannot change connectivity. The experiment remains proposed, with no default change.

l4clus0004 found that 10 of 23 sites had at least 1 % of scored frames over the 8,000-point cap;
the maximum share was 7.85 % and retained fractions fell to 0.26. Raising the cap to 100,000
changed confirmed vehicle counts beyond the chosen band at five capped sites, in both
directions, with a median change of zero. A count gain can be fragmentation and a loss can be
a resolved duplicate: those unlabelled counts cannot determine graph quality. The report's
2.3–2.7× p99 increases at seven capped sites are host-specific costs, not Pi measurements.
Its kirk0 labelled step did not run because the density arm had already failed the corpus rule.

The report also observed differences at sites with no capped scored frames and inferred that
warm-up clustering and track-guard feedback could explain them. That causal explanation was
not measured. Independent end-to-end replays therefore cannot establish that a changed path
was caused solely by added samples. The frozen-stream comparison below addresses that problem.
Earlier kirk0 results under CMR do not replace this controlled comparison under current defaults.

## Six arms, two factors

| Cap treatment              | DBSCAN control | XY radius graph    | XY graph with height condition |
| -------------------------- | -------------- | ------------------ | ------------------------------ |
| Defaults, 8,000            | a0             | radius-graph       | conditional                    |
| Existing 100,000-point set | nocap          | radius-graph-nocap | conditional-nocap              |

All low-cap arms use [tuning.defaults.json](../../../config/tuning.defaults.json); all high-cap
arms use [l4-cap100k.json](../../../config/experiments/l4-cap100k.json). Checked at this checkpoint,
the sole differing key is `l4.dbscan_xy_v1.foreground_max_input_points`, 8,000 versus 100,000.
No new JSON is needed. The six-arm replay/score estimate is 36 minutes before exclusions,
versus 18 minutes for three arms; the disk reservation is increased from 6 to 12 GB. These are
planning estimates, not measured runtime or output size. The [config register](../../../config/experiments/README.md) records reuse.

- Defaults SHA-256: `aefcc10ca9ed1e3cf796a800e9b3d3b51e4cbe3957237d8b10f8be7654446f15`.
- High-cap SHA-256: `990b50c403bfcba8573a55d0a2c9285eb911111b87a4c76861d24d9133eeb73b`.

These are the design checkpoint's fingerprints, not permission to silently run after config
drift. Recheck the single-key difference and record actual hashes in the future run report.
The finite 100,000 limit counts as uncapped only if every input frame, including warm-up, fits.
If it does not, stop and record a revised checked-in set before running; do not type an ad hoc cap.

## Frozen inputs answer the algorithm question

Take one baseline replay's stream after height selection and voxelisation, before subsampling.
For each frame retain the complete pre-cap sample set U and the actual default DBSCAN subset C,
including immutable source ordinals and aggregate membership. Verify C is a subset of U and that
it matches the points actually clustered. Do not reconstruct C with another random draw.
Use the same coordinates, normal-free height intervals, radius, filters, and labels in all six
oracle treatments. Do not rerun background estimation separately for this comparison.

The implementation already exposes post-cap points through
[DBSCANTrace.Processed](../../../internal/lidar/l4perception/cluster.go), and the
[pipeline](../../../internal/lidar/pipeline/tracking_pipeline.go) places voxelisation before the
DBSCAN cap. The [observation tap](../../../internal/lidar/pipeline/observation_frame_tap.go)
records stage lineage. These are reuse candidates, not a claim that a six-arm exporter exists.
The implement step must verify complete pre-cap/aggregate membership and reject lost identities.
The OQ-02 organised raw-frame exporter does not supply this L4 contract.

Freeze reviewed object instants fragmented under a0 as the oracle denominator, at 20 to 50 m.
A joined instant places all reviewed members of its original fragments in one raw component.
Use that same denominator and those same members for both cap treatments. Record numerator,
denominator, and cap exposure per band, plus cross-object components, noise, rejection, and
newly recovered reviewed samples. Report connectivity before shape filters separately from
accepted components: filtering is not evidence that a path never existed.

This allows three separate readings:

- Compare graphs with DBSCAN on C, or on U, to isolate the joining rule for a fixed point set.
- Compare the same algorithm on C and U to measure the sampling effect on that fixed upstream stream.
- Compare those two graph-versus-DBSCAN contrasts to identify an interaction; do not attribute the entire high-cap graph result to connectivity.

A simple counterexample explains why the new control is necessary: three collinear samples at
x = 0, 0.6, and 1.2 m form a connected graph at radius 0.8 m; removing the middle sample leaves
two components. This is a mathematical construction, not a measured kirk0 result. At unchanged
z it also passes the height condition. Adding more points can likewise create a bridge between
different objects, which is why the merge checks remain mandatory.

## Stop and acceptance comparisons

Each of the four graph/cap combinations has the original one-third join gate on that fixed
oracle denominator. Implement and replay only combinations passing it, with both DBSCAN controls.
If all four fail, stop this fixed-radius graph experiment and record the scope of the negative
result. A zero denominator is not evaluable. A capped-only failure cannot stop the uncapped
oracle or prove that all L4 rules fail. If no relevant scored frames are capped, the graph
comparison may still proceed, but the sampling interaction has no exposure on that window.

The original five-percentage-point fragmentation rule and merge/spurious-cluster/HOTA/IDF1
bounds still apply against a0. An uncapped graph must additionally pass those same thresholds
against nocap DBSCAN. This prevents a sampling benefit alone qualifying a new joining rule,
and prevents a gain against a poor uncapped control concealing a regression against a0.
The spec preserves the distinct oracle 20–50 m and replay 20–35 m ranges.

Use identical capture, scoring window, warm-up duration, label pack, and fixed parameters in the
later end-to-end replays. Record cap counts for warm-up as well as scoring. Each arm's tracks
and background guard may then change its future foreground; those results measure the total
pipeline effect. They must not be presented as the frozen-stream sampling contrast. Timing is
reported as cost; no new numerical performance gate is invented here.

## Readiness and limits

OQ-05 closes the control-design question. It does not close l4clus0005, demonstrate a gain,
implement replay selectors, or authorise an unattended campaign. The planned `pcl-join-oracle`
from the [tooling runbook](pcl-oracle-tooling.md) still needs the six partitions and ordinal tests.
The new source-loss fixture, shared-XY height fixture, bridge case, shuffled frozen membership,
and below-cap equality case must pass before replays. Keep raw exports and metrics local; commit
the human-readable future report with per-arm/per-case results, provenance, and limitations.

## Checkpoint validation

The config comparison found exactly the documented cap change, and `go test ./internal/config -run Experiment` passed. The experiment validator accepts all six arms and the regenerated
index; the documentation link checks and repository lint pass. This validates the design record,
not the absent graph implementations. The two existing PCL diagnostics rebuilt and their
segmentation CTest passed during OQ-04.

Current main was merged before this checkpoint, preserving both development-log histories.
The touched upstream Go packages passed targeted PCAP-enabled checks. The replayeval package
initially failed because this worktree held a Git LFS pointer rather than kirk0 data; rerunning
with the local capture matching its recorded SHA-256 passed. The original pointer was restored,
and no capture or raw test output enters this change. The final PR records the full test gate.
