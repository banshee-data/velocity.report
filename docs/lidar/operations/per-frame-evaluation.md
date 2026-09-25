# Per-frame evaluation

How to score two tracker arms per frame against reviewed, held-out annotation episodes, and what
the numbers do and do not mean.

- **Status:** Harness implemented and tested on synthetic packs; the held-out acceptance run against reviewed episodes is open
- **Layers:** L5 Tracks, L8 Analytics, offline analysis
- **Related:** [Annotation-scored tuning](annotation-scored-tuning.md), [Point annotation tool](point-annotation-tool.md), [gap analysis M5](../../../data/maths/paper-implementation-gap-analysis.md#consequences-m5), [State-estimation plan](../../plans/lidar-state-estimation-plan.md)
- **Code:** [perframeeval](../../../internal/lidar/perframeeval/doc.go), [l8analytics](../../../internal/lidar/l8analytics/perframe.go), [lidar-ground-truth-eval](../../../cmd/tools/lidar-ground-truth-eval/perframe.go)

## What it answers

The older ground-truth evaluator matches whole tracks on temporal overlap (gap M5). It cannot see
state estimates, it scores a fragmented track as an undetected one, and its reference is an earlier
run of the same tracker. This harness replaces all three: the reference is a person's reviewed
masks in an annotation pack, matching is per frame and spatial, and it reports the MOT16 family
(MOTA, MOTP, identity switches, fragmentation), HOTA with DetA and AssA, and IDF1 with identity
precision and recall. Two arms are scored on the same episodes and reported paired, with deltas.

It is the evidence a reassociation or association-cost change must pass before promotion: fewer
identity switches and fragmentations, higher AssA and IDF1, at no cost in DetA.

## The reference

The pack and its review sidecar become one reference point per mask. Every choice below moves the
numbers, so every result records it, and two arms scored under different choices are refused.

| Choice        | Default                                               | Alternative                                       |
| ------------- | ----------------------------------------------------- | ------------------------------------------------- |
| Review status | Reviewed mask of a reviewed object only               | `-include-proposed`: proposed masks scored too    |
| Position      | Centre of the mask's horizontal extent                | `-reference-position point_mean`: mean of returns |
| Partial masks | Scored                                                | `-ignore-partial-masks`: treated as uncertifiable |
| Road users    | car, van, truck, bus, motorcycle, pedestrian, cyclist | Fixed: the client's seven production labels       |

Partial masks are scored by default because the macOS client saves every mask as partial unless the
operator changes it; a reviewed partial mask still certifies the object was there. The point mean is
what the annotation-scored sweep used, and is kept so its numbers can be reproduced. Neither
position is the physical centre: both sit towards the faces the sensor saw.

A mask a person did not certify becomes an **ignore** point, MOT16's distractor rule: a hypothesis
matched to it is neither a true nor a false positive, and it is never missed.

| Mask                                                   | Becomes                        |
| ------------------------------------------------------ | ------------------------------ |
| Object or mask not reviewed (under the default policy) | Ignore: `unreviewed`           |
| Class not a road user (noise, ground, building, ...)   | Ignore: `not_road_user`        |
| Visibility fully occluded, outside view, or unknown    | Ignore: `visibility`           |
| Only uncertain returns, no certain members             | Ignore: `uncertain_membership` |
| Completeness never stated, or partial when ignored     | Ignore: `incomplete_mask`      |
| Object not scored by this episode                      | Ignore: `outside_episode`      |
| Object or mask rejected                                | Dropped                        |
| No returns at all                                      | Dropped                        |

The ignore rules the metrics apply are pinned case by case in
[ignore_rules_test.go](../../../internal/lidar/l8analytics/ignore_rules_test.go). Two go beyond
MOT16's devkit, deliberately: an ignored point never keeps a hypothesis by continuity (it could
take one from a scored object beside it), and a correspondence is carried across frames in which
its object is ignored (so an occlusion cannot hand the object to a stray). A real change of
hypothesis across an ignored stretch is still an identity switch.

## Held-out episodes: the split manifest

A split manifest freezes which objects belong to which partition of one pack, and which frame
intervals are scored as episodes. The annotation plan (§7) defines the partitions but no file; this
is the file, `velocity.report/annotation-split` schema version 1. Unknown fields are refused.

| Field                             | Meaning                                                                 |
| --------------------------------- | ----------------------------------------------------------------------- |
| `schema`, `schema_version`        | `velocity.report/annotation-split`, `1`                                 |
| `pack_digest`                     | The pack's `pack_digest`; the manifest is never applied to another pack |
| `dataset_id`                      | Optional; must match the pack's                                         |
| `sidecar_revision`                | Optional; pins the annotation revision the split was frozen against     |
| `note`                            | Free text                                                               |
| `splits[].name`, `.role`          | A partition and its role: `tuning` or `held_out`                        |
| `splits[].object_ids`             | Its objects; an object is in exactly one split                          |
| `episodes[].episode_id`, `.split` | An episode and the partition it belongs to                              |
| `episodes[].object_ids`           | The objects it scores; all must be in its own partition                 |
| `episodes[].frame_intervals`      | Inclusive `first_sample`/`last_sample` ranges, ordered and disjoint     |

Any other object with a mask in an episode's frames, from its own split or another, is ignored in
that episode. That is how a tuning object that shares frames with a held-out one stays out of the
held-out number. Split by object, not frame: every frame and revision of an object stays in the
partition it was frozen into.

## The arms

An arm is one of two sources, and both arms of a comparison must be the same kind.

| Source       | Selected by                                                                         | Track identity                                   |
| ------------ | ----------------------------------------------------------------------------------- | ------------------------------------------------ |
| Estimates    | `lidar_track_estimates` source, estimator, observation model, parameter hash, stage | `creation_sequence`, reproducible across replays |
| Analysis run | `lidar_run_tracks` run ID, positions from `lidar_track_observations`                | Renamed by first frame then position             |

Any estimate field left unset must be unambiguous in the database; if several versions match, the
command lists them and stops. Acceptance scores `final` estimates. Anything else, including every
analysis run (its positions are what the tracker believed at the time), is scored only with
`-a-declared-baseline` or `-b-declared-baseline`, and the output says so. **No `final` estimates
exist yet**: the pipeline writes `online` only, so every comparison run today is between declared
baselines. A source whose creation sequence restarts (a tracker reset mid-source) is refused.

## Matching

Each hypothesis point is moved onto the nearest reference frame within `-frame-tolerance-ms`
(default 10 ms). Two replays of one capture agree on frame times to about a millisecond, and the
matcher keys frames exactly. Points outside every episode interval are not the episode's business;
points inside one that land on no frame are counted, and more than `-max-unaligned-fraction`
(default 1%) of them refuses the arm.

The gate defaults to the D2 A/B's: one metre plus half the reference object's footprint diagonal
(`-gate footprint -gate-metres 1`). `-gate fixed -gate-metres 2` gives a fixed gate. HOTA's
similarity reaches zero at the same gate.

The assignment inside the matcher goes through a guard, `l8analytics/assign.go`, that transposes
to keep rows no more than columns and uses a finite penalty in place of the forbidden-pair sentinel;
a brute-force test pins it. It was written when the L5 Hungarian solver still padded with a 1e18
sentinel that destroyed the precision of real costs. The solver itself is now exact (gap analysis
H1), so the guard is redundant but still correct, and tracker association and the evaluator solve
the same problem the same way.

## Refusals

| Refused                                                            | Why                                                          |
| ------------------------------------------------------------------ | ------------------------------------------------------------ |
| Manifest for another pack, dataset or revision                     | Its held-out claim would describe different points or labels |
| An object in two splits; an episode scoring another split's object | The partition is not object-disjoint                         |
| A `tuning` split without `-allow-tuning-split`                     | Held-out scoring was asked for                               |
| An episode with no certifiable reference point                     | Every number would be a ratio over nothing                   |
| An episode object with no mask in its frames                       | The manifest was frozen against other labels                 |
| Two samples with one timestamp in an episode                       | Frames are matched by time                                   |
| A non-final arm without a declaration                              | Acceptance scores final estimates                            |
| An ambiguous estimate version; a restarted sequence                | An arm is one version, keyed reproducibly                    |
| Arms of different kinds                                            | Different write paths, not different estimators              |
| Differing policy, episodes, gate, tolerance or reference digest    | Not a comparison                                             |

## Output

`-json` writes the comparison (schema `velocity.report/perframe-comparison` version 1): the
reference identity with its SHA-256 digest, the gate and tolerance, per-episode paired summaries
with deltas (B minus A), the pooled totals, caveats, and each arm's full result including HOTA per
alpha and alignment counts. `-markdown` writes a reading copy. Pooled totals sum counts across
episodes and recompute ratios; HOTA pools as TrackEval does.

False positives include hypotheses on returns nobody labelled, so they are an upper bound unless
every road user in the episodes' frames has a mask. Deltas are sounder than either arm's count.

## Running it

Build once:

```bash
go build -o bin/lidar-ground-truth-eval ./cmd/tools/lidar-ground-truth-eval
```

### kirk0

The pack is `8e422582-…-20260922-043329` under the annotation directory
(`LIDAR_ANNOTATION_DIR`, default `../sensor_data/lidar/annotation-packs`). Find it, and read its
digest, current revision and review state:

```bash
PACK="$(ls -d "$LIDAR_ANNOTATION_DIR"/8e422582-*-20260922-043329)"
jq -r .pack_digest "$PACK/manifest.json"
jq -r .revision "$PACK/annotations.json"
jq -r '.objects[] | "\(.status) \(.class)"' "$PACK/annotations.json" | sort | uniq -c
```

Write a split manifest from the field table above, beside the pack rather than inside it, for
example `"$LIDAR_ANNOTATION_DIR"/splits/kirk0-split-v1.json`: pin `sidecar_revision`, put the
reviewed road users you will not tune against in a `held_out` split, and give each held-out
episode frame intervals inside the evidence runs' scored window.

First confirm the evaluator reproduces the D2 A/B's direction (state-estimation plan §21.1: the
medoid had fewer identity switches and fragmentations, and higher IDF1, than the OBB centre).
That comparison scored every labelled road user from 6 s, proposals included, at the point mean,
so it needs a manifest with one `tuning` split of all road users and one episode from sample 60 to
the end. The D2 evidence databases are on the LiDAR volume under
`velocity-campaign/obb-centre-ab-20260924/`:

```bash
bin/lidar-ground-truth-eval perframe \
  -pack "$PACK" -split-manifest "$LIDAR_ANNOTATION_DIR/splits/kirk0-d2-replication.json" \
  -split all-road-users -allow-tuning-split -include-proposed -reference-position point_mean \
  -a-label obb_centre -a-db "$D2_OBB_DB" -a-stage online -a-declared-baseline \
  -b-label medoid -b-db "$D2_MEDOID_DB" -b-stage online -b-declared-baseline \
  -json kirk0-d2-replication.json -markdown kirk0-d2-replication.md
```

Run it with no `-a-stage` first if unsure what a database holds: the refusal lists every estimate
version in it. Then score the held-out split for the change under test:

```bash
bin/lidar-ground-truth-eval perframe \
  -pack "$PACK" -split-manifest "$LIDAR_ANNOTATION_DIR/splits/kirk0-split-v1.json" -split held_out \
  -a-label baseline -a-db "$BASELINE_DB" -a-stage online -a-declared-baseline \
  -b-label candidate -b-db "$CANDIDATE_DB" -b-stage online -b-declared-baseline \
  -json kirk0-held-out.json -markdown kirk0-held-out.md
```

When both arms carry `final` estimates, drop the `-stage` and `-declared-baseline` flags: that is
the acceptance run.

### The three-site corpus

Evidence for each case comes from the corpus harness, one run per arm:

```bash
make evidence-run RUN=perframe-baseline CASE=columbus-broadway LIDAR_PCAP_DIR=/Volumes/lidar/lidar
make evidence-run RUN=perframe-candidate CASE=columbus-broadway LIDAR_PCAP_DIR=/Volumes/lidar/lidar \
  EVIDENCE_FLAGS="-experiment cascade"
```

Repeat for `marina-webster-beach` and `embarcadero-folsom`; the plan reserves Embarcadero as the
held-out site. Each database holds one source per case; pass `-a-source` and `-b-source` if a
database holds several. No reviewed pack exists for these sites yet. To make one, record a capture
of the case with points and cut a pack from it, then label and review it in the macOS client:

```bash
velocity lidar pcap-replay --pcap "$CAPTURE" --output "$VRLOG_DIR/columbus-annot" \
  --warmup-seconds 70 --start-seconds 70 --duration-seconds 20 --include-points
velocity lidar annotation-export --vrlog "$VRLOG_DIR/columbus-annot" \
  --output "$LIDAR_ANNOTATION_DIR/columbus-broadway-v1" --coverage foreground_only
```

Then freeze a split manifest for the pack and score each site as for kirk0, with the evidence
databases at `$LIDAR_EVIDENCE_DIR/perframe-baseline/observations/observations.db` and its
candidate twin.

## What remains

- **The held-out acceptance run itself.** It needs reviewed held-out episodes (kirk0 has four
  reviewed objects and 292 reviewed masks; the three corpus sites have no pack) and a frozen split
  manifest for each pack.
- **Final estimates.** Until the retrospective estimator writes `stage = final`, every run is a
  declared baseline and says so.
- **Coordinate frames.** The harness does not check that the pack and the estimates share a frame.
  kirk0's do (sensor origin, no site transform). For a site with a pose transform, check that the
  baseline arm's MOTP is well under a metre before reading anything else.
- **Episodes inside the scored window.** An evidence run writes no estimates during its warm-up, so
  an episode that starts before the scored window reads as missed objects.
- **HOTA ignore absorption** is decided per localisation threshold, not once before the sweep as
  TrackEval's preprocessing does. It affects only hypotheses close to an ignored point.
- **Results from before the exact solver.** Tracker runs recorded before `l5tracks.HungarianAssign`
  became exact (gap analysis H1) used a solver that could take a costlier assignment when clusters
  outnumbered tracks or a gated pair was forced. Score them only against runs from the same build.
