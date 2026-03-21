// Package l8analytics owns Layer 8 (Analytics) of the LiDAR data model.
//
// Responsibilities: canonical run metrics, summaries, cross-run comparisons,
// scoring, percentile helpers, and evaluation logic. This is the single
// authoritative home for any analytic computation that spans multiple tracks
// or compares analysis runs.
//
// Dependency rule: L8 may depend on L1-L7 (and cross-cutting packages such as
// storage), but never on HTML, Svelte, Swift, chart libraries, or
// transport-layer response types.
//
// See docs/lidar/architecture/lidar-data-layer-model.md for the full
// layer model.
package l8analytics
