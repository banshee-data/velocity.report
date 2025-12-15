package lidar

import (
	"math"
	"time"
)

// =============================================================================
// Phase 3: Long-Tail Track Management
// =============================================================================

// TrackStateVC represents the lifecycle state of a velocity-coherent track.
type TrackStateVC string

const (
	TrackPreTail   TrackStateVC = "pre_tail"  // Sparse early observations before full detection
	TrackTentVC    TrackStateVC = "tentative" // New track, needs confirmation
	TrackConfirmVC TrackStateVC = "confirmed" // Stable track with sufficient history
	TrackPostTail  TrackStateVC = "post_tail" // Track in prediction-only mode (exiting)
	TrackDeletedVC TrackStateVC = "deleted"   // Track marked for removal
)

// PreTailConfig controls pre-entry track detection.
type PreTailConfig struct {
	// Frames to look back for sparse early observations
	MaxPreTailFrames int // default 10

	// Minimum velocity confidence for pre-tail detection
	MinVelocityConfidence float32 // default 0.5

	// Minimum velocity match score to associate pre-tail cluster
	MinVelocityMatchScore float32 // default 0.7

	// Entry zone prediction radius growth per frame
	UncertaintyGrowthPerFrame float64 // meters, default 0.3

	// Boundary margin for detecting entry points
	BoundaryMarginMeters float64 // default 2.0
}

// DefaultPreTailConfig returns default pre-tail detection configuration.
func DefaultPreTailConfig() PreTailConfig {
	return PreTailConfig{
		MaxPreTailFrames:          10,
		MinVelocityConfidence:     0.5,
		MinVelocityMatchScore:     0.7,
		UncertaintyGrowthPerFrame: 0.3,
		BoundaryMarginMeters:      2.0,
	}
}

// PostTailConfig controls post-exit track continuation.
type PostTailConfig struct {
	// Maximum frames to continue prediction after last observation
	MaxPredictionFrames int // default 30 (3 seconds at 10 Hz)

	// Maximum uncertainty growth before abandoning track
	MaxUncertaintyRadius float64 // meters, default 10.0

	// Minimum confidence to recover a predicted track
	MinRecoveryConfidence float32 // default 0.5

	// Base uncertainty at start of prediction
	BaseUncertainty float64 // meters, default 0.5

	// Uncertainty growth per frame
	UncertaintyGrowthPerFrame float64 // meters, default 0.2
}

// DefaultPostTailConfig returns default post-tail configuration.
func DefaultPostTailConfig() PostTailConfig {
	return PostTailConfig{
		MaxPredictionFrames:       30,
		MaxUncertaintyRadius:      10.0,
		MinRecoveryConfidence:     0.5,
		BaseUncertainty:           0.5,
		UncertaintyGrowthPerFrame: 0.2,
	}
}

// PredictedPosition represents where a track is expected to be.
type PredictedPosition struct {
	TrackID           string
	PredictedX        float32
	PredictedY        float32
	VelocityX         float32
	VelocityY         float32
	UncertaintyRadius float32
	FramesSinceLast   int
	PredictionTime    time.Time
}

// PredictedEntryZone represents an area where objects are expected to appear.
type PredictedEntryZone struct {
	// Expected position (extrapolated from velocity)
	PredictedX, PredictedY float64

	// Velocity vector
	VelocityX, VelocityY float64

	// Uncertainty radius (grows with time since last observation)
	UncertaintyRadius float64

	// Source track (tentative or previous observation)
	SourceTrackID string

	// Time of prediction
	PredictionTimeNanos int64

	// Frames since last observation
	FramesSinceObservation int
}

// TrackAssociationType indicates the type of track-cluster association.
type TrackAssociationType string

const (
	AssociationNormal   TrackAssociationType = "normal"
	AssociationPreTail  TrackAssociationType = "pre_tail"
	AssociationRecovery TrackAssociationType = "recovery"
)

// TrackAssociation represents a match between a cluster and a track.
type TrackAssociation struct {
	ClusterID  int64
	TrackID    string
	Type       TrackAssociationType
	Confidence float32
}

// LongTailManager handles pre-tail and post-tail track management.
type LongTailManager struct {
	PreTailConfig  PreTailConfig
	PostTailConfig PostTailConfig

	// Active predicted positions for post-tail tracks
	predictions map[string]*PredictedPosition

	// Entry zones for pre-tail detection
	entryZones []PredictedEntryZone
}

// NewLongTailManager creates a new long-tail track manager.
func NewLongTailManager(preTail PreTailConfig, postTail PostTailConfig) *LongTailManager {
	return &LongTailManager{
		PreTailConfig:  preTail,
		PostTailConfig: postTail,
		predictions:    make(map[string]*PredictedPosition),
		entryZones:     make([]PredictedEntryZone, 0),
	}
}

// UpdatePredictions updates position predictions for tracks in post-tail state.
func (m *LongTailManager) UpdatePredictions(
	tracks map[string]*VelocityCoherentTrack,
	currentTime time.Time,
) []*PredictedPosition {
	nowNanos := currentTime.UnixNano()
	activePredictions := make([]*PredictedPosition, 0)

	for trackID, track := range tracks {
		if track.State != TrackPostTail {
			// Remove from predictions if track is no longer in post-tail
			delete(m.predictions, trackID)
			continue
		}

		// Compute frames since last observation
		framesSinceLast := int((nowNanos - track.LastUnixNanos) / 100_000_000) // 100ms per frame

		if framesSinceLast > m.PostTailConfig.MaxPredictionFrames {
			// Too long since last observation - mark for deletion
			delete(m.predictions, trackID)
			continue
		}

		// Predict current position using velocity
		dt := float64(nowNanos-track.LastUnixNanos) / 1e9 // seconds
		predictedX := track.X + track.VX*float32(dt)
		predictedY := track.Y + track.VY*float32(dt)

		// Grow uncertainty with time
		uncertaintyRadius := m.PostTailConfig.BaseUncertainty +
			float64(framesSinceLast)*m.PostTailConfig.UncertaintyGrowthPerFrame

		if uncertaintyRadius > m.PostTailConfig.MaxUncertaintyRadius {
			delete(m.predictions, trackID)
			continue
		}

		prediction := &PredictedPosition{
			TrackID:           trackID,
			PredictedX:        predictedX,
			PredictedY:        predictedY,
			VelocityX:         track.VX,
			VelocityY:         track.VY,
			UncertaintyRadius: float32(uncertaintyRadius),
			FramesSinceLast:   framesSinceLast,
			PredictionTime:    currentTime,
		}

		m.predictions[trackID] = prediction
		activePredictions = append(activePredictions, prediction)
	}

	return activePredictions
}

// TryRecoverTrack attempts to associate a cluster with a predicted track position.
func (m *LongTailManager) TryRecoverTrack(
	cluster VelocityCoherentCluster,
) *TrackAssociation {
	for trackID, pred := range m.predictions {
		// Check if cluster centroid is within uncertainty radius
		dx := float64(cluster.CentroidX) - float64(pred.PredictedX)
		dy := float64(cluster.CentroidY) - float64(pred.PredictedY)
		dist := math.Sqrt(dx*dx + dy*dy)

		if dist > float64(pred.UncertaintyRadius) {
			continue
		}

		// Check velocity consistency
		velDiffX := cluster.VelocityX - float64(pred.VelocityX)
		velDiffY := cluster.VelocityY - float64(pred.VelocityY)
		velDiff := math.Sqrt(velDiffX*velDiffX + velDiffY*velDiffY)

		// Allow larger velocity difference for recovery
		maxVelDiff := 3.0 // m/s
		if velDiff > maxVelDiff {
			continue
		}

		// Compute confidence based on position and velocity match
		posScore := 1.0 - dist/float64(pred.UncertaintyRadius)
		velScore := 1.0 - velDiff/maxVelDiff
		confidence := float32(posScore * velScore * 0.8) // Scale down for recovery

		if confidence >= m.PostTailConfig.MinRecoveryConfidence {
			return &TrackAssociation{
				ClusterID:  cluster.ClusterID,
				TrackID:    trackID,
				Type:       AssociationRecovery,
				Confidence: confidence,
			}
		}
	}

	return nil
}

// =============================================================================
// Phase 4: Sparse Continuation Logic
// =============================================================================

// SparseTrackConfig controls sparse track validation.
type SparseTrackConfig struct {
	// Absolute minimum points to maintain a track
	MinPointsAbsolute int // default 3

	// Minimum velocity confidence for sparse tracks
	MinVelocityConfidenceForSparse float32 // default 0.6

	// Maximum velocity variance for sparse tracks
	MaxVelocityVarianceForSparse float64 // m/s, default 0.5

	// Spatial coherence threshold
	MaxSpatialSpreadForSparse float64 // meters, default 2.0
}

// DefaultSparseTrackConfig returns default sparse track configuration.
func DefaultSparseTrackConfig() SparseTrackConfig {
	return SparseTrackConfig{
		MinPointsAbsolute:              3,
		MinVelocityConfidenceForSparse: 0.6,
		MaxVelocityVarianceForSparse:   0.5,
		MaxSpatialSpreadForSparse:      2.0,
	}
}

// VelocityCoherentTrack represents a tracked object using velocity-coherent methods.
type VelocityCoherentTrack struct {
	TrackID  string
	SensorID string
	State    TrackStateVC

	// Lifecycle counters
	Hits   int
	Misses int

	// Timestamps
	FirstUnixNanos int64
	LastUnixNanos  int64

	// Position and velocity (world frame)
	X  float32
	Y  float32
	VX float32
	VY float32

	// Velocity confidence history
	VelocityConfidence  float32
	VelocityConsistency float32 // How stable velocity has been
	MinPointsObserved   int     // Minimum points seen in any observation
	SparseFrameCount    int     // Number of frames with <12 points
	ObservationCount    int

	// Aggregated features
	BoundingBoxLengthAvg float32
	BoundingBoxWidthAvg  float32
	BoundingBoxHeightAvg float32
	HeightP95Max         float32
	IntensityMeanAvg     float32
	AvgSpeedMps          float32
	PeakSpeedMps         float32

	// History for quality tracking
	History []TrackPoint

	// Classification
	ObjectClass      string
	ObjectConfidence float32
}

// IsSparseTrackValid checks if a sparse cluster can maintain track identity.
func IsSparseTrackValid(
	cluster VelocityCoherentCluster,
	existingTrack *VelocityCoherentTrack,
	config SparseTrackConfig,
) (bool, float32) {
	// Minimum point count
	if cluster.PointCount < config.MinPointsAbsolute {
		return false, 0
	}

	// Velocity confidence threshold
	if cluster.VelocityConfidence < config.MinVelocityConfidenceForSparse {
		return false, 0
	}

	// Velocity must match existing track
	velDiff := math.Sqrt(
		math.Pow(cluster.VelocityX-float64(existingTrack.VX), 2) +
			math.Pow(cluster.VelocityY-float64(existingTrack.VY), 2),
	)

	if velDiff > config.MaxVelocityVarianceForSparse {
		return false, 0
	}

	// Spatial spread must be reasonable
	maxDim := cluster.BoundingBoxLength
	if cluster.BoundingBoxWidth > maxDim {
		maxDim = cluster.BoundingBoxWidth
	}
	if float64(maxDim) > config.MaxSpatialSpreadForSparse {
		return false, 0
	}

	// Compute confidence score
	velocityMatchScore := 1.0 - velDiff/config.MaxVelocityVarianceForSparse
	pointScore := float64(cluster.PointCount) / 10.0 // Scale 3-10 points to 0.3-1.0
	if pointScore > 1.0 {
		pointScore = 1.0
	}

	confidence := float32(velocityMatchScore * pointScore * float64(cluster.VelocityConfidence))

	return true, confidence
}

// AdaptiveTolerances returns velocity and spatial tolerances based on point count.
// As point count decreases, we tighten constraints (require better velocity match).
func AdaptiveTolerances(pointCount int) (velTol, spatialTol float64) {
	switch {
	case pointCount >= 12:
		return 2.0, 1.0 // Standard DBSCAN tolerance
	case pointCount >= 6:
		return 1.5, 0.8 // Reduced tolerance
	case pointCount >= 3:
		return 0.5, 0.5 // Strict velocity match required
	default:
		return 0, 0 // Cannot track with <3 points
	}
}
