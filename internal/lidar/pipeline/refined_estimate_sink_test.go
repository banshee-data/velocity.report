package pipeline

import (
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func refinedIdentity() RefinedEstimateIdentity {
	return RefinedEstimateIdentity{
		SourceID: "source/v1/test", CalibrationID: "calibration/v1/test", OnlineEstimatorID: "cv_kf_v1",
		ObservationModelID: "medoid_v0", OnlineParamHash: "sha256:online",
	}
}

func releasedState() l5tracks.SmoothedState {
	return l5tracks.SmoothedState{
		TrackID: "trk_a", CreationSequence: 7, FrameUnixNanos: 1_000, StateUnixNanos: 1_000,
		Stage: l5tracks.RefinementFixedLag, Lag: l5tracks.LagSeconds(0.5), Observed: true, Confirmed: true,
		Observation: l5tracks.FilterObservation{
			ClusterID: 42, MeasurementUnixNanos: 1_010, Source: l5tracks.MeasurementMedoidV0, X: 5, Y: 6,
			InnovationX: 0.25, InnovationY: -0.5, NIS: 2.5, HasInnovation: true,
			GeometryCovariance: l5tracks.MeasurementCovariance{XX: 0.1, XY: 0.01, YY: 0.2},
		},
		Online:   l5tracks.FilterMoments{X: 5, Y: 6, VX: 10},
		Smoothed: l5tracks.FilterMoments{X: 5.1, Y: 6, VX: 10.5, P: [16]float32{0.02}},
		Revision: l5tracks.Revision{
			DX: 0.1, PositionMetres: 0.1, VelocityMps: 0.5,
			Evidence: []l5tracks.EvidenceRef{
				{FrameUnixNanos: 1_100, ClusterID: 43, NIS: 1, HasInnovation: true},
				{FrameUnixNanos: 1_300, ClusterID: 44, NIS: 9, HasInnovation: true},
			},
		},
		LookaheadSteps: 3, LookaheadSecs: 0.5, ReleasedAtUnixNanos: 1_500, Release: l5tracks.ReleaseLag,
		ClampedPrediction: true,
	}
}

// A refined row must name exactly the online row and immutable observation
// the online sink wrote for the same track and frame, under a version key of
// its own that cannot collide with it.
func TestRefinedStateEstimateSharesTheOnlineIdentities(t *testing.T) {
	id := refinedIdentity()
	state := releasedState()
	hash, err := RefinedParamHash(id.OnlineParamHash, l5tracks.SmootherConfig{Lag: state.Lag})
	if err != nil {
		t.Fatal(err)
	}
	row, err := RefinedStateEstimate(id, hash, state)
	if err != nil {
		t.Fatal(err)
	}

	track := &l5tracks.TrackedObject{TrackID: state.TrackID, LastClusterID: state.Observation.ClusterID,
		CreationSequence: state.CreationSequence}
	track.LastResidual.Valid = true
	cfg := &TrackingPipelineConfig{
		ObservationSourceID: id.SourceID, ObservationCalibrationID: id.CalibrationID,
		StateEstimatorID: id.OnlineEstimatorID, StateObservationModelID: id.ObservationModelID, StateParameterHash: id.OnlineParamHash,
	}
	online, err := onlineStateEstimate(cfg, track, state.FrameUnixNanos)
	if err != nil {
		t.Fatal(err)
	}

	e, r, v := row.Estimate, row.Residual, row.Revision
	if v.RevisesEstimateID != online.Estimate.EstimateID || e.ObservationID != online.Estimate.ObservationID {
		t.Fatalf("refined row names %q / %q, online row is %q / %q", v.RevisesEstimateID, e.ObservationID,
			online.Estimate.EstimateID, online.Estimate.ObservationID)
	}
	if e.EstimateID == online.Estimate.EstimateID || !strings.Contains(e.EstimateID, hash) || !strings.Contains(e.EstimateID, "/fixed_lag/") {
		t.Fatalf("refined estimate ID %q must carry its version and differ from the online %q", e.EstimateID, online.Estimate.EstimateID)
	}
	if e.EstimatorID != "cv_kf_v1+"+l5tracks.SmootherID || e.ParamHash != hash || e.Stage != sqlite.EstimateStageFixedLag {
		t.Fatalf("version key %s / %s / %s", e.EstimatorID, e.ParamHash, e.Stage)
	}
	if e.X != 5.1 || e.VX != 10.5 || e.CreationSequence != 7 || e.MeasurementUnixNanos != 1_010 {
		t.Fatalf("estimate values %+v", e)
	}
	if r.PredictedX != 4.75 || r.PredictedY != 6.5 || r.NIS != 2.5 || r.GeometryCovYY != 0.2 || r.Reason != "fixed_assignment" {
		t.Fatalf("residual %+v", r)
	}
	strongest, _ := l4bobserve.ObservationID(id.SourceID, id.CalibrationID, 1_300, 44)
	if v.EvidenceCount != 2 || v.EvidenceFirstFrameUnixNanos != 1_100 || v.EvidenceLastFrameUnixNanos != 1_300 ||
		v.StrongestEvidenceObservationID != strongest || v.StrongestEvidenceNIS != 9 {
		t.Fatalf("evidence %+v", v)
	}
	if v.Lag != "0.5s" || v.ReleaseReason != "lag" || strings.Join(v.Flags, ",") != "clamped_prediction" || v.PreviousVX != 10 {
		t.Fatalf("revision %+v", v)
	}
}

func TestRefinedStateEstimateRefusesWhatTheOnlineSinkWouldNotWrite(t *testing.T) {
	id := refinedIdentity()
	for name, mutate := range map[string]func(*l5tracks.SmoothedState){
		"coasted":      func(s *l5tracks.SmoothedState) { s.Observed = false },
		"tentative":    func(s *l5tracks.SmoothedState) { s.Confirmed = false },
		"online stage": func(s *l5tracks.SmoothedState) { s.Stage = l5tracks.RefinementOnline },
	} {
		state := releasedState()
		mutate(&state)
		if _, err := RefinedStateEstimate(id, "sha256:x", state); err == nil {
			t.Errorf("%s: converted", name)
		}
	}
	bad := id
	bad.CalibrationID = ""
	if _, err := RefinedStateEstimate(bad, "sha256:x", releasedState()); err == nil {
		t.Error("converted without a calibration identity")
	}
}

// Every horizon is its own version, stable across calls, and never the
// online parameter hash.
func TestRefinedParamHashSeparatesHorizons(t *testing.T) {
	seen := map[string]string{}
	for _, lag := range []l5tracks.SmootherLag{l5tracks.LagFrames(3), l5tracks.LagSeconds(0.5), l5tracks.LagSeconds(1),
		l5tracks.LagSeconds(2), l5tracks.LagTrackEnd()} {
		cfg := l5tracks.SmootherConfig{Lag: lag}
		a, err := RefinedParamHash("sha256:online", cfg)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := RefinedParamHash("sha256:online", cfg)
		if a != b || a == "sha256:online" || !strings.HasPrefix(a, "sha256:") {
			t.Fatalf("%s: hash %q unstable or not distinct", lag, a)
		}
		if other, dup := seen[a]; dup {
			t.Fatalf("%s and %s share a parameter hash", lag, other)
		}
		seen[a] = lag.String()
	}
	a, _ := RefinedParamHash("sha256:one", l5tracks.SmootherConfig{Lag: l5tracks.LagFrames(3)})
	b, _ := RefinedParamHash("sha256:two", l5tracks.SmootherConfig{Lag: l5tracks.LagFrames(3)})
	if a == b {
		t.Fatal("the refined hash ignores the online parameter set it revises")
	}
}
