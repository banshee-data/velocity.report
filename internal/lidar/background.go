package lidar

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// Type aliases re-export L3 background model types from the l3grid package.
// These aliases enable gradual migration: callers can import from internal/lidar
// while the implementation lives in internal/lidar/l3grid.

// BackgroundParams configures the background model learning algorithm.
type BackgroundParams = l3grid.BackgroundParams

// RegionParams defines parameters that can vary per region.
type RegionParams = l3grid.RegionParams

// Region represents a contiguous spatial region with distinct parameters.
type Region = l3grid.Region

// RegionManager manages region identification and per-region parameterisation.
type RegionManager = l3grid.RegionManager

// BackgroundCell represents a single cell in the background grid.
type BackgroundCell = l3grid.BackgroundCell

// BackgroundGrid is the spatial grid used for background modelling.
type BackgroundGrid = l3grid.BackgroundGrid

// BackgroundManager orchestrates background model updates for a sensor.
type BackgroundManager = l3grid.BackgroundManager

// AcceptanceMetrics captures per-frame background acceptance statistics.
type AcceptanceMetrics = l3grid.AcceptanceMetrics

// CoarseBucket represents aggregate statistics for a grid region.
type CoarseBucket = l3grid.CoarseBucket

// GridHeatmap provides a coarse view of grid activity.
type GridHeatmap = l3grid.GridHeatmap

// BgStore defines the interface for persisting background snapshots.
type BgStore = l3grid.BgStore

// RegionStore defines the interface for persisting region data.
type RegionStore = l3grid.RegionStore

// ExportedCell represents a background cell for export/debugging.
type ExportedCell = l3grid.ExportedCell

// RegionInfo provides summary information about a detected region.
type RegionInfo = l3grid.RegionInfo

// RegionDebugInfo provides detailed diagnostics for a region.
type RegionDebugInfo = l3grid.RegionDebugInfo

// DriftMetrics captures background drift statistics.
type DriftMetrics = l3grid.DriftMetrics

// BackgroundSnapshotData contains complete background state for export.
type BackgroundSnapshotData = l3grid.BackgroundSnapshotData

// Constructor and function re-exports.

// NewRegionManager creates a RegionManager for the given grid dimensions.
var NewRegionManager = l3grid.NewRegionManager

// RegisterBackgroundManager registers a background manager for a sensor.
var RegisterBackgroundManager = l3grid.RegisterBackgroundManager

// GetBackgroundManager retrieves a registered background manager.
var GetBackgroundManager = l3grid.GetBackgroundManager

// NewBackgroundManager creates a new background manager.
var NewBackgroundManager = l3grid.NewBackgroundManager
