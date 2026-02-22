# OBB Heading Stability Review

**Status:** Partially Implemented (Fixes A, B, C applied; Fix D is config-only; Fix E pending proto regen; Fix F not yet started)
**Scope:** L4 clustering OBB, L5 tracking heading smoothing, visualiser rendering
**Created:** 2026-02-22
**Related:**
- [`docs/maths/clustering-maths.md`](../clustering-maths.md) (OBB via PCA, §5.2)
- [`docs/maths/tracking-maths.md`](../tracking-maths.md) (OBB heading handling, §7)
- [`docs/maths/proposals/20260220-velocity-coherent-foreground-extraction.md`](20260220-velocity-coherent-foreground-extraction.md) (velocity-coherent clustering)

---

## 1. Problem Statement

Tracked object bounding boxes **spin** visibly as vehicles move through the
scene. The boxes should remain relatively stable (aligned to the object's
physical shape), even though individual LiDAR points shift frame to frame.

Additionally, unassociated DBSCAN cluster boxes (cyan) are not visible in the
macOS visualiser — only track boxes appear (green for confirmed, yellow for
tentative). This needs clarification.

---

## 2. Root Cause Analysis

Five interacting problems produce the spinning-box symptom.

### 2.1 PCA 180° heading ambiguity with insufficient disambiguation

`EstimateOBBFromCluster` (`l4perception/obb.go`) computes heading from the
principal eigenvector of the 2D covariance matrix:

```text
heading = atan2(evY, evX)
```

PCA determines an **axis**, not a **direction**. The eigenvector can point in
either direction along the principal axis, producing heading values that differ
by π between frames for the same physical object. The result is a raw heading
that may flip by 180° whenever the point distribution shifts slightly.

The tracker (`l5tracks/tracking.go:1001–1024`) disambiguates using velocity:

```text
if speed > 0.5 m/s:
    flip heading toward velocity direction
```

**Problem:** When speed ≤ 0.5 m/s (slow-moving or momentarily stationary
objects, vehicles at junctions, pedestrians), no disambiguation occurs. The
raw PCA heading feeds directly into the EMA smoother, which then oscillates
as the sign alternates.

### 2.2 PCA 90° axis swap (length ↔ width swap)

PCA returns the eigenvector of the **largest** eigenvalue as the principal
axis. For a vehicle-sized cluster, this is normally the long axis (length
direction). However, when the point distribution becomes near-square — for
example due to partial occlusion, entry/exit framing, or point dropout on
one side — the eigenvalues approach equality and the principal axis can rotate
by approximately 90°.

When the principal axis rotates by 90°, two things happen simultaneously:

1. **Heading jumps by ~90°** (half a right angle, not caught by the 180°
   flip-toward-velocity guard).
2. **Length and width swap**: `obb.Length` becomes the old `obb.Width` and
   vice versa, because length is always defined as extent along the principal
   eigenvector.

The aspect-ratio lock guard (`tracking.go:981–996`) exists to suppress heading
updates when the cluster is near-square:

```text
aspectDiff / maxDim < OBBAspectRatioLockThreshold (0.25)
```

**Problem:** The threshold of 0.25 may be too loose. A cluster with aspect
ratio 1.33:1 (e.g. 2.0 m × 1.5 m) passes the guard but is close enough to
square that PCA can still swap axes between frames when a few points shift.

### 2.3 Dimension averaging without heading-locked axes

`BoundingBoxLengthAvg` and `BoundingBoxWidthAvg` are running averages
computed from per-frame OBB dimensions (`tracking.go:886–890`):

```go
track.BoundingBoxLengthAvg = ((n-1)*track.BoundingBoxLengthAvg + cluster.BoundingBoxLength) / n
track.BoundingBoxWidthAvg  = ((n-1)*track.BoundingBoxWidthAvg  + cluster.BoundingBoxWidth)  / n
```

Because `cluster.BoundingBoxLength` is the OBB extent along the **current
frame's** principal axis, and the principal axis can flip between frames
(§2.1, §2.2), the "length" and "width" labels are not consistent across
observations.

**Effect:** When axis swaps occur, the average dimensions converge toward
each other (both approach the mean of the true length and true width). For a
4 m × 2 m vehicle with frequent axis swaps, the averages would converge toward
3 m × 3 m, producing a square box.

### 2.4 Renderer dimension-heading mismatch

The macOS renderer (`MetalRenderer.swift:460–512`) builds track box transforms
using:

- **Dimensions:** `bboxLengthAvg` × `bboxWidthAvg` × `bboxHeightAvg`
  (running averages)
- **Heading:** `bboxHeadingRad` (smoothed OBB heading from tracker)

These two signals are not synchronised:

1. The heading is smoothed via EMA from per-frame OBB headings.
2. The dimensions are cumulative averages that include frames where the
   axis was swapped.

Result: the heading rotates but the box dimensions are averaged from mixed
orientations, producing visible spinning as the heading changes but the box
shape stays roughly square.

The web renderer (`MapPane.svelte:582–601`) has a partial fix — it uses
**per-frame** OBB dimensions (`bbox.length` / `bbox.width`) rather than
averaged dimensions, but still applies the smoothed OBB heading. This is
better, but the per-frame dimensions still swap when PCA axes swap (§2.2).

### 2.5 Why unassociated (DBSCAN) cluster boxes are not visible

The adapter (`adapter.go:212–259`) intentionally skips clusters that have been
associated with a track:

```go
if i < len(associations) && associations[i] != "" {
    continue  // Skip — rendered via the track instead
}
```

When tracking works well, **all** clusters associate with tracks, leaving zero
unassociated clusters. The cyan cluster boxes are therefore not rendered — only
track boxes (green/yellow) appear.

This is correct behaviour by design. However, it means that during the
investigation, DBSCAN cluster-level OBB quality can only be observed indirectly
through track boxes. To debug raw DBSCAN OBB stability, a diagnostic mode that
renders all cluster boxes (ignoring association) would be valuable.

---

## 3. Problematic Code Paths (Summary)

| Location | Issue |
|---|---|
| `l4perception/obb.go:103` | `heading = atan2(evY, evX)` — raw PCA heading has 180° ambiguity |
| `l4perception/obb.go:142–145` | `length` / `width` defined by principal axis — swaps when axis flips |
| `l5tracks/tracking.go:1006` | Velocity disambiguation only when speed > 0.5 m/s |
| `l5tracks/tracking.go:992` | Aspect-ratio lock threshold 0.25 may be too loose |
| `l5tracks/tracking.go:886–890` | Dimension averaging not axis-locked — averages mixed orientations |
| `l5tracks/tracking.go:1044–1048` | Per-frame OBB dimensions updated without axis consistency check |
| `MetalRenderer.swift:471–475` | Track boxes use averaged dims with smoothed heading (mismatch) |
| `adapter.go:223–225` | Associated clusters not rendered (correct, but hinders debugging) |

---

## 4. Relationship to Math Proposals

### 4.1 Clustering maths (clustering-maths.md §5.2)

The document correctly notes:

> PCA OBB for shape — Stable for elongated objects, ambiguous for near-square
> clusters.

This is the theoretical foundation of problem §2.2 above. The document does
not address the practical consequences for tracking (dimension averaging
with axis swaps).

### 4.2 Tracking maths (tracking-maths.md §7)

The document describes heading handling:

> 1. optional flip toward velocity direction when speed is sufficient,
> 2. wrap-aware EMA smoothing

This covers §2.1 (velocity disambiguation) but does not discuss:
- What happens at low speeds (no disambiguation).
- The axis-swap problem (§2.2) where heading jumps by 90°, not 180°.
- The dimension averaging inconsistency (§2.3).

### 4.3 Velocity-coherent foreground extraction (20260220 proposal)

This proposal enriches points with velocity estimates before clustering. When
implemented, per-point velocity vectors would provide an additional heading
signal that is independent of PCA axis orientation:

```text
cluster heading = mean(point velocity headings)
```

This would mitigate §2.1 and §2.2 for clusters with reliable velocity
estimates but would not fix the dimension-averaging problem (§2.3) or the
renderer mismatch (§2.4).

### 4.4 Unify L3/L4 settling (20260219 proposal)

Not directly related to OBB heading stability, but the shared confidence
substrate could improve upstream point-set stability (fewer background points
contaminating foreground clusters), which would reduce PCA noise.

---

## 5. Proposed Fixes

### Fix A: Canonical-axis dimension normalisation (addresses §2.2, §2.3)

**Problem:** Length and width swap when PCA axis flips.

**Fix:** After computing the OBB, normalise dimensions so that
`length >= width` always, and adjust heading accordingly:

```go
if obb.Width > obb.Length {
    obb.Length, obb.Width = obb.Width, obb.Length
    obb.HeadingRad += π/2
    // re-normalise heading to [-π, π]
}
```

This ensures "length" always means the longer extent and "width" always
means the shorter extent, regardless of which eigenvector PCA chose as
principal. The heading is adjusted to match.

**Impact:** Eliminates the 90° axis-swap problem. Running averages of
length and width become meaningful because they always refer to the same
physical axis.

**Effort:** Small — localised change in `EstimateOBBFromCluster`.

### Fix B: Smoothed dimensions in the tracker (addresses §2.3, §2.4)

**Problem:** Per-frame OBB dimensions jitter, and averaged dimensions blend
mixed orientations.

**Fix:** Apply EMA smoothing to length, width, and height in the tracker
(analogous to heading smoothing), using the same α:

```go
track.OBBLength = (1-α)*track.OBBLength + α*cluster.OBB.Length
track.OBBWidth  = (1-α)*track.OBBWidth  + α*cluster.OBB.Width
track.OBBHeight = (1-α)*track.OBBHeight + α*cluster.OBB.Height
```

This provides temporal stability without the axis-mixing problem of
cumulative averaging (because Fix A ensures consistent axis labelling).

**Impact:** Smoother box dimensions on screen. Combined with Fix A, prevents
dimension convergence toward square.

**Effort:** Small — localised change in tracker update.

### Fix C: Improve low-speed heading disambiguation (addresses §2.1)

**Problem:** Heading flip disambiguation only works above 0.5 m/s.

**Fix:** When speed is below the threshold, use the track's own recent
displacement vector (from position history) as the reference direction
instead of the Kalman velocity:

```go
if speed <= 0.5 && len(track.History) >= 2 {
    last := track.History[len(track.History)-1]
    prev := track.History[len(track.History)-2]
    dx := last.X - prev.X
    dy := last.Y - prev.Y
    displacement := sqrt(dx*dx + dy*dy)
    if displacement > 0.1 {  // 10 cm minimum displacement
        refHeading = atan2(dy, dx)
        // use refHeading for disambiguation
    }
}
```

As a fallback when both speed and displacement are insufficient (truly
stationary object), lock heading to the previous smoothed value (do not
update).

**Impact:** Reduces heading oscillation for slow-moving objects.

**Effort:** Small — localised change in tracker heading update block.

### Fix D: Tighten aspect-ratio lock threshold (addresses §2.2)

**Problem:** Threshold 0.25 permits near-square clusters to update heading.

**Fix:** Reduce `obb_aspect_ratio_lock_threshold` from 0.25 to 0.15 or lower.
This locks heading for any cluster where the length/width difference is less
than 15% of the longest dimension.

**Impact:** Fewer spurious heading updates from near-square clusters.

**Effort:** Config-only change — no code modification.

**Risk:** Setting this too tight suppresses legitimate heading updates for
vehicles viewed at angles where the cross-section appears near-square.
Validate against replay data.

### Fix E: Renderer uses per-frame (or smoothed) dimensions consistently (addresses §2.4)

**Problem:** macOS renderer uses averaged dimensions; web renderer uses
per-frame dimensions.

**Fix:** Both renderers should use the same smoothed dimensions from Fix B:

```swift
// MetalRenderer.swift — use per-frame (smoothed by tracker) dimensions
let scale = simd_float4x4(
    diagonal: simd_float4(
        track.bboxLength > 0 ? track.bboxLength : 1.0,
        track.bboxWidth  > 0 ? track.bboxWidth  : 1.0,
        track.bboxHeight > 0 ? track.bboxHeight : 1.0, 1.0))
```

**Impact:** Box dimensions and heading are synchronised from the same
smoothing pipeline.

**Effort:** Small — localised change in renderer.

### Fix F: Debug mode for raw cluster OBB rendering (addresses §2.5)

**Problem:** Cannot visually inspect DBSCAN OBB quality because associated
clusters are hidden.

**Fix:** Add a visualiser toggle (or debug overlay) that renders all cluster
OBBs regardless of association status. This would show:
- Cyan boxes for all DBSCAN clusters
- Green/yellow boxes for tracks (overlapping associated clusters)

**Implementation:** The adapter already has `adaptClusters` (adapter.go:261–288)
which converts all clusters without filtering. Wire this to a debug flag.

**Impact:** Enables visual debugging of OBB stability at the DBSCAN level,
before tracking smoothing is applied.

**Effort:** Small — flag + conditional in adapter and renderer.

---

## 6. Recommended Implementation Order

1. **Fix A** (canonical-axis normalisation) — eliminates the root cause of
   axis swaps. All downstream fixes build on this.
2. **Fix D** (tighten aspect-ratio threshold) — config-only, reduces noise
   while Fix A is validated.
3. **Fix C** (low-speed disambiguation) — addresses the remaining 180° flip
   cases.
4. **Fix B** (smoothed dimensions) — temporal stability for rendering.
5. **Fix E** (renderer consistency) — synchronise both renderers.
6. **Fix F** (debug cluster rendering) — optional, for ongoing tuning.

Fixes A, D, and C address correctness. Fixes B and E address rendering
quality. Fix F addresses observability.

---

## 7. Validation Approach

### 7.1 Unit test additions

- `obb_test.go`: Verify that after Fix A, `Length >= Width` always holds.
- `obb_test.go`: Verify heading adjustment when width > length.
- `tracking_test.go`: Verify heading stability for near-square clusters
  across multiple frames.
- `tracking_test.go`: Verify dimension consistency when PCA axis would
  otherwise swap.

### 7.2 Replay evaluation

- Run existing `.vrlog` captures through the pipeline with and without fixes.
- Measure heading jitter RMS (already tracked in
  `HeadingJitterSumSq`/`HeadingJitterCount`).
- Measure dimension convergence (average length/width should reflect true
  vehicle proportions, not converge to square).

### 7.3 Visual inspection

- With Fix F enabled, compare raw DBSCAN OBBs against smoothed track OBBs
  in the macOS visualiser.
- Confirm that cyan (cluster) boxes spin but green/yellow (track) boxes do
  not after fixes are applied.

---

## 8. Interaction with Existing Metrics

The tracker already computes:

- **Heading jitter** (`HeadingJitterSumSq` / `HeadingJitterCount`) — RMS of
  frame-to-frame heading changes. This directly measures the spinning problem.
  After fixes, expect significant reduction.
- **Velocity-trail alignment** (`AlignmentMeanRad`) — measures Kalman velocity
  vs displacement direction. Not directly affected by OBB fixes.
- **Speed jitter** (`SpeedJitterSumSq` / `SpeedJitterCount`) — not directly
  related but may improve if tracking association quality improves.

---

## 9. Open Questions

1. Should the canonical-axis normalisation (Fix A) be applied at the cluster
   level (`EstimateOBBFromCluster`) or at the tracker level when consuming
   the OBB? Applying at cluster level is simpler and benefits all consumers.

2. What is the optimal aspect-ratio lock threshold (Fix D)? The current 0.25
   is likely too loose; 0.15 is proposed but should be validated against replay
   data across multiple sites.

3. Should the velocity-coherent extraction proposal (20260220) subsume Fix C?
   If per-point velocity vectors become available, they provide a stronger
   heading signal than displacement history. However, Fix C is simpler and
   can be implemented immediately.

4. Should smoothed dimensions (Fix B) replace the current cumulative average
   (`BoundingBoxLengthAvg`/`BoundingBoxWidthAvg`)? The cumulative average
   has value for long-term classification features; the EMA is better for
   rendering. Consider keeping both.
