# Uncertainty manoeuvre and frozen-table scoring

- **Status:** Row 6 and row 7 execution implemented; scientific acceptance remains open
- **Related:** [Predeclared G-UNC-1 criteria](adaptive-uncertainty-criteria.md),
  [per-frame reference contract](per-frame-evaluation.md)

The offline evaluator now executes the two checks that were missing from the uncertainty
workflow. Neither command issues an overall pass or promotes a calibration table.

## Reviewed manoeuvres: row 6

`manoeuvres` joins the existing reviewed reference to `uncertainty_samples.jsonl`. It loads
the annotation pack, selected partition, pinned membership revision and frozen-split checks
through the same `LoadReference` path as the per-frame evaluator. It does not create a second
position-label store or move clips between tuning and held-out partitions. A tuning partition
requires `-allow-tuning-split`; the report identifies tuning and unfrozen evidence explicitly.

The event manifest has `schema_version: 1`, the existing comparison's `reference.digest` as
`reference_digest`, and an `events` array. Each event has:

| Field                     | Meaning                                                     |
| ------------------------- | ----------------------------------------------------------- |
| `id`                      | Unique event identity                                       |
| `episode_id`, `object_id` | Identities in the selected reference                        |
| `kind`                    | `brake`, `turn` or `lane_change`                            |
| `start_ns`, `end_ns`      | Inclusive capture-time interval, decimal nanosecond strings |
| `status`, `reviewer`      | `reviewed` and the responsible review identity              |

A human must review the events. Missing, proposed or TBD labels must not be converted into
reviewed intervals by the runner. An empty event array reports insufficient evidence. The
manifest pins the complete reference digest, so changing the pack, membership revision,
partition, selected episodes or reference policy requires a newly bound manifest. Intervals
must lie inside one selected episode interval; they cannot straddle a partition gap.

Matching uses exact capture-frame nanoseconds and the predeclared spatial gate, 1 m plus half
the reference footprint diagonal. It makes no time interpolation or nearest-object choice.
Exactly one eligible pairing must match the labelled object, and that pairing must not also
match another reference object. Ignored references can make a pairing ambiguous but cannot
enter the denominator. Repeated pairings at identical coordinates remain ambiguous: the sample
format carries no track identity with which to disambiguate them. Neither `assigned` nor
`gated` influences matching. Overlapping event intervals count an object/frame once.

The report publishes labelled, eligible, gated, missing and ambiguous counts, the conditional
gated fraction, reference identity, and SHA-256 of the event and sample files. Missing or
ambiguous samples prevent a `within` result. With complete coverage, the fraction must be
strictly below 1%; exactly 1% is outside. Zero eligible frames has a null fraction and is
insufficient. These coverage refusals are conservative execution guards, not changed acceptance
thresholds. Capture, arm and calibration provenance still need checking against the replay
record: hashing sample bytes alone does not establish their source.

```bash
make build-lidar-eval
bin/lidar-ground-truth-eval manoeuvres \
  -pack /scratch/pack -split-manifest /scratch/frozen-split.json \
  -split decision -events /scratch/reviewed-manoeuvres.json \
  -samples /scratch/candidate/uncertainty_samples.jsonl \
  > /scratch/manoeuvre-score.json
```

Use the same `-ignore-objects` policy as the comparison whose digest the manifest pins.
Exit zero means a report was produced; read `row_6_status` for the individual row's result.

## Frozen-table synthetic recovery: row 7

```bash
bin/lidar-ground-truth-eval uncertainty-recovery \
  -calibration /scratch/frozen/uncertainty_calibration.json \
  > /scratch/recovery-score.json
```

The canonical calibration parser checks the content address, bin layout, sources and positive
finite coefficients. The command runs the predeclared seed-13 scene: 45 frames at 10 Hz, an
8 m/s body starting at (-30 m, 15 m), radial/tangential noise of 0.03/0.08 m, and frames 25–29
hidden. Draws continue during occlusion. The result must recover the original confirmed
creation sequence within 0.5 m of truth in one or two returning frames. Rebirth does not count;
no recovery is reported as -1. The report names the table and fixed scene and publishes only
`row_7_status`, never an overall verdict.

## Validation and work still needed

Contract tests cover reviewed/frozen binding, stale manifests, explicit tuning opt-in, missing
gate flags, empty and duplicate samples, spatial ambiguity, event overlap, coverage and the
strict 1% boundary. Recovery tests load a valid content-addressed table and reject invalid
coefficients, unnamed tables and reborn identities. Both commands have CLI error/output tests.

No real reviewed manoeuvre manifest or decision-partition calibration has been supplied by
this implementation. The tests' event reviews and tables are synthetic fixtures. Fit and freeze
the table on the declared fitting partition, review and pin event labels, then execute rows
1–7 and the independent held-out identity guard once. The existing kirk0 goodness-of-fit
failures remain evidence against promotion; these tools do not reverse them.
