# Visualiser documentation

Architecture, API contracts, and feature specifications for
VelocityVisualiser, the macOS 3D point cloud viewer for LiDAR data.

## Architecture & design

| Document                               | Content                                 |
| -------------------------------------- | --------------------------------------- |
| [architecture.md](architecture.md)     | System design, Track A/B split          |
| [api-contracts.md](api-contracts.md)   | gRPC/protobuf `FrameBundle` wire format |
| [implementation.md](implementation.md) | Milestone checklist (M0–M8)             |
| [menu-layout.md](menu-layout.md)       | macOS menu bar and keyboard shortcuts   |

## Feature specifications

| Document                                                                   | Content                            |
| -------------------------------------------------------------------------- | ---------------------------------- |
| [light-mode.md](light-mode.md)                                             | Light/dark appearance tokens       |
| [performance-and-timeline-metrics.md](performance-and-timeline-metrics.md) | Scene health timeline              |
| [physics-checks.md](physics-checks.md)                                     | Violation types and thresholds     |
| [priority-review-queue.md](priority-review-queue.md)                       | Track review queue                 |
| [proto-contract.md](proto-contract.md)                                     | Track field serialisation contract |
| [qc-dashboard-and-audit.md](qc-dashboard-and-audit.md)                     | QC dashboard and audit export      |
| [qc-enhancements-overview.md](qc-enhancements-overview.md)                 | Shared QC table architecture       |
| [run-list-labelling-rollup.md](run-list-labelling-rollup.md)               | Run list labelling rollup icons    |
| [split-merge-repair.md](split-merge-repair.md)                             | Track split/merge repair workbench |
| [track-event-timeline.md](track-event-timeline.md)                         | Track event timeline bar           |
| [track-quality-scoring.md](track-quality-scoring.md)                       | Quality score weights and grades   |
| [trails-and-uncertainty.md](trails-and-uncertainty.md)                     | Ghost trails and uncertainty cones |
