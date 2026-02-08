package lidar

import (
	"encoding/binary"
	"time"
)

// ForegroundFrame represents a single frame of foreground points for ML training.
// Points are stored in sensor frame (polar coordinates) for pose independence.
type ForegroundFrame struct {
	SensorID         string       // Sensor that captured this frame
	TSUnixNanos      int64        // Timestamp of the frame
	SequenceID       string       // Optional sequence grouping (e.g., "seq_20251130_001")
	ForegroundPoints []PointPolar // Foreground points in polar (sensor) frame

	// Frame statistics
	TotalPoints      int
	BackgroundPoints int
}

// ExportForegroundFrame creates a ForegroundFrame from classified points.
// Points are stored in polar (sensor) coordinates for pose independence.
func ExportForegroundFrame(polarPoints []PointPolar, mask []bool, sensorID string, ts time.Time) *ForegroundFrame {
	foreground := ExtractForegroundPoints(polarPoints, mask)

	frame := &ForegroundFrame{
		SensorID:         sensorID,
		TSUnixNanos:      ts.UnixNano(),
		ForegroundPoints: foreground,
		TotalPoints:      len(polarPoints),
		BackgroundPoints: len(polarPoints) - len(foreground),
	}

	return frame
}

// SetSequenceID assigns this frame to a sequence for grouping.
func (f *ForegroundFrame) SetSequenceID(sequenceID string) {
	f.SequenceID = sequenceID
}

// ForegroundCount returns the number of foreground points.
func (f *ForegroundFrame) ForegroundCount() int {
	return len(f.ForegroundPoints)
}

// ForegroundFraction returns the ratio of foreground to total points.
func (f *ForegroundFrame) ForegroundFraction() float64 {
	if f.TotalPoints == 0 {
		return 0
	}
	return float64(len(f.ForegroundPoints)) / float64(f.TotalPoints)
}

// PolarPointCompact is a compact binary representation of a polar point.
// Total: 8 bytes per point (vs ~40+ bytes for PointPolar struct)
type PolarPointCompact struct {
	DistanceCm        uint16 // Distance in centimeters (0-655.35m range)
	AzimuthCentideg   uint16 // Azimuth in centidegrees (0-36000 = 0-360°)
	ElevationCentideg int16  // Elevation in centidegrees (-18000 to +18000 = -180° to +180°)
	Intensity         uint8  // Laser return intensity
	Ring              uint8  // Ring/channel number
}

// CompactPointSize is the size in bytes of a single compact point.
const CompactPointSize = 8

// EncodeForegroundBlob encodes foreground points to a compact binary blob.
// Format: Each point is 8 bytes: distance(2) + azimuth(2) + elevation(2) + intensity(1) + ring(1)
func EncodeForegroundBlob(points []PointPolar) []byte {
	blob := make([]byte, len(points)*CompactPointSize)

	for i, p := range points {
		offset := i * CompactPointSize

		// Distance in centimeters (0.01m precision)
		distCm := uint16(p.Distance * 100)
		binary.LittleEndian.PutUint16(blob[offset:], distCm)

		// Azimuth in centidegrees (0.01° precision)
		azCentideg := uint16(p.Azimuth * 100)
		binary.LittleEndian.PutUint16(blob[offset+2:], azCentideg)

		// Elevation in centidegrees (0.01° precision), signed
		elCentideg := int16(p.Elevation * 100)
		binary.LittleEndian.PutUint16(blob[offset+4:], uint16(elCentideg))

		// Intensity and Ring
		blob[offset+6] = p.Intensity
		blob[offset+7] = uint8(p.Channel)
	}

	return blob
}

// DecodeForegroundBlob decodes a compact binary blob back to polar points.
func DecodeForegroundBlob(blob []byte) []PointPolar {
	if len(blob)%CompactPointSize != 0 {
		return nil
	}

	numPoints := len(blob) / CompactPointSize

	// Limit maximum points to prevent excessive memory allocation from untrusted input.
	// At 40+ bytes per PointPolar struct, 1M points = ~40MB memory.
	const maxPoints = 1000000
	if numPoints > maxPoints || numPoints < 0 {
		return nil
	}

	points := make([]PointPolar, numPoints)

	for i := 0; i < numPoints; i++ {
		offset := i * CompactPointSize

		distCm := binary.LittleEndian.Uint16(blob[offset:])
		azCentideg := binary.LittleEndian.Uint16(blob[offset+2:])
		elCentideg := int16(binary.LittleEndian.Uint16(blob[offset+4:]))

		points[i] = PointPolar{
			Distance:  float64(distCm) / 100.0,
			Azimuth:   float64(azCentideg) / 100.0,
			Elevation: float64(elCentideg) / 100.0,
			Intensity: blob[offset+6],
			Channel:   int(blob[offset+7]),
		}
	}

	return points
}

// TrainingFrameMetadata contains metadata for a training frame without the point data.
// Useful for querying/filtering frames before loading full point clouds.
type TrainingFrameMetadata struct {
	FrameID          int64
	SensorID         string
	TSUnixNanos      int64
	SequenceID       string
	TotalPoints      int
	ForegroundPoints int
	BackgroundPoints int
	AnnotationStatus string // "unlabeled", "in_progress", "labeled"
}

// TrainingDataFilter defines criteria for filtering training frames.
type TrainingDataFilter struct {
	SensorID       string // Filter by sensor (empty = all)
	SequenceID     string // Filter by sequence (empty = all)
	MinForeground  int    // Minimum foreground points per frame
	AnnotationOnly bool   // Only return annotated frames
}

// DefaultTrainingDataFilter returns a filter suitable for high-quality training data.
func DefaultTrainingDataFilter() TrainingDataFilter {
	return TrainingDataFilter{
		MinForeground:  10, // At least 10 foreground points
		AnnotationOnly: false,
	}
}
