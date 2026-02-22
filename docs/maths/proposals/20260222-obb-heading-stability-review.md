# OBB Heading Stability Review

**Status:** Revised — Fix A reverted (unsuitable for non-vehicular objects); replaced with tracker-level 90° jump rejection. Fixes B, C applied. Heading-source debug rendering added. Fix D config-only; Fix E/F not yet started.
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

| Location | Issue | Status |
|---|---|---|
| `l4perception/obb.go:103` | `heading = atan2(evY, evX)` — raw PCA heading has 180° ambiguity | Mitigated by velocity/displacement disambiguation |
| `l4perception/obb.go:142–145` | `length` / `width` defined by principal axis — swaps when axis flips | Handled by 90° jump rejection (Guard 3) |
| `l5tracks/tracking.go` | Velocity disambiguation only when speed > 0.5 m/s | Displacement fallback added (Fix C) |
| `l5tracks/tracking.go` | Aspect-ratio lock threshold 0.25 may be too loose | Open (Fix D) |
| `l5tracks/tracking.go` | 90° heading jumps from PCA axis swaps | **Fixed:** Guard 3 rejects 60°–120° jumps |
| `l5tracks/tracking.go` | No heading-source diagnostic data | **Fixed:** HeadingSource enum added (Fix G) |
| `l5tracks/tracking.go` | Dimension averaging not axis-locked | EMA smoothing added (Fix B) |
| `MetalRenderer.swift` | Track boxes use averaged dims with smoothed heading | Comment updated; Fix E pending |
| `adapter.go` | Associated clusters not rendered (hinders debugging) | Open (Fix F) |

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

### Fix A: ~~Canonical-axis dimension normalisation~~ REVERTED

**Original proposal:** Force `Length >= Width` always, rotating heading by
π/2 when the perpendicular extent exceeds the principal extent.

**Reverted because:** Pedestrian and other non-vehicular clusters are
legitimately square or near-square. The normalisation incorrectly rotated
their headings by 90° every frame, causing the very spinning it was meant
to prevent. Only vehicle-shaped clusters (high aspect ratio) would benefit,
but the heuristic cannot distinguish object types at the OBB level.

**Replaced by:** Tracker-level 90° jump rejection (Guard 3 in the heading
update block). If the heading delta vs the previous smoothed heading is
between 60°–120° (i.e. near ±90°), the update is rejected and the heading
is locked. This catches PCA axis swaps for near-square clusters that
passed the aspect-ratio guard (Guard 2) without incorrectly rotating
headings for legitimately square objects.

### Fix G: Heading-source debug rendering (NEW)

**Problem:** Cannot determine which component is responsible for heading
drift without additional diagnostic tooling.

**Fix:** Added `HeadingSource` enum tracking through the full stack:
tracker → adapter → model → proto → gRPC → macOS renderer → web API.
Values: `PCA` (0), `velocity` (1), `displacement` (2), `locked` (3).

macOS visualiser gains `showHeadingSource` toggle that colours confirmed
track boxes by heading source instead of lifecycle state:
- **Blue** — velocity-disambiguated (healthy)
- **Yellow** — raw PCA (no disambiguation available)
- **Orange** — displacement-disambiguated (slow-moving)
- **Grey** — heading locked (aspect ratio guard or 90° jump rejection)

Web API exposes `heading_source` field in `TrackResponse` JSON.

**Impact:** Enables real-time visual diagnosis of which heading path is
responsible for angular drift on any given track.

### Fix B: Use cluster dimensions directly (addresses §2.3, §2.4)

**Problem:** Per-frame OBB dimensions jitter, and EMA-smoothed dimensions
mixed swapped axes when heading was locked due to PCA axis swaps.

**Fix:** Use cluster (DBSCAN) dimensions directly for per-frame rendering
instead of EMA smoothing. When the heading is updated normally, the cluster
dimensions are consistent with the heading. When the heading is locked
(Guard 1/2/3), only update height — length and width are held to avoid
desynchronising dimensions from the locked heading.

```go
if updateHeading {
    track.OBBLength = cluster.OBB.Length
    track.OBBWidth  = cluster.OBB.Width
    track.OBBHeight = cluster.OBB.Height
} else {
    // Only update height (axis-independent)
    track.OBBHeight = cluster.OBB.Height
}
```

This avoids the axis-mixing problem because the tracker-level 90° heading
jump rejection (Guard 3) prevents sudden relabelling of axes, and dimensions
are only updated when heading and dimensions are known to be consistent.

**Impact:** Per-frame box dimensions match the DBSCAN cluster dimensions
exactly, producing boxes that capture all cluster points. Dimensions remain
anisotropic for elongated objects instead of converging towards a square.

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

1. **Guard 3** (90° heading jump rejection) — catches PCA axis swaps at the
   tracker level. Replaces Fix A (canonical-axis normalisation, reverted).
2. **Fix C** (low-speed disambiguation) — addresses 180° flip cases via
   displacement fallback.
3. **Fix B** (use cluster dimensions directly) — per-frame rendering uses
   DBSCAN dimensions. Skips dimension updates when heading is locked to
   maintain axis consistency.
4. **Fix D** (tighten aspect-ratio threshold) — config-only, reduces noise.
5. **Fix G** (heading-source debug rendering) — colour-code boxes by heading
   origin for drift diagnosis.
6. **Fix E** (renderer consistency) — synchronise both renderers.
7. **Fix F** (debug cluster rendering) — optional, for ongoing tuning.

Guard 3, fixes B, C, and G are implemented.
Fix D is config-only. Fixes E and F are not yet started.

---

## 7. Validation Approach

### 7.1 Unit test additions

- `obb_test.go`: Verify PCA returns natural axes without forced normalisation.
- `obb_test.go`: Verify correct dimensions for rectangles in X and Y.
- `tracking_test.go`: Verify heading stability for near-square clusters
  across multiple frames.
- `tracking_test.go`: Verify dimension consistency when PCA axis would
  otherwise swap.

### 7.2 Replay evaluation

- Run existing `.vrlog` captures through the pipeline with and without fixes.
- Measure heading jitter RMS (already tracked in
  `HeadingJitterSumSq`/`HeadingJitterCount`).
- Use heading-source debug rendering (Fix G) to identify which tracks have
  unstable heading sources.

### 7.3 Visual inspection

- Enable `showHeadingSource` in macOS visualiser to see heading-source
  colour coding (blue/yellow/orange/grey) on track boxes.
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

1. **Resolved:** Canonical-axis normalisation (Fix A) was reverted because it
   incorrectly rotates headings for pedestrian and other square clusters.
   The 90° jump rejection (Guard 3) handles the axis-swap problem at the
   tracker level where temporal context is available.

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

5. Should the 90° jump rejection threshold (60°–120° band) be configurable?
   Currently hardcoded as π/3 to 2π/3. This should cover all practical PCA
   axis-swap scenarios but may need tuning for unusual sensor geometries.
