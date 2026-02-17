// Package pipeline provides orchestration for the LiDAR tracking pipeline.
//
// It wires together stages from L3-L6 and adapter sinks (persistence,
// publish) into a coherent processing flow for both real-time and
// replay use cases. The pipeline does not own domain logic — it
// delegates to layer packages and adapters.
//
// See docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md
// for the design rationale.
package pipeline
