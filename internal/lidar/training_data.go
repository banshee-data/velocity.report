package lidar

import (
	"github.com/banshee-data/velocity.report/internal/lidar/adapters"
)

// Backward-compatible type aliases — canonical implementation is in adapters/.

// ForegroundFrame represents a single frame of foreground points for ML training.
type ForegroundFrame = adapters.ForegroundFrame

// ExportForegroundFrame creates a ForegroundFrame from classified points.
var ExportForegroundFrame = adapters.ExportForegroundFrame

// PolarPointCompact is a compact binary representation of a polar point.
type PolarPointCompact = adapters.PolarPointCompact

// CompactPointSize is the size in bytes of a single compact point.
const CompactPointSize = adapters.CompactPointSize

// EncodeForegroundBlob encodes foreground points to a compact binary blob.
var EncodeForegroundBlob = adapters.EncodeForegroundBlob

// DecodeForegroundBlob decodes a compact binary blob back to polar points.
var DecodeForegroundBlob = adapters.DecodeForegroundBlob

// TrainingFrameMetadata contains metadata for a training frame.
type TrainingFrameMetadata = adapters.TrainingFrameMetadata

// TrainingDataFilter defines criteria for filtering training frames.
type TrainingDataFilter = adapters.TrainingDataFilter

// DefaultTrainingDataFilter returns a filter suitable for high-quality training data.
var DefaultTrainingDataFilter = adapters.DefaultTrainingDataFilter
