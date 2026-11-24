# W3 retained membership diagnostic

`lidar-ground-truth-eval fate` separates loss before clustering from returns
retained in multiple clusters. This is a diagnostic addition to the existing
return-fate scorer, not a new Kirk0 measurement or a production tuning change.

## Existing evidence and the remaining distinction

The [retention report](w3-retention-fragmentation-kirk0-2026-10.md) counted
cluster membership from retained coordinate samples, with capped samples only
providing estimates. The [return-fate report](w3-return-fate-kirk0-2026-10.md)
introduced exact packet/block/channel joins against foreground-complete
observation logs. The [Stage B report](w3-return-fate-stage-b-kirk0-2026-10.md)
showed why fewer fragments alone can mislead: an arm can lose returns instead of
joining them.

The existing `fate` command now adds `membership` counts to every measured
instant and each overall, class, range and object summary. Markdown prints the
same partition:

| Field                | Meaning                                                         |
| -------------------- | --------------------------------------------------------------- |
| `main_cluster`       | Reviewed returns in the largest reviewed fragment per instant   |
| `secondary_clusters` | Reviewed returns retained in all other accepted clusters        |
| `l3_background`      | Returns proven absent from the foreground using existing checks |
| `l4_rejected`        | Foreground returns rejected by the existing L4 fate categories  |
| `other_frame`        | Returns found in a neighbouring frame                           |
| `unresolved`         | Returns whose identity or absence cannot be resolved            |

These six counts sum to the measured reviewed returns. The first two sum to
`fates.clustered`. Main uses the object's reviewed membership, not the whole
cluster population. Ties have equal counts and do not require choosing an ID.
Summary counts sum the per-instant partitions; cluster IDs are not compared
across frames. Existing detailed fate counts and gap tables remain available.
L4 rejection includes height/surface filtering, voxel reduction, the input cap,
DBSCAN noise, shape rejection and an unknown rejection reason.

A fragmented instant losing its secondary fragment to background keeps its main
count but raises L3 loss. Joining that fragment instead raises the main count
without lowering retained membership. An L4 rejection raises the separate L4
count. Other-frame and unresolved counts must not be interpreted as losses.

## Reproduce and interpret

The fixture-backed command test exercises the existing `fate` entry point and
checks both Markdown and JSON. It creates synthetic packets, a reviewed pack and
an observation container; its fixture split named held-out is synthetic test
data, not a sealed corpus input.

```sh
go test ./internal/lidar/perframeeval/... ./cmd/tools/lidar-ground-truth-eval
```

For an authorised development pack and foreground-complete observation recording,
use the existing command with `-pack`, `-split-manifest`, `-split`,
`-observations`, `-allow-tuning-split` for the development split, and optionally
`-json`. Preserve the eligibility and provenance
requirements of the recording workflow. No new capture access or eligibility
bypass is introduced here.

No development recording was identified in this cloud workspace, and no sealed
held-out input was read. Fixture results establish the diagnostic's accounting,
not the current fate of Kirk0 truck 2's cab. Main is neither a tracker association
nor a cab label. The pack still contains only the recording's foreground; it
cannot establish the fate of vehicle returns absent from that reference.

The remaining A/B dependency is authorised development recordings from pinned
parameter sets with matching capture, split, reviewed masks and provenance.
Compare membership together with detailed fate, join coverage, not-measured
counts and reviewed near-merge evidence. Cab attribution requires the relevant
reviewed returns; a larger main fragment alone cannot certify correct joining.
Pi/live retention parity and CPU, memory and latency measurement remain separate
hardware dependencies. This cloud diagnostic neither requires Hydra nor closes
those gates, W3 acceptance or the production-default decision.
