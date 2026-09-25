package replayeval

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// The fixed_lag_rts experiment: causal tracking against fixed-assignment RTS
// at every comparison horizon, from one replay.
//
// One tracker runs, exactly as shipped. Its filter record is fanned out to a
// smoother per horizon, so every arm refines the same priors, posteriors and
// associations, and the comparison is paired state for state. The plan's
// three-frame comparator and the asynchronous tracking plan's 0.5, 1 and 2 s
// capture-time look-aheads are fixed_lag arms; the whole track is the final
// arm. The online arm is the filter's own posterior carried in those states.
//
// With an observation database, each arm's persistable states (observed and
// confirmed, the online sink's population) are written as their own version
// of lidar_track_estimates with a revision record, after the frame's own
// evidence has committed, so the per-frame evaluator can score any arm
// against reviewed episodes by estimator_id, param_hash and stage. The report
// lists those keys.
//
// This is the fixed-assignment half of the reassociation comparison. The
// revisable-association arm (alternative assignments, point ownership and
// shape belief re-solved inside the window) needs the asynchronous worker's
// immutable input; it would feed its own filter record through the same
// smoother and report into the same table.

// RefinementHorizons are the arms the fixed_lag_rts experiment compares, in
// report order after the online arm.
var RefinementHorizons = []l5tracks.SmootherLag{
	l5tracks.LagFrames(3), l5tracks.LagSeconds(0.5), l5tracks.LagSeconds(1), l5tracks.LagSeconds(2),
	l5tracks.LagTrackEnd(),
}

// onlineEstimatorID is the estimator_id the replay's online rows carry.
const onlineEstimatorID = "cv_kf_v1"

// RefinementReport is refinement_report.json.
type RefinementReport struct {
	SchemaVersion int    `json:"schema_version"`
	SmootherID    string `json:"smoother_id"`
	Population    string `json:"population"`
	// ScoringStartUnixNanos is the capture time from which states are scored.
	ScoringStartUnixNanos int64   `json:"scoring_start_unix_nanos"`
	MeasurementNoise      float64 `json:"measurement_noise"`
	// Persisted is true when the arms were also written to the observation
	// database under SourceID and ObservationModelID.
	Persisted          bool                               `json:"persisted"`
	SourceID           string                             `json:"source_id,omitempty"`
	ObservationModelID string                             `json:"observation_model_id"`
	Arms               []l8analytics.RefinementArmMetrics `json:"arms"`
	Smoothers          []RefinementSmootherReport         `json:"smoothers"`
}

// RefinementSmootherReport is one horizon's smoother accounting.
type RefinementSmootherReport struct {
	Lag         string                  `json:"lag"`
	WindowCap   int                     `json:"window_cap"`
	Stats       l5tracks.SmootherStats  `json:"stats"`
	Persisted   int                     `json:"persisted_estimates"`
	Config      l5tracks.SmootherConfig `json:"config"`
	EstimatorID string                  `json:"estimator_id"`
	ParamHash   string                  `json:"param_hash"`
}

// refinementHarness is the tracker's filter-step observer for the
// experiment. It is called from Update on the replay's frame goroutine, and
// flushed from the same goroutine after each frame.
type refinementHarness struct {
	smoothers   []*l5tracks.FixedLagSmoother
	paramHashes []string
	arms        []*l8analytics.RefinementAccumulator
	online      *l8analytics.RefinementAccumulator
	// reference is the whole-track arm; its release stream feeds the online
	// accumulator, since every arm releases every state exactly once.
	reference int

	onlineParamHash    string
	observationModelID string
	identity           *pipeline.RefinedEstimateIdentity
	db                 observationsqlite.DBClient
	pending            []observationsqlite.RevisedStateEstimate
	persisted          []int
	err                error

	scoreFrom        int64
	measurementNoise float64
}

func newRefinementHarness(measurementNoise float64, scoreFrom int64, onlineParamHash, observationModelID string,
	identity *pipeline.RefinedEstimateIdentity, db observationsqlite.DBClient) (*refinementHarness, error) {
	if identity != nil && db == nil {
		return nil, fmt.Errorf("refinement persistence needs a database")
	}
	h := &refinementHarness{
		onlineParamHash: onlineParamHash, observationModelID: observationModelID, identity: identity, db: db,
		online: l8analytics.NewOnlineAccumulator(measurementNoise, scoreFrom), reference: -1,
		scoreFrom: scoreFrom, measurementNoise: measurementNoise,
	}
	for i, lag := range RefinementHorizons {
		cfg := l5tracks.SmootherConfig{Lag: lag}
		s, err := l5tracks.NewFixedLagSmoother(cfg)
		if err != nil {
			return nil, err
		}
		hash, err := pipeline.RefinedParamHash(onlineParamHash, cfg)
		if err != nil {
			return nil, err
		}
		stage := l5tracks.RefinementFixedLag
		if lag.IsTrackEnd() {
			stage = l5tracks.RefinementFinal
			h.reference = i
		}
		h.smoothers = append(h.smoothers, s)
		h.paramHashes = append(h.paramHashes, hash)
		h.arms = append(h.arms, l8analytics.NewRefinementAccumulator(
			fmt.Sprintf("%s %s", stage, lag), string(stage), lag.String(), measurementNoise, scoreFrom))
	}
	if h.reference < 0 {
		return nil, fmt.Errorf("refinement horizons need a whole-track arm to carry the online estimate")
	}
	h.persisted = make([]int, len(h.smoothers))
	return h, nil
}

// ObserveFilterFrame implements l5tracks.FilterStepObserver.
func (h *refinementHarness) ObserveFilterFrame(frame l5tracks.FilterFrame) {
	for i, s := range h.smoothers {
		h.take(i, s.Observe(frame))
	}
}

func (h *refinementHarness) take(arm int, states []l5tracks.SmoothedState) {
	for _, state := range states {
		h.arms[arm].Add(state)
		if arm == h.reference {
			h.online.Add(state)
		}
		if h.identity == nil || h.err != nil || !pipeline.Persistable(state) {
			continue
		}
		row, err := pipeline.RefinedStateEstimate(*h.identity, h.paramHashes[arm], state)
		if err != nil {
			h.err = fmt.Errorf("prepare refined estimate: %w", err)
			continue
		}
		h.pending = append(h.pending, row)
		h.persisted[arm]++
	}
}

// flush writes what the last frame released, in one transaction. The frame's
// own observations and online rows have committed by then, so every refined
// row's observation and revised estimate already exist.
func (h *refinementHarness) flush() {
	if h == nil || h.err != nil || len(h.pending) == 0 {
		return
	}
	if err := observationsqlite.InsertRevisedStateEstimates(h.db, h.pending); err != nil {
		h.err = fmt.Errorf("store refined estimates: %w", err)
		return
	}
	h.pending = h.pending[:0]
}

// finish ends the input: every open chain is released as capture_end, the
// last batch is written, and the report assembled.
func (h *refinementHarness) finish(sourceID string) (*RefinementReport, error) {
	for i, s := range h.smoothers {
		h.take(i, s.Flush())
	}
	h.flush()
	if h.err != nil {
		return nil, h.err
	}
	report := &RefinementReport{
		SchemaVersion: 1, SmootherID: l5tracks.SmootherID,
		Population:            "confirmed_at_frame_from_scoring_start; residuals over observed states",
		ScoringStartUnixNanos: h.scoreFrom, MeasurementNoise: h.measurementNoise,
		Persisted: h.identity != nil, ObservationModelID: h.observationModelID,
	}
	if h.identity != nil {
		report.SourceID = sourceID
	}
	online := h.online.Metrics()
	online.EstimatorID, online.ParamHash = onlineEstimatorID, h.onlineParamHash
	report.Arms = append(report.Arms, online)
	for i, s := range h.smoothers {
		metrics := h.arms[i].Metrics()
		metrics.EstimatorID = pipeline.RefinedEstimatorID(onlineEstimatorID)
		metrics.ParamHash = h.paramHashes[i]
		report.Arms = append(report.Arms, metrics)
		cfg := s.Config()
		report.Smoothers = append(report.Smoothers, RefinementSmootherReport{
			Lag: cfg.Lag.String(), WindowCap: cfg.WindowCap(), Stats: s.Stats(), Persisted: h.persisted[i],
			Config: cfg, EstimatorID: metrics.EstimatorID, ParamHash: metrics.ParamHash,
		})
	}
	return report, nil
}
