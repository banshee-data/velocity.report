# Facet F0 on kirk0: near-face attribution from the reviewed pack

A protocol for one working session, six to seven hours on the Mac, that gives the facet
registration experiment its first deliverable from the one well-annotated capture: what the
present body estimate gets wrong on the faces a person labelled, which part of that error the
extent prior owns, how much of it arrives with a face transition, and where on this capture the
sparse tail begins. It runs existing tools only. Nothing in it tunes an estimator.

- **Status:** Protocol, not yet run; written for a separate runner
- **Layers:** L4 geometric evidence, L5 estimation, L8 analytics, offline evaluation
- **Related:** [Facet registration plan](../../plans/lidar-facet-registration-experiment-plan.md) (F0 and the attribution gate), [facet maths review](../../../data/maths/proposals/20261008-facet-registration-maths-review.md), [near-edge tracked state](../../plans/lidar-near-edge-tracked-state-plan.md) (F9), [near-face evaluation](near-face-evaluation.md), [per-frame evaluation](per-frame-evaluation.md), [October campaign](near-edge-campaign-2026-10.md)
- **Code:** [lidar-near-face-eval](../../../cmd/tools/lidar-near-face-eval/main.go), [lidar-ground-truth-eval](../../../cmd/tools/lidar-ground-truth-eval/main.go), [lidar-annotation-split-draft](../../../cmd/tools/lidar-annotation-split-draft/main.go), [lidar-state-estimation-baseline](../../../cmd/tools/lidar-state-estimation-baseline/main.go)

## 1. Why this experiment, and why now

The facet plan's first deliverable is an error-budget report that says whether the present
headway limit is longitudinal endpoint geometry, speed, identity, visibility, or missing evidence.
Its revised Section 4 adds an attribution gate: arm D is funded only if enough of the along-path
error is edge localisation rather than the extent prior. The maths review predicts the prior
dominates, and that patch density cannot move it. That prediction has not been tested on data.

The near-face evaluator is built and tested on synthetic packs, and its first run on a reviewed
pack is still open (near-edge plan, checklist item F9). kirk0's pack holds 20 reviewed road-user
objects and about 3,400 scored masks, 19 objects and about 3,100 masks after the first solid-body
row at 6.2 s. The evaluator measures exactly the three quantities the attribution gate needs where
a face is visible: the end-normal residual (the near bumper's placement), the side-normal residual
(the near side's placement), and the end tangent (the error a rank-one fix leaves along the end
face). It stratifies by whether the extents are evidence or a class prior, which is the
prior-versus-localisation split the gate asks for.

So this is the cheapest experiment that can change the plan: four short replays of one capture, two
existing evaluators, and an afternoon of stratification. It also gives the October campaign the
one thing it could not: a label-backed reading of whether the lateral-face-entry tail is an
accuracy error or a smoothness artefact.

## 2. What this run must not do

- **Do not tune on kirk0.** It is the capture the shadow work was developed on; its split role is
  `tuning` and the reports must say so. Every number here is a tuning score.
- **Do not change an estimator or a default.** Use the arms as they exist, from one frozen build.
- **Do not commit raw outputs.** Evidence databases, JSON reports, logs, and the analysis script's
  outputs stay under a local `results/` directory, which the index guard rejects. The written
  report under `docs/` carries the summary tables and the provenance identifiers.
- **Do not read a trail's smoothness as accuracy.** The residuals here are against labelled
  returns, which is the point; the five-point residual is not used.

## 3. Pre-flight, about fifteen minutes

Work from a clean checkout of `main` merged with the branch that carries this protocol. Record the
commit. Then:

1. **Capture.** `internal/lidar/perf/pcap/kirk0.pcapng` is a git-lfs object. Confirm it is the
   real file, not a pointer, and that its digest is
   `ae16ca0125f84113f59170337b1f7c9f42dce99f8579c69a0c2246c3b733c0b6`, the value the coverage
   survey recorded. If it is a pointer, run `git lfs pull` first.
2. **Pack.** The pack is
   `/Volumes/lidar/offload/sensor_data/lidar/annotation-packs/ad8b9438-c1d4-40f0-bd0a-f1852c984569-20260929-184718.156560000`.
   List it. Expect `manifest.json`, `samples.json`, `points.bin`, and `annotations.json`. Record
   from the manifest: `dataset_id`, the pack digest, `coverage`, the coordinate contract's
   `transform_version` (empty means the sensor frame, origin at the sensor), and the source's
   `pcap_basename`. Record the sidecar's revision and its object count by class and review
   status. Note whether `physical-references.json`, `feature-proposals.pb`, and
   `backgrounds.json` are present: the first enables Step 6, the second is not used here, and the
   third is the input a later extent-end labeller would read.
3. **Split.** If `$LIDAR_ANNOTATION_DIR/splits/kirk0-tuning.json` or a frozen kirk0 split already
   exists, use it and record its digest. Otherwise Step 2 drafts one.
4. **Build.** One binary per tool, stamped, from the recorded commit:

```bash
export W=$HOME/vr-scratch/facet-f0-kirk0   # local; never inside the checkout
export PACK=/Volumes/lidar/offload/sensor_data/lidar/annotation-packs/ad8b9438-c1d4-40f0-bd0a-f1852c984569-20260929-184718.156560000
export LIDAR_ANNOTATION_DIR=${LIDAR_ANNOTATION_DIR:-$HOME/vr-annotations}
mkdir -p "$W/bin" "$LIDAR_ANNOTATION_DIR/splits"
STAMP="-X github.com/banshee-data/velocity.report/internal/version.GitSHA=$(git rev-parse HEAD)"
go build -tags=pcap -ldflags "$STAMP" -o "$W/bin/baseline" ./cmd/tools/lidar-state-estimation-baseline
go build -ldflags "$STAMP" -o "$W/bin/split-draft" ./cmd/tools/lidar-annotation-split-draft
go build -ldflags "$STAMP" -o "$W/bin/near-face" ./cmd/tools/lidar-near-face-eval
go build -ldflags "$STAMP" -o "$W/bin/gt-eval" ./cmd/tools/lidar-ground-truth-eval
shasum -a 256 "$W"/bin/* | tee "$W/bin/sha256.txt"
```

If `go build -tags=pcap` fails for want of libpcap, `brew install libpcap`.

## 4. Arms

All four arms share `BASE`, the shadow configuration the October campaign froze. The control is
the shadow; the other three feed the near-edge measurement into the tracked state. B0, the
production tracker, writes no solid bodies and cannot be face-scored; it is not an arm here.

| Arm        | `-experiment`                                      | What it tests on labels                                                       |
| ---------- | -------------------------------------------------- | ----------------------------------------------------------------------------- |
| `control`  | `$BASE`                                            | The shadow body, T1 with T3 and full members: the facet plan's arm A baseline |
| `track`    | `$BASE,near_edge_track`                            | A2, the tracked near-edge candidate the campaign kept default-off             |
| `track_t5` | `$BASE,near_edge_track,solid_body_rank_one_medoid` | The rank-one medoid remedy: does it pull centres toward the sensor?           |
| `track_a1` | `$BASE,near_edge_track,near_edge_track_a1`         | Medoid gating: A2's association value, on identity and faces                  |

`BASE=solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members`.

## 5. Steps

### Step 1: replay the four arms, keeping evidence, about forty minutes

```bash
BASE=solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members
for arm in "control:$BASE" "track:$BASE,near_edge_track" \
  "track_t5:$BASE,near_edge_track,solid_body_rank_one_medoid" \
  "track_a1:$BASE,near_edge_track,near_edge_track_a1"; do
  name=${arm%%:*}
  mkdir -p "$W/$name"
  "$W/bin/baseline" \
    -corpus cmd/tools/lidar-state-estimation-baseline/testdata/kirk0-corpus.json \
    -index cmd/tools/lidar-state-estimation-baseline/testdata/kirk0-index.json \
    -pcap-root internal/lidar/perf -pcap-subdir pcap -warmup 20 -case kirk0 \
    -experiment "${arm#*:}" -sample-points 256 \
    -source-manifest "$W/$name/source-manifest.json" -out "$W/$name/out" \
    -evidence-dir "$W/$name/evidence" -evidence-per-case \
    > "$W/$name/run.log" 2>&1 || { echo "$name failed: see $W/$name/run.log"; break; }
done
```

The tool's determinism repeat stays on. Confirm in each `out/phase0-summary.json` that the repeat
was byte-equal and that the default replay matched the committed kirk0 baseline. An arm whose
repeat differs is not scored; record it and stop.

### Step 2: draft the split, five minutes

Skip if a split exists. Otherwise, from the control arm's evidence:

```bash
"$W/bin/split-draft" -pack "$PACK" \
  -from-evidence "$W/control/evidence/kirk0.db" \
  -output "$LIDAR_ANNOTATION_DIR/splits/kirk0-tuning.json"
```

Record the objects it kept and the ones it left out with their reasons. The window starts at the
first solid-body row, 6.2 s into the capture with `-warmup 20`.

### Step 3: score the faces, ten minutes

```bash
SPLIT=$LIDAR_ANNOTATION_DIR/splits/kirk0-tuning.json
"$W/bin/near-face" -pack "$PACK" -split-manifest "$SPLIT" -split tuning -allow-tuning-split \
  -arm control="$W/control/evidence/kirk0.db" -arm track="$W/track/evidence/kirk0.db" \
  -arm track_t5="$W/track_t5/evidence/kirk0.db" -arm track_a1="$W/track_a1/evidence/kirk0.db" \
  -instants -json "$W/faces.json" -markdown "$W/faces.md"
```

The control is the first arm, so the paired differences are against it. The sensor is at the
origin in this pack's frame, which is the flags' default; the per-arm centre-to-footprint distance
in the accounting should be about a metre. A median of several metres means the arm and the pack
are not in one frame, and the run stops there.

### Step 4: score identity on the same labels, ten minutes

```bash
for arm in track track_t5 track_a1; do
  "$W/bin/gt-eval" perframe -pack "$PACK" -split-manifest "$SPLIT" -split tuning -allow-tuning-split \
    -a-label control -a-db "$W/control/evidence/kirk0.db" -a-stage online -a-declared-baseline \
    -b-label "$arm" -b-db "$W/$arm/evidence/kirk0.db" -b-stage online -b-declared-baseline \
    -json "$W/identity-$arm.json" -markdown "$W/identity-$arm.md"
done
```

### Step 5: stratify and join, about two hours

Write one analysis script, kept beside the outputs and never committed, and record its SHA-256 in
the report. It reads `faces.json` with instants, the pack's `samples.json` for sample timestamps,
and each arm's `lidar_track_solid_bodies` table. Every quantity below is defined so the report
can be reproduced from the same inputs.

**Per instant.** From `faces.json`, each scored instant carries `sample_id`, `object_id`,
`track_key`, `range_m`, `returns`, `extent_evidence`, `sensor_axial_m`, `sensor_lateral_m`, and the
three residuals where scored. Derive the folded aspect in degrees, zero end-on and ninety
broadside:

```text
aspect_deg = degrees(atan2(|sensor_lateral_m|, |sensor_axial_m|))
```

**Strata.** Report every residual, per arm, in each of these, with `n`, mean, median, mean
absolute, p95 absolute, and the share over 10 cm:

| Stratum     | Bins                                                                             |
| ----------- | -------------------------------------------------------------------------------- |
| Extent      | `prior`, `evidence` (the tool's own)                                             |
| Range       | under 20 m, 20 to 50 m, 50 to 80 m, 80 m and beyond                              |
| Returns     | under 30, 30 to 59, 60 to 119, 120 and over                                      |
| Aspect      | end-on under 20 degrees, oblique 20 to 69 degrees, broadside 70 degrees and over |
| Sparse tail | any of: range 50 m or more, returns under 60, aspect under 20 degrees            |

**Along-path exposure.** Per range bin, the share of matched instants whose end-normal residual
was scored. Its complement is the share of the pass in which no end face is visible and the
along-path direction is unobserved by faces. Report the unscored reasons beside it.

**Paired differences.** For each arm against the control, on the instants both scored, the mean
and p95 absolute difference per residual and per stratum, with a 95 percent interval from
resampling whole objects (2,000 replicates, seed 20261008). Nineteen objects is a small number of
units; say so, and do not quote a frame-level interval.

**Face-entry join.** In each arm's evidence database, order `lidar_track_solid_bodies` by
`track_id` and `frame_unix_nanos`, using the single estimator, observation model, and parameter
hash the evaluator scored. For each row derive `entered_lateral` (the row's `visible_faces` gains
`left` or `right` relative to the previous row of the same track), `entered_any`, and
`frames_since_change`. Join instants to rows through `track_key`, which is the creation sequence
(`seq-NNNNNN` against `creation_sequence`), and the sample's timestamp against `frame_unix_nanos`
within the report's `frame_tolerance_ns`. Compare the three residuals in three groups: the entry
frame and the two after a lateral face enters; the same for any face change; and stable frames,
at least three since the last change. Report `measurement_rank`, `fallback_reason`, and
`inferred_extent` counts per group so a difference can be read against what the fix used.

**Return budget against the synthetic model.** For every scored instant, compare `returns` with the
synthetic prediction at the same range and aspect, using the deployed ring table and the
mount height recorded for kirk0's deployment (if none is recorded, use 3 m and say so):

```text
N_pred(r, a) = rings_on_body(r) * (L sin a + W cos a) / (r * 0.2 deg in radians)
```

with `L = 4.5 m` and `W = 1.8 m` for cars, and the ring count from the elevations that strike a
1.5 m body at ground range `r`. Report the ratio `returns / N_pred` by range bin as median and
interquartile range, for cars only. A ratio well under one at all ranges is expected from the
height band and dropouts; what matters is whether it is flat in range, which is what the
synthetic generator assumes.

### Step 6: physical references, only if the pack has them, about twenty minutes

If `physical-references.json` exists and its review state admits scoring, run the per-frame
evaluator once more with `-physical-reference` and `-physical-bundle-dir "$W/physical-bundle"`,
control against `track`. This is the only step that measures the along-path error when no end face
is visible, so it is the one that could close the attribution gate rather than bound it. Record
the physical revision and the expected-instant count the tool prints. If the file is absent or
unreviewed, say so in the report and leave the gate bounded, not closed.

### Step 7: write the report, about ninety minutes

Write `docs/lidar/operations/facet-f0-attribution-kirk0-report.md` with the question, the arms,
the split and pack identities, the tables from Step 5, the identity headline from Step 4, the
readings of Section 6 below, the limitations of Section 8, and the frozen provenance: commit,
binary digests, pack digest, sidecar revision, split digest, capture digest, and the analysis
script's digest. Link it from this protocol's status line, from the facet plan's Section 7, from
the near-edge plan's F9 checklist item, and from the devlog. Raw outputs stay in `$W`.

## 6. Predeclared readings

These are readings for the facet plan, not promotion gates. Choose nothing after seeing the
numbers that is not listed here.

| Question                                 | Measure                                                                  | Reading                                                                                                                                                                                                                                  |
| ---------------------------------------- | ------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Does the extent prior own the end error? | Control's end-normal mean absolute in `prior` against `evidence`         | Twice or more: the prior dominates where ends are visible, the review's prediction holds, and the attribution gate routes effort to extent memory and interval output. Under 1.25: edge localisation is in play and arm D keeps its case |
| Is the end tangent a real error?         | Control's end-tangent p95 absolute, all instants                         | Over 0.3 m: the error a rank-one fix leaves is material and a second face or physical edge is worth paying for. Under 0.15 m: it is not the limiting term                                                                                |
| Is the lateral-entry tail accuracy?      | End tangent and side normal, entry group against stable group, per arm   | Entry group worse by more than its interval width: the campaign's tail is an accuracy error and the T remedies have a target. Equal: the five-point tail was smoothness, and label-free residuals should stop deciding it                |
| Where does the sparse tail begin here?   | Along-path exposure and `min_returns` unscored share by range bin        | The first bin in which exposure falls under half, or the unscored share rises over a fifth, is kirk0's tail onset; compare with the review's 60 to 80 m prediction                                                                       |
| Is the synthetic sampling model usable?  | `returns / N_pred` median by range bin                                   | Flat within a factor of 1.5 across bins: E1 may use the generator for transfer claims. Falling with range: the generator needs a dropout model before E1 trusts it at range                                                              |
| Does T5 pull centres toward the sensor?  | `track_t5` against `track`: side-normal and end-normal mean, with sign   | A positive shift beyond the interval: T5 trades the five-point tail for a bias toward the sensor, as the near-edge plan feared. No shift: T5 is harmless on labels and still not worth keeping                                           |
| Does A2's gate protect identity?         | Identity deltas, `track_a1` against `track`                              | More switches or fragmentations under A1 confirm the campaign's reading on labels; fewer would reopen it                                                                                                                                 |
| Does the tracked arm beat the shadow?    | `track` against `control`: paired end-normal and side-normal, all strata | A paired improvement with an interval excluding zero is the first label-backed evidence for A2; a regression anywhere in the sparse-tail stratum is reported beside it, not netted off                                                   |

## 7. Time budget

| Step                       | Minutes | Note                                                                    |
| -------------------------- | ------: | ----------------------------------------------------------------------- |
| Pre-flight and build       |      20 | Longer if libpcap or lfs must be installed                              |
| Four replays with repeats  |      40 | kirk0 is 83 s of capture; each arm is minutes, not the campaign's hours |
| Split draft, two scorers   |      25 |                                                                         |
| Analysis script and tables |     120 | The face-entry join is most of it                                       |
| Physical scoring, if any   |      20 | Conditional                                                             |
| Report and links           |      90 |                                                                         |
| Buffer                     |      45 |                                                                         |
| Total                      |     360 | Six hours; the buffer makes seven                                       |

If the replays run long, drop `track_a1` first and `track_t5` second: the control and `track`
pair carries every reading in Section 6 except the two named for them.

## 8. Limitations known before the run

- kirk0 is a tuning split. Nothing here is a held-out score, and the report must say so.
- The near-face residuals see only faces the sensor sees. When only a side is visible the
  along-path error is unmeasured; Step 6 is the only route to it, and it depends on references
  that may not exist. The along-path exposure share says how much of the pass is in that state.
- The reference face is the outermost labelled return less a trimmed share, so range noise of a few
  centimetres floors every residual, and a partly seen face gives a lower bound on where it ends.
- Nineteen objects on one capture, one placement, one day. Intervals resample objects and will be
  wide; a reading that depends on a handful of objects is reported with their count.
- The return-budget comparison assumes a mount height; the report states the value used.
- Update cost is not measured here. The campaign's timing figures stand.

## 9. What this run sets up, and does not do

The pack's optional `backgrounds.json` and `background.bin` hold the settled L3 baseline over the
excerpt. That is the input for the extent-end labeller the facet plan's E1 now requires, which
casts the reviewed body against the baseline and the frame's nearer returns to label each extent
end as physical, occluded, field of view, or dropout. It needs new code and is not part of this
session. Neither are the E1 synthetic must-pass checks, which belong in fixture-backed tests, nor
any facet extraction. This session's product is the error budget those decisions rest on.
