// Package pipeline provides the real-time tracking pipeline that orchestrates
// processing stages from L3 Grid through L6 Objects.
//
// This package is the composition root: it imports from layer packages
// (l2frames, l3grid, l4perception, l5tracks, l6objects) and storage, but
// none of those packages import pipeline/.
//
// See docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md
// for the design rationale.
package pipeline
