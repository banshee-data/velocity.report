// Package adapters contains transport and IO boundary implementations
// for the LiDAR subsystem (HTTP, gRPC, UDP).
//
// Adapter code translates between external wire formats and internal
// domain types. Handlers and servers in this package should delegate
// business logic to layer packages and use-case services.
//
// See docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md §3
// for the design rationale.
package adapters
