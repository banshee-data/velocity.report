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
	// HeadingSourceReleased marks the frame on which the rejection counter
	// forced a locked heading to release and snap to the measurement. It is a
	// distinct source rather than a counter so that the event survives into
	// the recorded stream: a VRLOG carries heading source per frame, and
	// without this a forced release is indistinguishable from an ordinary
	// unlocked frame on replay.
	HeadingSourceReleased HeadingSource = 4
	HeadingSourceAxis     HeadingSource = 5 // Supported axis interpretation, not directed body yaw
	// HeadingSourceAmbiguous means the aligned and swapped interpretations
	// scored too closely to separate. This is the genuine quarter-turn
	// ambiguity, and forcing a winner here is not an improvement.
	HeadingSourceAmbiguous    HeadingSource = 6
	HeadingSourceInsufficient HeadingSource = 7 // Missing or invalid geometry, or too few points
	// The two abstentions below were previously reported as ambiguous too.
	// They are separated because they call for different fixes: a near-square
	// observation carries no axis to recover, whereas an observation that
	// matches neither interpretation indicts the support reference.
	HeadingSourceAxisSquare HeadingSource = 8 // No distinguishable long axis
	HeadingSourceAxisNoFit  HeadingSource = 9 // Neither interpretation fits the support reference
	// HeadingSourceAxisReleased marks the frame on which a sustained run of
	// abstentions re-seeded the support reference and snapped the heading to
	// the observation. As with HeadingSourceReleased it is a source rather
	// than a counter so the event survives into a recording.
	HeadingSourceAxisReleased HeadingSource = 10
	// HeadingSourceAxisLowSupport marks a view showing too little of the
	// believed long axis to orient the box. Extent alone cannot refuse it — a
	// short span is consistent with any longer object — so this is the floor
	// that does.
	HeadingSourceAxisLowSupport HeadingSource = 11

	// HeadingSourceCount is the number of heading sources, for sizing
	// per-source counters. Keep it one past the last source above.
	HeadingSourceCount = 12
)

// IsLocked reports a decision that held the previous heading instead of
// accepting a measured one. The complement is the acceptance rate, which is
// the figure to compare across heading paths: the guard path and the axis
// path label their accepted frames differently, so held share alone is not
// like for like between them.
func (h HeadingSource) IsLocked() bool {
	switch h {
	case HeadingSourceLocked, HeadingSourceAmbiguous, HeadingSourceInsufficient,
		HeadingSourceAxisSquare, HeadingSourceAxisNoFit, HeadingSourceAxisLowSupport:
		return true
	}
	return false
}

// String names a heading source for diagnostics and JSON keys.
func (h HeadingSource) String() string {
	switch h {
	case HeadingSourcePCA:
		return "pca"
	case HeadingSourceVelocity:
		return "velocity"
	case HeadingSourceDisplacement:
		return "displacement"
	case HeadingSourceLocked:
		return "locked"
	case HeadingSourceReleased:
		return "released"
	case HeadingSourceAxis:
		return "axis"
	case HeadingSourceAmbiguous:
		return "ambiguous"
	case HeadingSourceInsufficient:
		return "insufficient"
	case HeadingSourceAxisSquare:
		return "axis_square"
	case HeadingSourceAxisNoFit:
		return "axis_no_fit"
	case HeadingSourceAxisReleased:
		return "axis_released"
	case HeadingSourceAxisLowSupport:
		return "axis_low_support"
	default:
		return "unknown"
	}
}

// TrackerConfig holds configuration parameters for the tracker.
type TrackerConfig struct {
	MaxTracks             int     // Maximum number of concurrent tracks
	MaxMisses             int     // Consecutive misses before tentative track deletion
	MaxMissesConfirmed    int     // Consecutive misses before confirmed track deletion (coasting)
	HitsToConfirm         int     // Consecutive hits needed for confirmation
	GatingDistanceSquared float32 // Squared gating distance for association (metres²)
	ProcessNoisePos       float32 // Process noise for position (σ²)
	ProcessNoiseVel       float32 // Process noise for velocity (σ²)
	MeasurementNoise      float32 // Measurement noise (σ²)

	// CoupledProcessNoise switches the prediction step from the shipped
	// diagonal Q to the continuous white-noise-acceleration form, which adds
	// the position-velocity cross terms the diagonal form omits (gap K1).
	//
	// JosephCovarianceUpdate switches the posterior covariance from
	// P' = (I-KH)P to the Joseph stabilised form, which stays symmetric under
	// float error rather than only starting that way (gap K2).
	//
	// Both are Go-level options with the shipped behaviour as the default, and
	// deliberately not tuning keys: TuningConfig.Fingerprint hashes the whole
	// resolved config, so adding keys would make the committed perf baselines
	// refuse to compare before anyone had measured whether the change helps.
	CoupledProcessNoise    bool
	JosephCovarianceUpdate bool

	// LikelihoodAssociationCost changes the assignment cost from the bare
	// squared Mahalanobis distance d² to the Gaussian negative log-likelihood
	// d² + ln|S| (gap analysis S3). With d² alone, a track whose innovation
	// covariance S is large pays less for the same miss, so a track coasting
	// through a missed frame outbids a freshly updated one for the same
	// cluster, and OcclusionCovInflation widens that discount on purpose. The
	// log-determinant charges a vague track for its vagueness. The gate is
	// unchanged: it stays on d². Default false, and not a tuning key, for the
	// same fingerprint reason as the two options above.
	LikelihoodAssociationCost bool

	// CascadedAssociation matches confirmed tracks to clusters first and
	// offers only the clusters they leave to tentative tracks (gap analysis
	// S2, after DeepSORT's final stage; it does not address S3, whose two
	// bidders are both confirmed). With one joint assignment a
	// tentative track a frame old can outbid a confirmed track coasting
	// through one miss for the same cluster, because the assignment sees two
	// costs and no history. Default false: the campaign's ground-truth and
	// label-free harnesses measure it against the shipped behaviour first.
	CascadedAssociation bool
	// MeasurementSourceMode selects the position model. Empty means the
	// production medoid; obb_centre_v1 opts into D2's candidate.
	MeasurementSourceMode   MeasurementSource
	OcclusionCovInflation   float32       // Extra covariance inflation per occluded frame
	DeletedTrackGracePeriod time.Duration // How long to keep deleted tracks before cleanup

	// Capture-time options. Every one is default-off and, like the options
	// above, deliberately not a tuning key: the shipped estimator's temporal
	// behaviour is pinned by replay before any of it is tuned. See
	// time_domain.go for the boundary they sit inside.
	//
	// MaxCoastSecsTentative and MaxCoastSecsConfirmed bound how long, in
	// capture time, a track may go without an accepted observation. Zero
	// disables the bound, leaving the frame-count rule (MaxMisses,
	// MaxMissesConfirmed) as the only expiry. The two rules measure different
	// things: misses count frames that reached the tracker, so a frame that
	// was throttled, lost in transport or dropped at a capture join extends a
	// coasting track's life for free; the capture-time bound does not care
	// how many frames arrived. The bound is checked before association, so an
	// observation arriving after it has lapsed seeds a new track rather than
	// reviving a hypothesis nothing supported in the interval.
	MaxCoastSecsTentative float32
	MaxCoastSecsConfirmed float32

	// CaptureGapPrediction predicts across the whole capture-time gap
	// between frames, in steps of at most MaxPredictDt, instead of clamping
	// the step to MaxPredictDt. The clamp was written for throttle-sized gaps;
	// across a longer transport gap it predicts a moving object a fraction of
	// the distance it travelled, so reacquisition compares the returning
	// cluster against a stale position. Sub-stepping keeps the covariance cap
	// applying per step exactly as it does frame to frame. Default false.
	CaptureGapPrediction bool

	// MeasurementTimePrediction predicts each associated track to its
	// measurement's own acquisition time (WorldCluster.TSUnixNanos) before the
	// update, instead of treating every cluster as observed at the frame's
	// start. A rotation takes about 100 ms, so an object near the end of the
	// sweep is measured up to one frame period after the time the filter
	// assumes, and near the azimuth wrap that offset changes abruptly between
	// consecutive frames. This is state-estimation plan question Q3. Gating
	// and assignment still use the frame-time prediction; only the update is
	// moved. Default false.
	MeasurementTimePrediction bool

	// Kinematics/physics limits
	MaxReasonableSpeedMps float32 // Maximum reasonable speed (m/s; ~108 km/h at 30.0)
	MaxPositionJumpMetres float32 // Maximum position jump between observations (metres)
	MaxPredictDt          float32 // Maximum dt (seconds) per predict step
	MaxCovarianceDiag     float32 // Maximum covariance diagonal element

	// OBB heading params
	MinPointsForPCA             int     // Minimum cluster points for PCA heading
	OBBHeadingSmoothingAlpha    float32 // EMA smoothing factor for OBB heading [0,1]
	OBBAspectRatioLockThreshold float32 // Aspect ratio similarity below which heading is locked
	OBBHeadingLockMaxRejections int     // Consecutive Guard 3 rejections before the lock releases (0 = never)
	OBBAxisCoherenceEnabled     bool    // Experimental axis selection and coherent observed envelope
	// OBBHeadingFlipRule applies AB3DMOT's orientation correction before
	// Guard 3: a PCA heading more than 90 degrees from the track's smoothed
	// heading is flipped by 180 degrees, on the grounds that a body cannot
	// reverse its orientation within one frame (gap analysis P3). It is
	// stated without reference to velocity, so it also disambiguates a
	// stationary object, where the velocity and displacement resolvers have
	// nothing to work with. Default false; measured before it ships.
	OBBHeadingFlipRule bool

	// MinAssociableExtentMetres is the smallest cluster extent that may be
	// associated with a metre-scale track. 0 disables the fragment guard.
	MinAssociableExtentMetres float32

	// AssociationExtentCostWeight scales the bounded extent-compatibility term
	// in the association cost. 0 keeps the hard fragment guard instead.
	AssociationExtentCostWeight float32

	// DeletedTrackRenderFade is how long a deleted track is still published to
	// clients, fading out. Separate from DeletedTrackGracePeriod, which governs
	// internal re-association.
	DeletedTrackRenderFade time.Duration

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
		OBBHeadingLockMaxRejections:      l5cfg.OBBHeadingLockMaxRejections,
		OBBAxisCoherenceEnabled:          l5cfg.OBBAxisCoherenceEnabled,
		MinAssociableExtentMetres:        float32(l5cfg.MinAssociableExtentMetres),
		AssociationExtentCostWeight:      float32(l5cfg.AssociationExtentCostWeight),
		DeletedTrackRenderFade:           mustParseDuration(l5cfg.DeletedTrackRenderFade),
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
