package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Refined estimates: how a retrospectively refined state becomes a
// lidar_track_estimates row beside the online one it revises.
//
// A refined version is keyed so it can never overwrite, or be mistaken for,
// the online estimates of the same run (plan §11.2):
//
//   - estimator_id is the online estimator's with the smoother appended,
//     "cv_kf_v1+rts_fixed_assignment_v1": the same filter, revised by a named
//     algorithm;
//   - param_hash covers the online parameter hash and the smoother's own
//     configuration, so each horizon is its own version;
//   - stage is fixed_lag or final;
//   - estimate_id carries all three, where the online key carries none.
//
// Only states the online sink would also have written are converted: observed
// and confirmed at that frame. The two arms of a comparison therefore cover
// the same frames, and a refined arm is never scored on tentative tracks the
// online arm never published. Coasted states are smoothed and scored in
// memory but not persisted: a row must name the immutable observation behind
// it, and a coasted state has none. Their persistence belongs to the VRLOG
// trajectory record, which carries observed, inferred and unsupported status.

// RefinedEstimateIdentity names the online estimate family a refined stage
// revises. The fields are those of TrackingPipelineConfig's state sink.
type RefinedEstimateIdentity struct {
	SourceID           string
	CalibrationID      string
	OnlineEstimatorID  string
	ObservationModelID string
	OnlineParamHash    string
}

func (id RefinedEstimateIdentity) validate() error {
	if id.SourceID == "" || id.CalibrationID == "" || id.OnlineEstimatorID == "" || id.ObservationModelID == "" || id.OnlineParamHash == "" {
		return fmt.Errorf("refined estimates require source, calibration, estimator, observation-model and parameter identities")
	}
	return nil
}

// RefinedEstimatorID is the estimator_id of a smoother's output.
func RefinedEstimatorID(onlineEstimatorID string) string {
	return onlineEstimatorID + "+" + l5tracks.SmootherID
}

// RefinedParamHash is the param_hash of one smoother configuration's output
// over one online parameter set.
func RefinedParamHash(onlineParamHash string, cfg l5tracks.SmootherConfig) (string, error) {
	payload, err := json.Marshal(struct {
		Online   string                  `json:"online_param_hash"`
		Smoother string                  `json:"smoother_id"`
		Config   l5tracks.SmootherConfig `json:"config"`
		Cap      int                     `json:"window_cap"`
	}{onlineParamHash, l5tracks.SmootherID, cfg, cfg.WindowCap()})
	if err != nil {
		return "", fmt.Errorf("encode smoother configuration: %w", err)
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// Persistable reports whether a refined state belongs to the persisted
// population: observed and confirmed at its frame, as online rows are.
func Persistable(state l5tracks.SmoothedState) bool {
	return state.Observed && state.Confirmed
}

// RefinedStateEstimate converts one released state. paramHash is
// RefinedParamHash for the smoother that released it.
func RefinedStateEstimate(id RefinedEstimateIdentity, paramHash string, state l5tracks.SmoothedState) (sqlite.RevisedStateEstimate, error) {
	if err := id.validate(); err != nil {
		return sqlite.RevisedStateEstimate{}, err
	}
	if !Persistable(state) {
		return sqlite.RevisedStateEstimate{}, fmt.Errorf("refined state %s at %d is not observed and confirmed", state.TrackID, state.FrameUnixNanos)
	}
	stage := string(state.Stage)
	if stage != sqlite.EstimateStageFixedLag && stage != sqlite.EstimateStageFinal {
		return sqlite.RevisedStateEstimate{}, fmt.Errorf("refined state %s has stage %q", state.TrackID, stage)
	}
	obs := state.Observation
	observationID, err := l4bobserve.ObservationID(id.SourceID, id.CalibrationID, state.FrameUnixNanos, obs.ClusterID)
	if err != nil {
		return sqlite.RevisedStateEstimate{}, fmt.Errorf("derive refined estimate observation identity: %w", err)
	}
	estimatorID := RefinedEstimatorID(id.OnlineEstimatorID)
	estimateID := fmt.Sprintf("estimate/%s/%s/%s/%s/%s/%d", state.TrackID, estimatorID, id.ObservationModelID, paramHash, stage, state.FrameUnixNanos)
	m := state.Smoothed
	estimate := sqlite.TrackEstimate{
		EstimateID: estimateID, TrackID: state.TrackID, ObservationID: observationID,
		SourceID: id.SourceID, CalibrationID: id.CalibrationID,
		FrameUnixNanos: state.FrameUnixNanos, MeasurementUnixNanos: obs.MeasurementUnixNanos,
		CreationSequence: state.CreationSequence, EstimatorID: estimatorID, ObservationModelID: id.ObservationModelID,
		ParamHash: paramHash, Stage: stage, MeasurementSource: string(obs.Source),
		X: m.X, Y: m.Y, VX: m.VX, VY: m.VY, Covariance: m.P,
	}
	// The residual row is the association evidence the estimate was computed
	// under, which fixed assignment leaves exactly as the filter measured it.
	// The refined position's own residual is measurement minus X/Y above.
	residual := sqlite.TrackResidual{
		EstimateID: estimateID, ObservationID: observationID,
		PredictedX: obs.X - obs.InnovationX, PredictedY: obs.Y - obs.InnovationY,
		MeasurementX: obs.X, MeasurementY: obs.Y, InnovationX: obs.InnovationX, InnovationY: obs.InnovationY,
		NIS: obs.NIS, GeometryCovXX: obs.GeometryCovariance.XX, GeometryCovXY: obs.GeometryCovariance.XY,
		GeometryCovYY: obs.GeometryCovariance.YY, Disposition: "accepted", Reason: "fixed_assignment",
	}
	revision, err := estimateRevision(id, estimateID, state)
	if err != nil {
		return sqlite.RevisedStateEstimate{}, err
	}
	return sqlite.RevisedStateEstimate{Estimate: estimate, Residual: residual, Revision: revision}, nil
}

func estimateRevision(id RefinedEstimateIdentity, estimateID string, state l5tracks.SmoothedState) (sqlite.EstimateRevision, error) {
	online := state.Online
	rev := state.Revision
	out := sqlite.EstimateRevision{
		EstimateID:          estimateID,
		RevisesEstimateID:   onlineEstimateID(state.TrackID, id.OnlineEstimatorID, id.ObservationModelID, state.FrameUnixNanos),
		SmootherID:          l5tracks.SmootherID,
		Lag:                 state.Lag.String(),
		LookaheadSteps:      state.LookaheadSteps,
		LookaheadSecs:       state.LookaheadSecs,
		ReleasedAtUnixNanos: state.ReleasedAtUnixNanos,
		ReleaseReason:       string(state.Release),
		ChainEndReason:      string(state.ChainEnd),
		Flags:               refinementFlags(state),
		PreviousX:           online.X, PreviousY: online.Y, PreviousVX: online.VX, PreviousVY: online.VY,
		RevisionPositionMetres: rev.PositionMetres,
		RevisionVelocityMps:    rev.VelocityMps,
		EvidenceCount:          len(rev.Evidence),
	}
	if len(rev.Evidence) > 0 {
		out.EvidenceFirstFrameUnixNanos = rev.Evidence[0].FrameUnixNanos
		out.EvidenceLastFrameUnixNanos = rev.Evidence[len(rev.Evidence)-1].FrameUnixNanos
	}
	if strongest, ok := rev.StrongestEvidence(); ok {
		observationID, err := l4bobserve.ObservationID(id.SourceID, id.CalibrationID, strongest.FrameUnixNanos, strongest.ClusterID)
		if err != nil {
			return sqlite.EstimateRevision{}, fmt.Errorf("derive strongest evidence identity: %w", err)
		}
		out.StrongestEvidenceObservationID = observationID
		out.StrongestEvidenceNIS = strongest.NIS
	}
	return out, nil
}

func refinementFlags(state l5tracks.SmoothedState) []string {
	var flags []string
	if state.LookaheadTruncated {
		flags = append(flags, "lookahead_truncated")
	}
	if state.ClampedPrediction {
		flags = append(flags, "clamped_prediction")
	}
	if state.CovarianceFallback {
		flags = append(flags, "covariance_fallback")
	}
	if state.RevisionWithoutEvidence {
		flags = append(flags, "revision_without_evidence")
	}
	return flags
}
