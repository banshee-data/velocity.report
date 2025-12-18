package lidar

import (
	"encoding/json"
	"sync"
	"time"
)

// TrackingAlgorithm identifies which tracking algorithm to use.
type TrackingAlgorithm string

const (
	// AlgorithmBackgroundSubtraction uses the existing background subtraction + DBSCAN approach
	AlgorithmBackgroundSubtraction TrackingAlgorithm = "background_subtraction"

	// AlgorithmVelocityCoherent uses the new velocity-coherent 6D clustering approach
	AlgorithmVelocityCoherent TrackingAlgorithm = "velocity_coherent"

	// AlgorithmDual runs both algorithms in parallel for comparison
	AlgorithmDual TrackingAlgorithm = "dual"
)

// String returns the string representation of the algorithm.
func (a TrackingAlgorithm) String() string {
	return string(a)
}

// IsValid returns true if the algorithm is a known valid value.
func (a TrackingAlgorithm) IsValid() bool {
	switch a {
	case AlgorithmBackgroundSubtraction, AlgorithmVelocityCoherent, AlgorithmDual:
		return true
	default:
		return false
	}
}

// DualPipelineConfig holds configuration for the dual extraction pipeline.
type DualPipelineConfig struct {
	// Active algorithm selection
	ActiveAlgorithm TrackingAlgorithm `json:"active_algorithm"`

	// Background subtraction configuration
	BackgroundSubtractionEnabled bool          `json:"background_subtraction_enabled"`
	DBSCANParams                 DBSCANParams  `json:"dbscan_params"`
	TrackerConfig                TrackerConfig `json:"tracker_config"`

	// Velocity coherent configuration
	VelocityCoherentEnabled bool                          `json:"velocity_coherent_enabled"`
	VCConfig                VelocityCoherentTrackerConfig `json:"vc_config"`
}

// DefaultDualPipelineConfig returns default configuration.
func DefaultDualPipelineConfig() DualPipelineConfig {
	return DualPipelineConfig{
		ActiveAlgorithm:              AlgorithmBackgroundSubtraction,
		BackgroundSubtractionEnabled: true,
		DBSCANParams:                 DefaultDBSCANParams(),
		TrackerConfig:                DefaultTrackerConfig(),
		VelocityCoherentEnabled:      false,
		VCConfig:                     DefaultVelocityCoherentTrackerConfig(),
	}
}

// DualExtractionPipeline runs both tracking algorithms in parallel.
// This allows comparison between the existing background-subtraction approach
// and the new velocity-coherent approach.
type DualExtractionPipeline struct {
	config DualPipelineConfig

	// Background subtraction tracker (existing)
	bgTracker *Tracker

	// Velocity coherent tracker (new)
	vcTracker *VelocityCoherentTracker

	// Callbacks for track events
	onBGTrackCreated   func(*TrackedObject)
	onBGTrackCompleted func(*TrackedObject)
	onVCTrackCreated   func(*VelocityCoherentTrack)
	onVCTrackCompleted func(*VelocityCoherentTrack)

	// Statistics
	stats PipelineStats

	mu sync.RWMutex
}

// PipelineStats tracks statistics for both algorithms.
type PipelineStats struct {
	// Background subtraction stats
	BGFramesProcessed  int64   `json:"bg_frames_processed"`
	BGClustersTotal    int64   `json:"bg_clusters_total"`
	BGTracksCreated    int64   `json:"bg_tracks_created"`
	BGTracksConfirmed  int64   `json:"bg_tracks_confirmed"`
	BGTracksCompleted  int64   `json:"bg_tracks_completed"`
	BGAvgClustersFrame float64 `json:"bg_avg_clusters_frame"`

	// Velocity coherent stats
	VCFramesProcessed  int64   `json:"vc_frames_processed"`
	VCClustersTotal    int64   `json:"vc_clusters_total"`
	VCTracksCreated    int64   `json:"vc_tracks_created"`
	VCTracksConfirmed  int64   `json:"vc_tracks_confirmed"`
	VCTracksCompleted  int64   `json:"vc_tracks_completed"`
	VCAvgClustersFrame float64 `json:"vc_avg_clusters_frame"`

	// Comparison
	LastComparisonTime time.Time `json:"last_comparison_time"`
}

// NewDualExtractionPipeline creates a new dual pipeline with the given configuration.
func NewDualExtractionPipeline(config DualPipelineConfig) *DualExtractionPipeline {
	return &DualExtractionPipeline{
		config:    config,
		bgTracker: NewTracker(config.TrackerConfig),
		vcTracker: NewVelocityCoherentTracker(config.VCConfig),
	}
}

// ProcessFrame processes a frame through the active algorithm(s).
// For background subtraction, it expects clusters from DBSCAN.
// For velocity coherent, it expects raw world points.
func (p *DualExtractionPipeline) ProcessFrame(
	worldPoints []WorldPoint,
	clusters []WorldCluster, // Pre-computed clusters for BG algorithm
	timestamp time.Time,
	sensorID string,
) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.config.ActiveAlgorithm {
	case AlgorithmBackgroundSubtraction:
		p.processBGFrame(clusters, timestamp)
	case AlgorithmVelocityCoherent:
		p.processVCFrame(worldPoints, timestamp, sensorID)
	case AlgorithmDual:
		p.processBGFrame(clusters, timestamp)
		p.processVCFrame(worldPoints, timestamp, sensorID)
	}
}

// processBGFrame processes a frame using background subtraction + DBSCAN.
func (p *DualExtractionPipeline) processBGFrame(clusters []WorldCluster, timestamp time.Time) {
	if !p.config.BackgroundSubtractionEnabled {
		return
	}

	prevCount := len(p.bgTracker.Tracks)
	p.bgTracker.Update(clusters, timestamp)
	newCount := len(p.bgTracker.Tracks)

	// Update stats
	p.stats.BGFramesProcessed++
	p.stats.BGClustersTotal += int64(len(clusters))
	if newCount > prevCount {
		p.stats.BGTracksCreated += int64(newCount - prevCount)
	}

	// Calculate running average
	if p.stats.BGFramesProcessed > 0 {
		p.stats.BGAvgClustersFrame = float64(p.stats.BGClustersTotal) / float64(p.stats.BGFramesProcessed)
	}

	// Count confirmed tracks
	confirmed := 0
	for _, track := range p.bgTracker.Tracks {
		if track.State == TrackConfirmed {
			confirmed++
		}
	}
	p.stats.BGTracksConfirmed = int64(confirmed)
}

// processVCFrame processes a frame using velocity-coherent extraction.
func (p *DualExtractionPipeline) processVCFrame(worldPoints []WorldPoint, timestamp time.Time, sensorID string) {
	if !p.config.VelocityCoherentEnabled {
		return
	}

	prevCount := p.vcTracker.TrackCount()
	p.vcTracker.Update(worldPoints, timestamp, sensorID)
	newCount := p.vcTracker.TrackCount()

	// Update stats
	p.stats.VCFramesProcessed++
	if newCount > prevCount {
		p.stats.VCTracksCreated += int64(newCount - prevCount)
	}

	// Count confirmed tracks
	confirmedTracks := p.vcTracker.GetConfirmedTracks()
	p.stats.VCTracksConfirmed = int64(len(confirmedTracks))

	// Count completed tracks
	p.stats.VCTracksCompleted = int64(len(p.vcTracker.GetCompletedTracks()))
}

// GetActiveAlgorithm returns the currently active algorithm.
func (p *DualExtractionPipeline) GetActiveAlgorithm() TrackingAlgorithm {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config.ActiveAlgorithm
}

// SetActiveAlgorithm switches the active algorithm. Thread-safe.
func (p *DualExtractionPipeline) SetActiveAlgorithm(alg TrackingAlgorithm) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !alg.IsValid() {
		return
	}

	p.config.ActiveAlgorithm = alg

	// Enable/disable based on algorithm
	switch alg {
	case AlgorithmBackgroundSubtraction:
		p.config.BackgroundSubtractionEnabled = true
		p.config.VelocityCoherentEnabled = false
	case AlgorithmVelocityCoherent:
		p.config.BackgroundSubtractionEnabled = false
		p.config.VelocityCoherentEnabled = true
	case AlgorithmDual:
		p.config.BackgroundSubtractionEnabled = true
		p.config.VelocityCoherentEnabled = true
	}
}

// GetConfig returns the current pipeline configuration.
func (p *DualExtractionPipeline) GetConfig() DualPipelineConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

// UpdateConfig updates the pipeline configuration. Thread-safe.
func (p *DualExtractionPipeline) UpdateConfig(config DualPipelineConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config = config

	// Update sub-tracker configs
	if p.bgTracker != nil {
		p.bgTracker.Config = config.TrackerConfig
	}
	if p.vcTracker != nil {
		p.vcTracker.UpdateConfig(config.VCConfig)
	}
}

// UpdateVCConfig updates just the velocity-coherent configuration.
func (p *DualExtractionPipeline) UpdateVCConfig(config VelocityCoherentTrackerConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config.VCConfig = config
	if p.vcTracker != nil {
		p.vcTracker.UpdateConfig(config)
	}
}

// UpdateDBSCANParams updates the DBSCAN parameters for background subtraction.
func (p *DualExtractionPipeline) UpdateDBSCANParams(params DBSCANParams) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.DBSCANParams = params
}

// GetStats returns pipeline statistics.
func (p *DualExtractionPipeline) GetStats() PipelineStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// ResetStats resets pipeline statistics.
func (p *DualExtractionPipeline) ResetStats() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats = PipelineStats{}
}

// GetBGTracker returns the background subtraction tracker.
func (p *DualExtractionPipeline) GetBGTracker() *Tracker {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.bgTracker
}

// GetVCTracker returns the velocity-coherent tracker.
func (p *DualExtractionPipeline) GetVCTracker() *VelocityCoherentTracker {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.vcTracker
}

// Reset resets both trackers.
func (p *DualExtractionPipeline) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.bgTracker = NewTracker(p.config.TrackerConfig)
	p.vcTracker.Reset()
	p.stats = PipelineStats{}
}

// SetOnBGTrackCreated sets the callback for when a BG track is created.
func (p *DualExtractionPipeline) SetOnBGTrackCreated(fn func(*TrackedObject)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onBGTrackCreated = fn
}

// SetOnVCTrackCreated sets the callback for when a VC track is created.
func (p *DualExtractionPipeline) SetOnVCTrackCreated(fn func(*VelocityCoherentTrack)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onVCTrackCreated = fn
}

// AlgorithmConfigJSON represents the JSON structure for algorithm configuration API.
type AlgorithmConfigJSON struct {
	Active           string            `json:"active"`
	VelocityCoherent *VCConfigJSON     `json:"velocity_coherent,omitempty"`
	DBSCAN           *DBSCANConfigJSON `json:"dbscan,omitempty"`
}

// VCConfigJSON represents velocity-coherent config in JSON format.
type VCConfigJSON struct {
	MinPts              int     `json:"min_pts,omitempty"`
	PositionEps         float64 `json:"position_eps,omitempty"`
	VelocityEps         float64 `json:"velocity_eps,omitempty"`
	MaxPredictionFrames int     `json:"max_prediction_frames,omitempty"`
	MaxMisses           int     `json:"max_misses,omitempty"`
	HitsToConfirm       int     `json:"hits_to_confirm,omitempty"`
}

// DBSCANConfigJSON represents DBSCAN config in JSON format.
type DBSCANConfigJSON struct {
	Eps    float64 `json:"eps,omitempty"`
	MinPts int     `json:"min_pts,omitempty"`
}

// MarshalJSON implements json.Marshaler for DualPipelineConfig.
func (c DualPipelineConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ActiveAlgorithm              string  `json:"active_algorithm"`
		BackgroundSubtractionEnabled bool    `json:"background_subtraction_enabled"`
		VelocityCoherentEnabled      bool    `json:"velocity_coherent_enabled"`
		DBSCANEps                    float64 `json:"dbscan_eps"`
		DBSCANMinPts                 int     `json:"dbscan_min_pts"`
		VCMinPts                     int     `json:"vc_min_pts"`
		VCPositionEps                float64 `json:"vc_position_eps"`
		VCVelocityEps                float64 `json:"vc_velocity_eps"`
		VCMaxMisses                  int     `json:"vc_max_misses"`
		VCHitsToConfirm              int     `json:"vc_hits_to_confirm"`
		VCMaxPredictionFrames        int     `json:"vc_max_prediction_frames"`
	}{
		ActiveAlgorithm:              string(c.ActiveAlgorithm),
		BackgroundSubtractionEnabled: c.BackgroundSubtractionEnabled,
		VelocityCoherentEnabled:      c.VelocityCoherentEnabled,
		DBSCANEps:                    c.DBSCANParams.Eps,
		DBSCANMinPts:                 c.DBSCANParams.MinPts,
		VCMinPts:                     c.VCConfig.Clustering.MinPts,
		VCPositionEps:                c.VCConfig.Clustering.PositionEps,
		VCVelocityEps:                c.VCConfig.Clustering.VelocityEps,
		VCMaxMisses:                  c.VCConfig.MaxMisses,
		VCHitsToConfirm:              c.VCConfig.HitsToConfirm,
		VCMaxPredictionFrames:        c.VCConfig.PostTail.MaxPredictionFrames,
	})
}
