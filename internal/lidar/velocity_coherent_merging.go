package lidar

import (
	"math"
	"sort"
)

// =============================================================================
// Phase 5: Track Fragment Merging
// =============================================================================

// MergeConfig controls fragment matching sensitivity.
type MergeConfig struct {
	// Maximum time gap between fragments to consider merging
	MaxTimeGapSeconds float64 // default 5.0

	// Maximum position error (predicted vs actual entry point)
	MaxPositionErrorMeters float64 // default 3.0

	// Maximum velocity difference at junction
	MaxVelocityDifferenceMs float64 // default 2.0

	// Minimum trajectory alignment score
	MinAlignmentScore float32 // default 0.7
}

// DefaultMergeConfig returns default fragment merge configuration.
func DefaultMergeConfig() MergeConfig {
	return MergeConfig{
		MaxTimeGapSeconds:       5.0,
		MaxPositionErrorMeters:  3.0,
		MaxVelocityDifferenceMs: 2.0,
		MinAlignmentScore:       0.7,
	}
}

// TrackFragment represents a potentially incomplete track segment.
type TrackFragment struct {
	Track *VelocityCoherentTrack

	// Entry/exit characteristics
	EntryPoint    [2]float32 // Position where track first appeared
	ExitPoint     [2]float32 // Position where track last appeared
	EntryVelocity [2]float32 // Velocity at entry
	ExitVelocity  [2]float32 // Velocity at exit

	// Temporal bounds
	StartNanos int64
	EndNanos   int64

	// Flags
	HasNaturalEntry bool // Started from sensor boundary (vs. appeared mid-field)
	HasNaturalExit  bool // Ended at sensor boundary (vs. disappeared mid-field)
}

// MergeCandidatePair represents two fragments that might be the same object.
type MergeCandidatePair struct {
	Earlier *TrackFragment
	Later   *TrackFragment

	// Matching scores
	PositionScore   float32 // How well predicted position matches
	VelocityScore   float32 // How well velocities align
	TrajectoryScore float32 // How well overall trajectory matches
	OverallScore    float32

	// Gap information
	GapSeconds float64
}

// SensorBoundary represents the sensor's field of view boundary.
type SensorBoundary struct {
	// Simple rectangular boundary for now
	MinX, MaxX float64
	MinY, MaxY float64

	// Margin for "near boundary" detection
	Margin float64
}

// DefaultSensorBoundary returns a default sensor boundary.
func DefaultSensorBoundary() SensorBoundary {
	return SensorBoundary{
		MinX:   -50.0,
		MaxX:   50.0,
		MinY:   -50.0,
		MaxY:   100.0, // Forward direction typically extends further
		Margin: 2.0,
	}
}

// IsNearBoundary checks if a point is near the sensor boundary.
func (sb *SensorBoundary) IsNearBoundary(x, y float32) bool {
	fx, fy := float64(x), float64(y)

	// Check if within margin of any boundary edge
	if fx <= sb.MinX+sb.Margin || fx >= sb.MaxX-sb.Margin {
		return true
	}
	if fy <= sb.MinY+sb.Margin || fy >= sb.MaxY-sb.Margin {
		return true
	}
	return false
}

// FragmentMerger handles detection and merging of track fragments.
type FragmentMerger struct {
	Config         MergeConfig
	SensorBoundary SensorBoundary
}

// NewFragmentMerger creates a new fragment merger.
func NewFragmentMerger(config MergeConfig, boundary SensorBoundary) *FragmentMerger {
	return &FragmentMerger{
		Config:         config,
		SensorBoundary: boundary,
	}
}

// DetectFragments identifies tracks that may be fragments of longer trajectories.
func (fm *FragmentMerger) DetectFragments(tracks []*VelocityCoherentTrack) []TrackFragment {
	fragments := make([]TrackFragment, 0, len(tracks))

	for _, track := range tracks {
		if len(track.History) < 2 {
			continue
		}

		entry := track.History[0]
		exit := track.History[len(track.History)-1]

		// Compute velocities at entry and exit
		var entryVX, entryVY, exitVX, exitVY float32

		if len(track.History) >= 2 {
			// Entry velocity from first two points
			dt := float64(track.History[1].Timestamp-track.History[0].Timestamp) / 1e9
			if dt > 0 {
				entryVX = (track.History[1].X - track.History[0].X) / float32(dt)
				entryVY = (track.History[1].Y - track.History[0].Y) / float32(dt)
			}

			// Exit velocity from last two points
			dtExit := float64(exit.Timestamp-track.History[len(track.History)-2].Timestamp) / 1e9
			if dtExit > 0 {
				exitVX = (exit.X - track.History[len(track.History)-2].X) / float32(dtExit)
				exitVY = (exit.Y - track.History[len(track.History)-2].Y) / float32(dtExit)
			}
		}

		fragment := TrackFragment{
			Track:         track,
			EntryPoint:    [2]float32{entry.X, entry.Y},
			ExitPoint:     [2]float32{exit.X, exit.Y},
			EntryVelocity: [2]float32{entryVX, entryVY},
			ExitVelocity:  [2]float32{exitVX, exitVY},
			StartNanos:    track.FirstUnixNanos,
			EndNanos:      track.LastUnixNanos,
		}

		// Check if entry/exit are at sensor boundary
		fragment.HasNaturalEntry = fm.SensorBoundary.IsNearBoundary(entry.X, entry.Y)
		fragment.HasNaturalExit = fm.SensorBoundary.IsNearBoundary(exit.X, exit.Y)

		fragments = append(fragments, fragment)
	}

	return fragments
}

// FindMergeCandidates identifies fragment pairs that may belong together.
func (fm *FragmentMerger) FindMergeCandidates(fragments []TrackFragment) []MergeCandidatePair {
	candidates := make([]MergeCandidatePair, 0)

	// Sort by start time
	sortedFragments := make([]TrackFragment, len(fragments))
	copy(sortedFragments, fragments)
	sort.Slice(sortedFragments, func(i, j int) bool {
		return sortedFragments[i].StartNanos < sortedFragments[j].StartNanos
	})

	for i := range sortedFragments {
		earlier := &sortedFragments[i]

		// Skip if natural exit (went to boundary - complete track)
		if earlier.HasNaturalExit {
			continue
		}

		for j := i + 1; j < len(sortedFragments); j++ {
			later := &sortedFragments[j]

			// Skip if natural entry (came from boundary - new track)
			if later.HasNaturalEntry {
				continue
			}

			// Check time gap
			gapSeconds := float64(later.StartNanos-earlier.EndNanos) / 1e9
			if gapSeconds < 0 || gapSeconds > fm.Config.MaxTimeGapSeconds {
				continue
			}

			// Predict where earlier track would be at later.StartNanos
			predictedX := earlier.ExitPoint[0] + earlier.ExitVelocity[0]*float32(gapSeconds)
			predictedY := earlier.ExitPoint[1] + earlier.ExitVelocity[1]*float32(gapSeconds)

			// Position error
			posError := math.Sqrt(
				math.Pow(float64(predictedX-later.EntryPoint[0]), 2) +
					math.Pow(float64(predictedY-later.EntryPoint[1]), 2),
			)

			if posError > fm.Config.MaxPositionErrorMeters {
				continue
			}

			// Velocity difference
			velDiff := math.Sqrt(
				math.Pow(float64(earlier.ExitVelocity[0]-later.EntryVelocity[0]), 2) +
					math.Pow(float64(earlier.ExitVelocity[1]-later.EntryVelocity[1]), 2),
			)

			if velDiff > fm.Config.MaxVelocityDifferenceMs {
				continue
			}

			// Compute scores
			posScore := float32(1.0 - posError/fm.Config.MaxPositionErrorMeters)
			velScore := float32(1.0 - velDiff/fm.Config.MaxVelocityDifferenceMs)
			trajectoryScore := fm.computeTrajectoryAlignment(earlier, later)

			overallScore := (posScore + velScore + trajectoryScore) / 3.0

			if overallScore >= fm.Config.MinAlignmentScore {
				candidates = append(candidates, MergeCandidatePair{
					Earlier:         earlier,
					Later:           later,
					PositionScore:   posScore,
					VelocityScore:   velScore,
					TrajectoryScore: trajectoryScore,
					OverallScore:    overallScore,
					GapSeconds:      gapSeconds,
				})
			}
		}
	}

	return candidates
}

// computeTrajectoryAlignment computes how well the trajectories align.
func (fm *FragmentMerger) computeTrajectoryAlignment(earlier, later *TrackFragment) float32 {
	// Compute heading vectors
	exitMag := math.Sqrt(
		math.Pow(float64(earlier.ExitVelocity[0]), 2) +
			math.Pow(float64(earlier.ExitVelocity[1]), 2),
	)
	entryMag := math.Sqrt(
		math.Pow(float64(later.EntryVelocity[0]), 2) +
			math.Pow(float64(later.EntryVelocity[1]), 2),
	)

	if exitMag < 0.1 || entryMag < 0.1 {
		return 0.5 // Cannot compute heading for stationary objects
	}

	// Normalized direction vectors
	exitDirX := float64(earlier.ExitVelocity[0]) / exitMag
	exitDirY := float64(earlier.ExitVelocity[1]) / exitMag
	entryDirX := float64(later.EntryVelocity[0]) / entryMag
	entryDirY := float64(later.EntryVelocity[1]) / entryMag

	// Dot product gives cosine of angle
	cosAngle := exitDirX*entryDirX + exitDirY*entryDirY

	// Convert to [0, 1] score (1.0 = same direction, 0.0 = opposite)
	return float32((cosAngle + 1.0) / 2.0)
}

// MergeFragments merges two track fragments into a single track.
func (fm *FragmentMerger) MergeFragments(
	earlier *TrackFragment,
	later *TrackFragment,
	gapSeconds float64,
) *VelocityCoherentTrack {
	// Create merged track using earlier's ID (maintains track identity)
	merged := &VelocityCoherentTrack{
		TrackID:  earlier.Track.TrackID,
		SensorID: earlier.Track.SensorID,
		State:    later.Track.State, // Use later track's current state

		// Lifecycle spans both fragments
		FirstUnixNanos: earlier.Track.FirstUnixNanos,
		LastUnixNanos:  later.Track.LastUnixNanos,

		// Position and velocity from later track (most recent)
		X:  later.Track.X,
		Y:  later.Track.Y,
		VX: later.Track.VX,
		VY: later.Track.VY,

		// Combine lifecycle counters
		Hits:             earlier.Track.Hits + later.Track.Hits,
		Misses:           later.Track.Misses,
		ObservationCount: earlier.Track.ObservationCount + later.Track.ObservationCount,

		// Velocity metrics (average)
		VelocityConfidence: (earlier.Track.VelocityConfidence + later.Track.VelocityConfidence) / 2,

		// Track sparse metrics
		MinPointsObserved: min(earlier.Track.MinPointsObserved, later.Track.MinPointsObserved),
		SparseFrameCount:  earlier.Track.SparseFrameCount + later.Track.SparseFrameCount,

		// Features (weighted average or take max)
		BoundingBoxLengthAvg: (earlier.Track.BoundingBoxLengthAvg + later.Track.BoundingBoxLengthAvg) / 2,
		BoundingBoxWidthAvg:  (earlier.Track.BoundingBoxWidthAvg + later.Track.BoundingBoxWidthAvg) / 2,
		BoundingBoxHeightAvg: (earlier.Track.BoundingBoxHeightAvg + later.Track.BoundingBoxHeightAvg) / 2,
		HeightP95Max:         max(earlier.Track.HeightP95Max, later.Track.HeightP95Max),
		IntensityMeanAvg:     (earlier.Track.IntensityMeanAvg + later.Track.IntensityMeanAvg) / 2,
		AvgSpeedMps:          (earlier.Track.AvgSpeedMps + later.Track.AvgSpeedMps) / 2,
		PeakSpeedMps:         max(earlier.Track.PeakSpeedMps, later.Track.PeakSpeedMps),

		// Classification (use higher confidence)
		ObjectClass:      earlier.Track.ObjectClass,
		ObjectConfidence: earlier.Track.ObjectConfidence,
	}

	if later.Track.ObjectConfidence > earlier.Track.ObjectConfidence {
		merged.ObjectClass = later.Track.ObjectClass
		merged.ObjectConfidence = later.Track.ObjectConfidence
	}

	// Merge histories
	merged.History = make([]TrackPoint, 0, len(earlier.Track.History)+len(later.Track.History))
	merged.History = append(merged.History, earlier.Track.History...)

	// Optionally interpolate gap points
	if gapSeconds > 0 && gapSeconds < 2.0 {
		// Add interpolated points during the gap
		gapPoints := interpolateGapPoints(
			earlier.ExitPoint, earlier.ExitVelocity,
			later.EntryPoint,
			earlier.EndNanos, later.StartNanos,
		)
		merged.History = append(merged.History, gapPoints...)
	}

	merged.History = append(merged.History, later.Track.History...)

	return merged
}

// interpolateGapPoints creates interpolated points to bridge a gap.
func interpolateGapPoints(
	exitPoint [2]float32,
	exitVelocity [2]float32,
	entryPoint [2]float32,
	startNanos, endNanos int64,
) []TrackPoint {
	gapNanos := endNanos - startNanos
	numPoints := int(gapNanos / 100_000_000) // One point per 100ms

	if numPoints <= 0 {
		return nil
	}

	points := make([]TrackPoint, numPoints)
	stepNanos := gapNanos / int64(numPoints+1)

	for i := 0; i < numPoints; i++ {
		t := float64(i+1) / float64(numPoints+1)

		// Linear interpolation between predicted exit and actual entry
		predX := exitPoint[0] + exitVelocity[0]*float32(float64(i+1)*0.1)
		predY := exitPoint[1] + exitVelocity[1]*float32(float64(i+1)*0.1)

		// Blend with actual entry point (more weight to actual as we approach)
		x := predX*(1-float32(t)) + entryPoint[0]*float32(t)
		y := predY*(1-float32(t)) + entryPoint[1]*float32(t)

		points[i] = TrackPoint{
			X:         x,
			Y:         y,
			Timestamp: startNanos + stepNanos*int64(i+1),
		}
	}

	return points
}
