# LiDAR Maths

## Scope

This folder covers estimation, filtering, gating, optimization, and confidence math that affects runtime behavior.

## Separation

- Implemented/runtime maths docs live directly in `docs/maths/`.
- Proposed or deferred maths models live in `docs/maths/proposals/`.

## Workstream Boundary

- Vector-grid foundations (current runtime path) are documented as implemented maths.
- Velocity-coherent extraction and other non-runtime models are documented as proposals.

Use directory listings for file-level navigation.

## Document layout

```

# $Topic

**Status:** $status
**Layer:** L$n Tracks (`internal/lidar/$track`)
**Related:** [$related]($related.md)

## 1. Purpose

...
```
