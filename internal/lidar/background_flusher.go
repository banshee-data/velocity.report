package lidar

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// Type aliases and function re-exports for background flusher.

// Persister defines the interface for persisting background data.
type Persister = l3grid.Persister

// BackgroundFlusher manages periodic background snapshots.
type BackgroundFlusher = l3grid.BackgroundFlusher

// BackgroundFlusherConfig configures the background flusher.
type BackgroundFlusherConfig = l3grid.BackgroundFlusherConfig

// Function re-exports.

// NewBackgroundFlusher creates a new background flusher.
var NewBackgroundFlusher = l3grid.NewBackgroundFlusher
