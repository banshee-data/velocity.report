package lidar

import "github.com/banshee-data/velocity.report/internal/lidar/l4perception"

// Type aliases for ground filtering types migrated to l4perception.
// These maintain backward compatibility for existing code that imports from internal/lidar.

// GroundRemover is an alias for l4perception.GroundRemover.
type GroundRemover = l4perception.GroundRemover

// HeightBandFilter is an alias for l4perception.HeightBandFilter.
type HeightBandFilter = l4perception.HeightBandFilter

// NewHeightBandFilter is an alias for l4perception.NewHeightBandFilter.
var NewHeightBandFilter = l4perception.NewHeightBandFilter

// DefaultHeightBandFilter is an alias for l4perception.DefaultHeightBandFilter.
var DefaultHeightBandFilter = l4perception.DefaultHeightBandFilter
