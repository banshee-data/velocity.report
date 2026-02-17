// Package l1packets owns Layer 1 (Packets) of the LiDAR data model.
//
// Responsibilities: raw UDP/serial packet ingestion, PCAP replay, and
// low-level byte parsing. This layer produces raw point arrays consumed
// by L2 (Frames).
//
// Dependency rule: L1 has no inward dependencies on higher layers.
//
// See docs/lidar/architecture/lidar-data-layer-model.md for the full
// layer model and docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md
// for the migration plan.
package l1packets
