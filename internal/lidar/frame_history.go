package lidar

import (
	"time"
)

// PointWithVelocity extends a point with estimated velocity.
type PointWithVelocity struct {
	// Position in world coordinates
	X, Y, Z float64

	// Original polar coordinates (for reference)
	Distance  float64
	Azimuth   float64
	Elevation float64
	Channel   int

	// Estimated velocity (m/s)
	VX, VY float64

	// Velocity confidence [0, 1]
	Confidence float32

	// Index of corresponding point in previous frame (-1 if none)
	CorrespondenceIdx int

	// Original point data
	Intensity uint8
	Timestamp int64
}

// VelocityFrame is a processed frame with spatial index for correspondence matching.
type VelocityFrame struct {
	Points       []PointWithVelocity
	SpatialIndex *SpatialIndex
	Timestamp    time.Time
	FrameID      string
}

// FrameHistory maintains a sliding window of frames for multi-frame correspondence.
type FrameHistory struct {
	frames   []*VelocityFrame
	capacity int
	head     int // Points to next write position
	size     int // Current number of frames stored
}

// NewFrameHistory creates a new frame history buffer with the specified capacity.
func NewFrameHistory(capacity int) *FrameHistory {
	if capacity < 1 {
		capacity = 10 // Default
	}
	return &FrameHistory{
		frames:   make([]*VelocityFrame, capacity),
		capacity: capacity,
		head:     0,
		size:     0,
	}
}

// Add stores a new frame in the history, overwriting the oldest if at capacity.
func (fh *FrameHistory) Add(frame *VelocityFrame) {
	fh.frames[fh.head] = frame
	fh.head = (fh.head + 1) % fh.capacity
	if fh.size < fh.capacity {
		fh.size++
	}
}

// Previous returns the frame N steps back from the most recent.
// Previous(1) returns the most recently added frame.
// Previous(2) returns the one before that.
// Returns nil if the requested frame doesn't exist.
func (fh *FrameHistory) Previous(n int) *VelocityFrame {
	if n < 1 || n > fh.size {
		return nil
	}

	// Calculate index: head-1 is most recent, head-2 is one before, etc.
	idx := (fh.head - n + fh.capacity) % fh.capacity
	return fh.frames[idx]
}

// Size returns the current number of frames in history.
func (fh *FrameHistory) Size() int {
	return fh.size
}

// Capacity returns the maximum number of frames that can be stored.
func (fh *FrameHistory) Capacity() int {
	return fh.capacity
}

// Clear removes all frames from history.
func (fh *FrameHistory) Clear() {
	for i := range fh.frames {
		fh.frames[i] = nil
	}
	fh.head = 0
	fh.size = 0
}

// GetAll returns all frames in history from oldest to newest.
func (fh *FrameHistory) GetAll() []*VelocityFrame {
	if fh.size == 0 {
		return nil
	}

	result := make([]*VelocityFrame, fh.size)
	for i := 0; i < fh.size; i++ {
		// Start from oldest
		idx := (fh.head - fh.size + i + fh.capacity) % fh.capacity
		result[i] = fh.frames[idx]
	}
	return result
}

// TimeDeltaSeconds returns the time delta between the two most recent frames.
// Returns 0 if fewer than 2 frames are available.
func (fh *FrameHistory) TimeDeltaSeconds() float64 {
	if fh.size < 2 {
		return 0
	}

	current := fh.Previous(1)
	previous := fh.Previous(2)

	if current == nil || previous == nil {
		return 0
	}

	return current.Timestamp.Sub(previous.Timestamp).Seconds()
}

// BuildWorldPointsWithVelocity converts world points to PointWithVelocity
// with initial zero velocity (to be filled by correspondence matching).
func BuildWorldPointsWithVelocity(worldPoints []WorldPoint, polar []PointPolar) []PointWithVelocity {
	if len(worldPoints) == 0 {
		return nil
	}

	points := make([]PointWithVelocity, len(worldPoints))
	for i, wp := range worldPoints {
		points[i] = PointWithVelocity{
			X:                 wp.X,
			Y:                 wp.Y,
			Z:                 wp.Z,
			VX:                0,
			VY:                0,
			Confidence:        0,
			CorrespondenceIdx: -1,
			Intensity:         wp.Intensity,
			Timestamp:         wp.Timestamp.UnixNano(),
		}
		// Copy polar data if available
		if i < len(polar) {
			points[i].Distance = polar[i].Distance
			points[i].Azimuth = polar[i].Azimuth
			points[i].Elevation = polar[i].Elevation
			points[i].Channel = polar[i].Channel
		}
	}
	return points
}

// NewVelocityFrame creates a new velocity frame from points.
// Builds a spatial index for efficient correspondence matching.
func NewVelocityFrame(points []PointWithVelocity, timestamp time.Time, frameID string, cellSize float64) *VelocityFrame {
	// Convert to WorldPoints for spatial indexing
	worldPoints := make([]WorldPoint, len(points))
	for i, p := range points {
		worldPoints[i] = WorldPoint{
			X:         p.X,
			Y:         p.Y,
			Z:         p.Z,
			Intensity: p.Intensity,
			Timestamp: time.Unix(0, p.Timestamp),
		}
	}

	// Build spatial index
	si := NewSpatialIndex(cellSize)
	si.Build(worldPoints)

	return &VelocityFrame{
		Points:       points,
		SpatialIndex: si,
		Timestamp:    timestamp,
		FrameID:      frameID,
	}
}
