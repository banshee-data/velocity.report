# L2 Dual Representation (Polar and Cartesian)

Graduated plan: [lidar-l2-dual-representation-plan.md](../../plans/lidar-l2-dual-representation-plan.md)

**Status:** Implemented

## Decision

Store both polar and Cartesian representations once at L2. L3 consumes the
stored polar slice directly; L2/L9 consumers continue reading Cartesian.

This removes the per-frame `frame.Points → []PointPolar` rebuild in the
tracking callback without changing clustering/tracking maths.

## LiDARFrame Shape

```go
type LiDARFrame struct {
    PolarPoints []PointPolar  // sensor-polar view (canonical L3 input)
    Points      []Point       // sensor-Cartesian view (L2/L9 consumers)
    // ...
}
```

### Rules

- L1 parser emits `[]PointPolar`.
- L2 frame builder stores incoming polar, computes Cartesian once.
- `AddPointsPolar()` copies parser-owned input before storing.
- `len(frame.PolarPoints) == len(frame.Points)` for every completed frame.
- Per-index metadata matches between views.

## Consumer Access

| Consumer                    | Reads                                   |
| --------------------------- | --------------------------------------- |
| L3 foreground extraction    | `frame.PolarPoints`                     |
| L4 world transform          | `frame.PolarPoints` → foreground subset |
| gRPC visualiser point cloud | `frame.Points`                          |
| ASC frame export            | `frame.Points`                          |
| LidarView forwarding        | polar                                   |
| Foreground snapshot store   | polar-first                             |

## Risks

- **Memory growth** — two views per frame. Acceptable only if transient
  hot-path allocations are removed.
- **Partial migration** — no consumer should silently rebuild its own polar
  slice. The old rebuild path must be deleted.
