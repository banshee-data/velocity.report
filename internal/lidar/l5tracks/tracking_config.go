package l5tracks

import (
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
)

// TrackState represents the lifecycle state of a track.
type TrackState string

const (
	TrackTentative TrackState = "tentative" // New track, needs confirmation
	TrackConfirmed TrackState = "confirmed" // Stable track with sufficient history
	TrackDeleted   TrackState = "deleted"   // Track marked for removal
)

// HeadingSource indicates which mechanism determined the current track heading.
// This is exposed via the visualiser so that renderers can colour-code boxes
// to help diagnose angular drift (e.g. velocity-blue, PCA-yellow).
type HeadingSource int

const (
	HeadingSourcePCA          HeadingSource = 0 // Raw PCA heading (no disambiguation)
	HeadingSourceVelocity     HeadingSource = 1 // Disambiguated using Kalman velocity
	HeadingSourceDisplacement HeadingSource = 2 // Disambiguated using position displacement
	HeadingSourceLocked       HeadingSource = 3 // Heading locked (aspect ratio guard or jump rejection)
)

// TrackerConfig holds configuration parameters for the tracker.
type TrackerConfig struct {
	MaxTracks               int           // Maximum number of concurrent tracks
	MaxMisses               int           // Consecutive misses before tentative track deletion
	MaxMissesConfirmed      int           // Consecutive misses before confirmed track deletion (coasting)
	HitsToConfirm           int           // Consecutive hits needed for confirmation
	GatingDistanceSquared   float32       // Squared gating distance for association (metres²)
	ProcessNoisePos         float32       // Process noise for position (σ²)
	ProcessNoiseVel         float32       // Process noise for velocity (σ²)
	MeasurementNoise        float32       // Measurement noise (σ²)
	OcclusionCovInflation   float32       // Extra covariance inflation per occluded frame
	DeletedTrackGracePeriod time.Duration // How long to keep deleted tracks before cleanup

	// Kinematics/physics limits
	MaxReasonableSpeedMps float32 // Maximum reasonable speed (m/s; ~108 km/h at 30.0)
	MaxPositionJumpMetres float32 // Maximum position jump between observations (metres)
	MaxPredictDt          float32 // Maximum dt (seconds) per predict step
	MaxCovarianceDiag     float32 // Maximum covariance diagonal element

	// OBB heading params
	MinPointsForPCA             int     // Minimum cluster points for PCA heading
	OBBHeadingSmoothingAlpha    float32 // EMA smoothing factor for OBB heading [0,1]
	OBBAspectRatioLockThreshold float32 // Aspect ratio similarity below which heading is locked

	// History limits
	MaxTrackHistoryLength int // Maximum position trail length
	MaxSpeedHistoryLength int // Maximum speed history samples

	// Merge/split detection
	MergeSizeRatio float32 // Cluster area ratio above which → merge candidate
	SplitSizeRatio float32 // Cluster area ratio below which → split candidate

	// Classification
	MinObservationsForClassification int // Minimum observations before classification
}

// DefaultTrackerConfig returns tracker configuration loaded from the
// canonical tuning defaults file (config/tuning.defaults.json).
// Panics if the file cannot be found — intended for tests and binaries
// that have already validated config availability.
func DefaultTrackerConfig() TrackerConfig {
	cfg := config.MustLoadDefaultConfig()
	return TrackerConfigFromTuning(cfg.L5.CvKfV1)
}

// TrackerConfigFromTuning builds a TrackerConfig from the active L5 engine
// block. Callers are expected to pass the validated selected engine struct for
// the current pipeline on this branch.
func TrackerConfigFromTuning(l5cfg *config.L5CvKfV1) TrackerConfig {
	if l5cfg == nil {
		return TrackerConfig{}
	}
	return TrackerConfig{
		MaxTracks:                        l5cfg.MaxTracks,
		MaxMisses:                        l5cfg.MaxMisses,
		MaxMissesConfirmed:               l5cfg.MaxMissesConfirmed,
		HitsToConfirm:                    l5cfg.HitsToConfirm,
		GatingDistanceSquared:            float32(l5cfg.GatingDistanceSquared),
		ProcessNoisePos:                  float32(l5cfg.ProcessNoisePos),
		ProcessNoiseVel:                  float32(l5cfg.ProcessNoiseVel),
		MeasurementNoise:                 float32(l5cfg.MeasurementNoise),
		OcclusionCovInflation:            float32(l5cfg.OcclusionCovInflation),
		DeletedTrackGracePeriod:          mustParseDuration(l5cfg.DeletedTrackGracePeriod),
		MaxReasonableSpeedMps:            float32(l5cfg.MaxReasonableSpeedMps),
		MaxPositionJumpMetres:            float32(l5cfg.MaxPositionJumpMetres),
		MaxPredictDt:                     float32(l5cfg.MaxPredictDt),
		MaxCovarianceDiag:                float32(l5cfg.MaxCovarianceDiag),
		MinPointsForPCA:                  l5cfg.MinPointsForPCA,
		OBBHeadingSmoothingAlpha:         float32(l5cfg.OBBHeadingSmoothingAlpha),
		OBBAspectRatioLockThreshold:      float32(l5cfg.OBBAspectRatioLockThreshold),
		MaxTrackHistoryLength:            l5cfg.MaxTrackHistoryLength,
		MaxSpeedHistoryLength:            l5cfg.MaxSpeedHistoryLength,
		MergeSizeRatio:                   float32(l5cfg.MergeSizeRatio),
		SplitSizeRatio:                   float32(l5cfg.SplitSizeRatio),
		MinObservationsForClassification: l5cfg.MinObservationsForClassification,
	}
}

func mustParseDuration(raw string) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil {
		panic(fmt.Sprintf("mustParseDuration: invalid duration %q: %v", raw, err))
	}
	return d
}
