package l4perception

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases re-export ground removal types from the parent package.

// GroundRemover defines the interface for ground-plane filtering.
type GroundRemover = lidar.GroundRemover

// HeightBandFilter removes points outside a vertical band.
type HeightBandFilter = lidar.HeightBandFilter

// Constructor re-exports.

// NewHeightBandFilter creates a HeightBandFilter with explicit floor/ceiling.
var NewHeightBandFilter = lidar.NewHeightBandFilter

// DefaultHeightBandFilter returns a HeightBandFilter with production defaults.
var DefaultHeightBandFilter = lidar.DefaultHeightBandFilter
