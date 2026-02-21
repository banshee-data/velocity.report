# Documentation Structure Audit

Status: Historical
Scope: `docs/**`

## Objective

Audit documentation placement and separate proposal/planning content from implemented/operational content.

## Separation Rule

1. `docs/proposals/` contains non-runtime design/draft/research documents.
2. Non-`docs/proposals/` folders contain implemented behavior, active architecture, or operations.

## Reorganization Completed

### New proposal folders

- `docs/proposals/features/`
- `docs/proposals/lidar/architecture/`
- `docs/proposals/lidar/visualiser/`
- `docs/proposals/maths/`

### Proposal index policy update

Proposal folders were initially given local `README.md` indexes, then those were removed in the same audit cycle to reduce maintenance overhead and avoid stale file listings. Navigation is now by directory listing.

### Files moved

1. `docs/features/serial-configuration-ui.md` -> `docs/proposals/features/serial-configuration-ui.md`
2. `docs/features/site-config-cosine-correction-spec.md` -> `docs/proposals/features/site-config-cosine-correction-spec.md`
3. `docs/features/speed-limit-schedules.md` -> `docs/proposals/features/speed-limit-schedules.md`
4. `docs/features/time-partitioned-data-tables.md` -> `docs/proposals/features/time-partitioned-data-tables.md`
5. `docs/lidar/architecture/av-range-image-format-alignment.md` -> `docs/proposals/lidar/architecture/av-range-image-format-alignment.md`
6. `docs/lidar/architecture/dynamic-algorithm-selection.md` -> `docs/proposals/lidar/architecture/dynamic-algorithm-selection.md`
7. `docs/lidar/architecture/gps-ethernet-parsing.md` -> `docs/proposals/lidar/architecture/gps-ethernet-parsing.md`
8. `docs/lidar/architecture/lidar-multi-model-ingestion-and-configuration.md` -> `docs/proposals/lidar/architecture/lidar-multi-model-ingestion-and-configuration.md`
9. `docs/lidar/architecture/lidar-network-configuration.md` -> `docs/proposals/lidar/architecture/lidar-network-configuration.md`
10. `docs/lidar/architecture/vector-scene-map.md` -> `docs/proposals/lidar/architecture/vector-scene-map.md`
11. `docs/lidar/visualiser/06-labelling-qc-enhancements-overview.md` -> `docs/proposals/lidar/visualiser/06-labelling-qc-enhancements-overview.md`
12. `docs/lidar/visualiser/07-track-quality-score.md` -> `docs/proposals/lidar/visualiser/07-track-quality-score.md`
13. `docs/lidar/visualiser/08-track-event-timeline-bar.md` -> `docs/proposals/lidar/visualiser/08-track-event-timeline-bar.md`
14. `docs/lidar/visualiser/09-split-merge-repair-workbench.md` -> `docs/proposals/lidar/visualiser/09-split-merge-repair-workbench.md`
15. `docs/lidar/visualiser/10-trails-and-uncertainty-visualisation.md` -> `docs/proposals/lidar/visualiser/10-trails-and-uncertainty-visualisation.md`
16. `docs/lidar/visualiser/11-physics-checks-and-confirmation-gates.md` -> `docs/proposals/lidar/visualiser/11-physics-checks-and-confirmation-gates.md`
17. `docs/lidar/visualiser/12-priority-review-queue.md` -> `docs/proposals/lidar/visualiser/12-priority-review-queue.md`
18. `docs/lidar/visualiser/13-qc-dashboard-and-audit-export.md` -> `docs/proposals/lidar/visualiser/13-qc-dashboard-and-audit-export.md`
19. `docs/lidar/visualiser/15-performance-and-scene-health-timeline-metrics.md` -> `docs/proposals/lidar/visualiser/15-performance-and-scene-health-timeline-metrics.md`
20. `docs/lidar/future/track-labeling-auto-aware-tuning.md` -> `docs/lidar/future/track-labeling-auto-aware-tuning-future.md`
21. `docs/maths/ground-plane-maths.md` -> `docs/proposals/maths/ground-plane-vector-scene-maths.md`

### Files created for clean implementation/proposal split

- `docs/maths/ground-plane-maths.md` (implemented runtime math only)
- `docs/proposals/maths/ground-plane-vector-scene-maths.md` (proposal math)
- `docs/lidar/operations/track-labeling-auto-aware-tuning.md` (implemented runtime status only)

## Link and Path Cleanup

- Updated internal links to new proposal paths across `docs/`.
- Updated roadmap/track-labelling references after moving track-labelling doc to operations.
- Updated stale LiDAR package path links in troubleshooting checklist.
- Removed stale `.DS_Store` files under `docs/`.
- Removed list-only README indexes from proposal folders and kept only concise structural READMEs where needed.

## Verification

A full relative-link sweep across `docs/**/*.md` now reports no broken internal markdown links.

## Remaining Intentional Mixed Docs

Some status/roadmap docs include both implemented and planned sections by design:

- `docs/lidar/ROADMAP.md`
- `docs/lidar/refactor/01-tracking-upgrades.md`

These are status-tracking documents, not proposal specs.
