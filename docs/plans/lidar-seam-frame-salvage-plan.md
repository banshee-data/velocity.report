# LiDAR seam frame salvage (v0.6.1)

- **Status:** Draft
- **Layers:** LiDAR pipeline (L2 frame assembly, L3 background, L4/L5 perception and tracking)
- **Target:** v0.6.1; follows the multi-file replay work in
  [lidar-captures-multi-file-cases-plan](lidar-captures-multi-file-cases-plan.md) <!-- link-ignore -->
- **Canonical:** [ARCHITECTURE.md](../../ARCHITECTURE.md)

## Motivation

Multi-file replay discards the revolution straddling a capture-file join that is continuous
enough to cross but not seamless. That was the safe choice, and this plan asks whether it is
still the right one — whether the two partial revolutions either side can be kept instead.

The answer turns on facts that were not established when the drop was written. They are
established below, and they change the shape of the work: the obstacle is not the salvage
technique, it is that the pipeline has no way to say "this frame is good for background but not
for tracking".

## Current state

### What a lossy join actually costs

At a join with gap `G`, three things are in play:

|                                                | Recoverable?                                                  |
| ---------------------------------------------- | ------------------------------------------------------------- |
| The packets inside `G`                         | **No.** They were never captured. No technique recovers them. |
| The earlier file's tail — a partial revolution | Yes, currently discarded                                      |
| The later file's head — a partial revolution   | Yes, currently discarded                                      |

So the ceiling on salvage is **one revolution of real points per lossy join**, and the gap
itself is a permanent hole whatever we do.

### Measured: every join in the field corpus is already seamless

174 joins across the 15 sessions indexed from `/Volumes/lidar/lidar/s2` (189 captures, 122 GB):

|         | Gap      | As azimuth at 600 RPM |
| ------- | -------- | --------------------- |
| Minimum | 0.177 ms | 0.6°                  |
| Median  | 0.598 ms | 2.2°                  |
| Maximum | 5.479 ms | 19.7°                 |

Every one is inside the 10 ms seamless bound, so **the drop never fires on this corpus**. The
straddling revolution is fed straight through, missing a wedge of between half a degree and
twenty degrees of azimuth. That is the current, silent, behaviour — and it is fine, because of
the next finding.

### L3 does not treat an unobserved cell as evidence

[background_manager.go](../../internal/lidar/l3grid/background_manager.go) accumulates per-cell
counts for a frame and then skips any cell the frame did not observe:

```go
if counts[cellIdx] == 0 {
    continue
}
```

A cell in the missing wedge keeps its prior state rather than being told the space is empty.
**A partial frame is therefore already safe for background modelling.** This is the single most
important fact in this plan, and it removes most of the reason to reconstruct anything.

### Nothing downstream gates on frame completeness

`LiDARFrame` carries `SpinComplete`, `AzimuthCoverage`, `CompletenessRatio` and `PacketGaps`.
`calculateFrameCompleteness` populates them on every frame. Outside `l2frames`, **nothing reads
any of them.**

That is the real problem, and it is not confined to seams. Packet loss inside a single capture
already produces partial frames, and L4 clusters them and L5 tracks them as though they were
whole revolutions. A vehicle in a missing wedge is a track that ends for no reason.

## Findings

| Area                                 | Current state               | Severity | Release view                                                       |
| ------------------------------------ | --------------------------- | -------- | ------------------------------------------------------------------ |
| Completeness is computed and ignored | Four fields, no consumer    | High     | Pre-existing; affects every capture, not just seams                |
| Partial frames reach tracking        | No gate                     | High     | Same defect, visible as unexplained track ends                     |
| Straddling revolution discarded      | `DropNextFrame`             | Low      | Costs ≤1 revolution per lossy join; correct while there is no gate |
| Gap is unaccounted                   | Frame counts silently short | Medium   | Evaluation metrics under-count without saying why                  |
| Stitching not implemented            | —                           | Low      | Buys little once L3 is known to be partial-safe                    |

## Design / approach

### The obstacle is expressiveness, not technique

Salvaging the partials is easy. Making it _safe_ is not, because today every frame goes to every
layer. A partial frame that helps L3 harms L5, and there is no way to send it to one and not the
other. Until there is, discarding is the only choice that cannot mislead — which is why the drop
was right, and why lifting it starts with the gate rather than with the salvage.

### Frame quality, and who honours it

Three states, carried on the frame and set where the frame is built:

| Quality     | Means                                                             | L3 background | L4/L5 perception and tracking |
| ----------- | ----------------------------------------------------------------- | ------------- | ----------------------------- |
| `complete`  | A full revolution as captured                                     | Consume       | Consume                       |
| `partial`   | A real but incomplete sweep — a seam tail or head, or packet loss | Consume       | **Skip**                      |
| `composite` | Assembled from two sweeps captured `G` apart                      | Consume       | **Skip**                      |

`partial` is decided by the existing `AzimuthCoverage` against `MinAzimuthCoverage`, which is
already computed; the change is that something now acts on it.

The asymmetry is the point. Background is an average over time and missing observations only
slow it down. Tracking is a statement about where an object is _now_, and a frame with a hole in
it says an object left when it did not.

### Splitting: salvage without fabrication

At a lossy join, force a frame boundary instead of arming a discard:

1. Finalise the earlier file's tail as its own frame, `partial`.
2. Start the later file's head as a fresh frame, which completes at its next azimuth wrap —
   also `partial`.

No point is lost, nothing is invented, and both frames carry honest coverage figures. Once
quality is honoured, L3 gets every salvaged point and tracking is untroubled. This replaces
`DropNextFrame` with `SplitAtSeam` and is strictly less machinery than the drop it removes.

### Stitching: possible, and worth less than it looks

The two partials can be spliced into one apparently complete revolution, but only when their
azimuth ranges are complementary. After a gap `G` at spin rate `f`, the later file resumes at

```
phase_error = ((G × f) mod 1) × 360°
```

ahead of where the earlier one stopped. The splice is clean when `phase_error` is near zero —
that is, **when the gap is close to a whole number of revolutions**. At 10 Hz a 400 ms gap
splices perfectly; a 450 ms gap leaves the two halves overlapping by 180°, with a duplicated
wedge and a missing one.

Even where it is clean, it earns little. L3 already consumes the two partials and skips
unobserved cells, so it sees exactly the same points either way. Stitching preserves the frame
_count_ and produces something that would satisfy a completeness gate — and it does so by
asserting that points captured `G` apart belong to one instant, which for anything that moves is
false. A vehicle at 20 m/s travels 8 m in 400 ms and would appear twice in one frame.

**Recommendation: do not build the stitcher** unless a consumer appears that needs frame counts
to be exact and can tolerate a composite. Splitting delivers the salvage; stitching delivers a
number.

### Accounting for what is genuinely gone

A gap of `G` at spin rate `f` costs `G × f` revolutions that no technique recovers. Recording it
turns a silent shortfall into a stated one, so a run's frame count can be reconciled:

```
expected = elapsed × f
observed + missing_at_seams = expected
```

Without this, an evaluation run over a case with lossy joins is quietly short of frames and
nothing says why.

## Scope

### Item 1: make frame completeness load-bearing

**Summary:** Give frames a quality, and have the pipeline honour it.

**Steps:**

1. Add `Quality` to `LiDARFrame`, derived from `AzimuthCoverage` against `MinAzimuthCoverage`.
2. Gate L4/L5 on `complete`; leave L3 consuming everything.
3. Count skipped frames per run so the gate is visible rather than silent.
4. Tests: a partial frame reaches L3 and not L5; a complete one reaches both.

**Milestone:** v0.6.1. Valuable alone — it fixes in-file packet loss, which is more common than
lossy joins and today is silently tracked as though whole.

### Item 2: split at a lossy join instead of discarding

**Summary:** Keep both partial revolutions rather than dropping the straddling one.

**Steps:**

1. Replace `DropNextFrame` with `SplitAtSeam`: finalise the tail, start the head fresh.
2. Mark both `partial`; carry the join's gap on them for provenance.
3. Update `ReadPCAPSequence` to call it where it currently arms the drop.
4. Integration test over the reference capture: frame count rises by one per lossy join
   relative to the drop, no frame spans the gap, and the recovered points equal the tail plus
   the head.

**Milestone:** v0.6.1. Depends on item 1; without the gate this hands tracking a holed frame.

### Item 3: account for the revolutions inside the gap

**Summary:** State what was lost so frame counts reconcile.

**Steps:**

1. Emit a gap record per join: start, end, and `G × f` revolutions missing.
2. Surface it on the run and in the Captures session view beside the joins figure.
3. Reconcile expected against observed plus missing in the run summary.

**Milestone:** v0.6.1.

### Item 4: stitch complementary partials — deferred, see below

**Summary:** Splice tail and head into one `composite` revolution when the phase permits.

**Steps:**

1. Feasibility test on `phase_error`; splice only under a tolerance.
2. Mark `composite`; never admit it to L4/L5.
3. Report the proportion of joins where the phase permits it.

**Milestone:** Not scheduled. See the residuals.

## Dependencies

- Item 2 depends on item 1: splitting without the gate is worse than the drop it replaces.
- Item 3 needs a spin rate per join; the parser already reports motor speed per packet.
- Item 4 depends on items 1 and 2 and is not recommended.

## Risks

| Risk                                                                   | Likelihood | Impact | Mitigation                                                                                                                                                 |
| ---------------------------------------------------------------------- | ---------- | ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Gating L4/L5 on completeness drops frames that were previously tracked | High       | High   | The gate reveals a pre-existing defect rather than creating one; measure the skip rate across the corpus before enabling, and land it behind a tuning flag |
| `MinAzimuthCoverage` at 340° is the wrong threshold for a real sensor  | Medium     | Medium | Measure the coverage distribution over the 189 indexed captures first; the threshold has never been validated against field data because nothing read it   |
| Split frames change perf-gate frame counts                             | Medium     | Low    | No join in the current corpus is lossy, so the gate is unaffected until one appears; recapture baselines with the change if that stops being true          |
| Composite frames escape into tracking                                  | Low        | High   | Item 4 is not scheduled; if built, the gate from item 1 is the control                                                                                     |

## Checklist

### Outstanding

- [ ] Item 1: frame quality, honoured by L4/L5 (`M`)
- [ ] Item 2: split at a lossy join rather than discard (`S`)
- [ ] Item 3: account for the revolutions inside the gap (`S`)

### Deferred

- [ ] Item 4: stitching complementary partials. It recovers no points that splitting does not,
      because L3 skips unobserved cells; it preserves a frame count at the cost of asserting
      that points captured milliseconds to a second apart are simultaneous. Build it only for a
      consumer that needs the count and tolerates the composite.

### Accepted residuals (no action planned)

- [ ] The packets inside the gap stay lost. They were never captured.
- [ ] On the present corpus none of this changes any output: all 174 joins are seamless, no
      frame is discarded, and the straddling revolution is fed through missing between 0.6° and
      19.7° of azimuth. The work matters for volumes whose joins are lossy, and item 1 matters
      everywhere.
