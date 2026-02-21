# LiDAR Architecture

Status: Active

Architecture notes for the LiDAR pipeline.

## Layer Model

The runtime follows a six-layer model mapped to `internal/lidar/` packages:

- `l1packets/`: UDP/PCAP ingest and packet parsing
- `l2frames/`: frame assembly and geometric transforms
- `l3grid/`: background/foreground modeling and region behavior
- `l4perception/`: clustering, geometry, and ground-related filtering
- `l5tracks/`: multi-frame tracking and assignment
- `l6objects/`: semantic/object-level interpretation

Cross-cutting orchestration and IO live in `pipeline/`, `storage/`, `adapters/`, `sweep/`, and `monitor/`.

## Scope Separation

- Active runtime architecture docs live directly in this folder.
- Proposals and research docs live in `docs/proposals/lidar/architecture/`.

Use directory listings for per-document discovery to minimize stale index maintenance.
