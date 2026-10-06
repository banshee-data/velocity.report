# Near-face evaluation

How to score where an estimated body puts the face the sensor sees, against the returns a person
labelled on it, and how to draft the split manifest that selects the labelled window.

- **Status:** Implemented and tested on synthetic packs; the first run on a reviewed pack is open
- **Layers:** L5 Tracks, L8 Analytics, offline analysis
- **Related:** [Per-frame evaluation](per-frame-evaluation.md), [Point annotation tool](point-annotation-tool.md), [near-edge tracked state](../../plans/lidar-near-edge-tracked-state-plan.md), [physical reference review](../../plans/lidar-physical-reference-review-plan.md)
- **Code:** [perframeeval/neareface.go](../../../internal/lidar/perframeeval/neareface.go), [lidar-near-face-eval](../../../cmd/tools/lidar-near-face-eval/main.go), [split_draft.go](../../../internal/lidar/annotation/split_draft.go), [lidar-annotation-split-draft](../../../cmd/tools/lidar-annotation-split-draft/main.go)

## What it answers

[Per-frame evaluation](per-frame-evaluation.md) says whether tracks keep their identity. Its
reference is the centre of the visible returns, which sits toward the sensor as the medoid does, so
it cannot say whether a body estimate is accurate. Physical references would, and a person has to
author them. This is the part of the answer the labels already give.

Take the returns of a reviewed mask and the body an arm believed at that frame, put the returns in
the body's own axes, and look at the faces the sensor can see. The returns are not tracker output,
and where the sensor sees a face is where the face is, to the sensor's noise, whatever the body's
far side does.

| Residual    | What it is                                                                       | Sign and meaning                                                                                                                 |
| ----------- | -------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| End normal  | Half the believed length minus the outermost labelled return, on the sensor side | Positive: the believed face is out toward the sensor beyond the returns. A centre biased toward the sensor, or a length too long |
| Side normal | The same for the sensor-facing side and half the believed width                  | As above                                                                                                                         |
| End tangent | Offset of the believed centre from the midpoint of the end face's lateral span   | Absolute value only: its sign follows the heading, which may be ambiguous. The error along the face that a rank-one fix leaves   |

The tangent is reported only where the span covers 0.7 to 1.5 of the believed width and enough
returns lie within 0.3 m of the face; otherwise it is withheld and counted, and the normal residuals
still count. A face is scored only if the sensor is outside the believed box's slab on that axis,
because a face edge-on to the sensor is not seen.

A body whose extents are class priors is scored but kept in its own stratum: its residual mostly
measures the prior. Strata are all instants, extents learnt or prior, and range bands.

## What it does not answer

- The centre along an axis whose face the sensor cannot see, the far bumper and yaw. Those need
  physical references ([review plan](../../plans/lidar-physical-reference-review-plan.md)).
- Identity. Score that with the per-frame evaluator, on the same labels.
- Whether a centre or an extent is wrong. A positive normal residual is either; the two are not
  told apart.
- The size of a face the sensor sees only part of. The outermost return is a lower bound on it.

Every mask is saved as partial by default, and a partial mask still certifies that its returns
belong to the object, so partial masks are scored. A split that is not held out is scored only with
`-allow-tuning-split`, and the report says it is not held out.

## Drafting the split manifest

Replay the arms first (below), then draft from where their estimates start:

```bash
go run ./cmd/tools/lidar-annotation-split-draft -pack "$PACK" \
  -from-evidence "$OUT/control/evidence/kirk0.db" \
  -output "$LIDAR_ANNOTATION_DIR/splits/kirk0-tuning.json"
```

It takes every object that would survive freezing: a road user, reviewed, with no mask still
proposed or of unstated completeness, and at least one scored mask in the window. It lists what it
left out and why, pins the annotation revision it read, and never overwrites a file.

The window starts where the replay's estimates do, because a replay writes none before its first
confirmed track, and a mask earlier than that reads as an object nobody found. That is not where
`-warmup` ends: on kirk0, with `-warmup 20`, the first solid-body row is 6.2 s into the capture.
`-from-evidence` reads it from the database; `-from-seconds` and `-from-sample` set it by hand, and
`-to-sample` bounds the end.

To freeze the draft, add `-freeze-draft FILE -case CASE -capture BASENAME=SHA256`, then run
`velocity lidar annotation-split freeze --draft FILE --author NAME --output SPLIT`. A frozen split
binds a role to a capture by its digest, and a pack cut from a case's capture takes the case's
role: kirk0 is the capture the shadow work was developed on, so its role is `tuning`, never held
out.

## Scoring

An arm is one evidence database of solid bodies. Replay with the solid body on and keep the
evidence database (no `-discard-evidence`). kirk0 runs as a one-case corpus:

```bash
BASE=solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members
for arm in control track; do
  EXP=$BASE; [ "$arm" = track ] && EXP=$BASE,near_edge_track
  go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline \
    -corpus cmd/tools/lidar-state-estimation-baseline/testdata/kirk0-corpus.json \
    -index cmd/tools/lidar-state-estimation-baseline/testdata/kirk0-index.json \
    -pcap-root internal/lidar/perf -pcap-subdir pcap -warmup 20 -case kirk0 \
    -experiment "$EXP" -sample-points 256 \
    -source-manifest "$OUT/$arm/source-manifest.json" -out "$OUT/$arm/out" \
    -evidence-dir "$OUT/$arm/evidence" -evidence-per-case
done
```

Then score both arms on the labels, and the identity of the same two arms beside them:

```bash
go run ./cmd/tools/lidar-near-face-eval -pack "$PACK" \
  -split-manifest "$LIDAR_ANNOTATION_DIR/splits/kirk0-tuning.json" -split tuning -allow-tuning-split \
  -arm control="$OUT/control/evidence/kirk0.db" -arm track="$OUT/track/evidence/kirk0.db" \
  -json faces.json -markdown faces.md

go run ./cmd/tools/lidar-ground-truth-eval perframe -pack "$PACK" \
  -split-manifest "$LIDAR_ANNOTATION_DIR/splits/kirk0-tuning.json" -split tuning -allow-tuning-split \
  -a-label control -a-db "$OUT/control/evidence/kirk0.db" -a-stage online -a-declared-baseline \
  -b-label track -b-db "$OUT/track/evidence/kirk0.db" -b-stage online -b-declared-baseline \
  -json identity.json -markdown identity.md
```

The first arm is the one the others are compared with. Per arm the report gives what was labelled,
matched and scored, the reasons for what was not, the residuals by stratum (mean, median, RMS, p95
and p99 of the absolute value, and the share over 10 cm), and the paired difference against the
first arm on the instants both scored. `-instants` keeps every scored instant in the JSON.

| Flag                                       | Meaning                                                                          |
| ------------------------------------------ | -------------------------------------------------------------------------------- |
| `-gate-metres`, `-frame-tolerance-ms`      | How a body is matched to a mask: this plus half the footprint diagonal; time     |
| `-min-returns`                             | Returns a mask needs to place a face                                             |
| `-face-quantile`                           | Share of the outermost returns ignored, as a whole count (none below fifty)      |
| `-face-band-metres`                        | Depth from the face within which its span is measured                            |
| `-min-span-coverage`, `-max-span-coverage` | Share of the believed width the end face's span must cover for the tangent       |
| `-sensor-x`, `-sensor-y`                   | The sensor's place in the pack's frame (the origin for a pack with no transform) |

The centre-to-footprint distance in each arm's accounting is a check that arm and pack share a
frame: a metre or so is usual, because the mask sits on the faces the sensor sees, and a median of
several metres means they do not.

## What remains

- The first run on a reviewed pack. kirk0's pack has 20 road-user objects, 19 of them (about
  3,100 of 3,400 scored masks) after the first estimate at 6.2 s.
- Held-out use. kirk0 cannot be held out. A labelled `embarcadero-folsom` window cut through the
  Segments flow is what a held-out score needs.
- The side-face tangent and a far-face estimate are not scored.
