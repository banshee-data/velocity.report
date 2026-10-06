package pipeline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
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
		// A body centre measured from a medoid: the row must state the
		// former, which the measurement source cannot supply.
		Reference: l5tracks.ReferenceBodyCentre, Support: l5tracks.SupportObserved,
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
	if e.Reference != l5tracks.ReferenceBodyCentre || e.Support != l5tracks.SupportObserved || e.MeasurementSource != "medoid_v0" {
		t.Fatalf("the row states %s and %q from %s, not the smoother's own", e.Reference, e.Support, e.MeasurementSource)
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

// trackedBodyState is releasedState under NearEdgeTracking: the step carried
// the body's online reading, whose state is the online posterior.
func trackedBodyState(stage l5tracks.RefinementStage) l5tracks.SmoothedState {
	state := releasedState()
	state.Stage = stage
	state.Online.P = [16]float32{0.05, 0, 0, 0, 0, 0.05, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	state.Smoothed.P = [16]float32{0.02, 0.001, 0, 0, 0.001, 0.03, 0, 0, 0, 0, 0.5, 0, 0, 0, 0, 0.5}
	state.SolidBody = &l5tracks.SolidBodyReading{
		Estimate: l5tracks.SolidBodyEstimate{
			StateModel: l5tracks.StateModelCVCartesianV1, Reference: state.Reference,
			X: state.Online.X, Y: state.Online.Y,
			PositionCovariance: [4]float32{state.Online.P[0], state.Online.P[1], state.Online.P[4], state.Online.P[5]},
			Orientation:        l5tracks.OrientationBelief{PsiRad: 0.1, VarianceRad2: 0.01, Provenance: l5tracks.ProvenanceObserved},
			Length:             l5tracks.DimensionBelief{Metres: 4.25, SigmaMetres: 0.25, AdmissibleFrames: 12, Provenance: l5tracks.ProvenanceAccumulated},
			Width:              l5tracks.DimensionBelief{Metres: 1.75, SigmaMetres: 0.25, AdmissibleFrames: 12, Provenance: l5tracks.ProvenanceAccumulated},
			Height:             l5tracks.DimensionBelief{Metres: 1.6, SigmaMetres: 1.5, Provenance: l5tracks.ProvenanceClassPrior},
			GroundSurfaceModel: l5tracks.GroundSurfaceClusterMinZ,
			Motion:             l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.9},
			Estimation:         l5tracks.EstimationEstablished,
			Stage:              l5tracks.StageLive,
			Support:            l5tracks.SupportState{PointCount: 180, Instant: l5tracks.SupportObserved},
		},
		VX: state.Online.VX, VY: state.Online.VY, Covariance: state.Online.P,
		Measurement: l5tracks.SolidBodyMeasurement{Source: l5tracks.MeasurementNearEdgeCandidateV1, Rank: 1, NIS: 0.5},
	}
	return state
}

// A refined state carrying the tracked body's reading files a solid body
// beside its estimate: the estimate's version, stage, observation and
// revised state, the online body's beliefs, and a key of its own. Written,
// it reads back through the version reader as it was given.
func TestRefinedStateEstimateFilesTheSolidBodyBesideIt(t *testing.T) {
	id := refinedIdentity()
	id.ObservationModelID = string(l5tracks.MeasurementNearEdgeCandidateV1)
	database, cleanup := db.NewTestDB(t)
	defer cleanup()
	store := sqlite.NewStateEstimateStore(database)
	for _, stage := range []l5tracks.RefinementStage{l5tracks.RefinementFixedLag, l5tracks.RefinementFinal} {
		state := trackedBodyState(stage)
		row, err := RefinedStateEstimate(id, "sha256:"+string(stage), state)
		if err != nil {
			t.Fatal(err)
		}
		sb, e := row.SolidBody, row.Estimate
		if sb == nil {
			t.Fatalf("%s: no solid body filed", stage)
		}
		if sb.Stage != e.Stage || sb.EstimatorID != e.EstimatorID || sb.ParamHash != e.ParamHash || sb.ObservationID != e.ObservationID ||
			sb.TrackID != e.TrackID || sb.CreationSequence != e.CreationSequence || sb.FrameUnixNanos != e.FrameUnixNanos ||
			sb.ObservationModelID != string(l5tracks.MeasurementNearEdgeCandidateV1) {
			t.Fatalf("%s: solid body version %+v beside estimate %+v", stage, *sb, e)
		}
		if !strings.HasPrefix(sb.EstimateID, "solid_body/") || !strings.Contains(sb.EstimateID, "/"+e.ParamHash+"/"+e.Stage+"/") {
			t.Fatalf("%s: solid body key %q does not carry its version", stage, sb.EstimateID)
		}
		r, online := sb.Reading, *state.SolidBody
		if r.Estimate.X != state.Smoothed.X || r.Estimate.Y != state.Smoothed.Y || r.VX != state.Smoothed.VX ||
			r.Covariance != state.Smoothed.P || r.Estimate.PositionCovariance != [4]float32{0.02, 0.001, 0.001, 0.03} {
			t.Fatalf("%s: the solid body does not carry the revised state: %+v", stage, r)
		}
		if r.Estimate.Stage != l5tracks.StageSmoothed || r.Estimate.Reference != state.Reference {
			t.Fatalf("%s: stage %s, reference %s", stage, r.Estimate.Stage, r.Estimate.Reference)
		}
		// Every belief is the online body's.
		r.Estimate.X, r.Estimate.Y, r.VX, r.VY = online.Estimate.X, online.Estimate.Y, online.VX, online.VY
		r.Covariance, r.Estimate.PositionCovariance, r.Estimate.Stage = online.Covariance, online.Estimate.PositionCovariance, online.Estimate.Stage
		if !reflect.DeepEqual(r, online) {
			t.Fatalf("%s: beliefs changed:\n got %+v\nwant %+v", stage, r, online)
		}
		if state.SolidBody.Estimate.X != state.Online.X {
			t.Fatalf("%s: building the row modified the shared reading", stage)
		}

		insertObservationFor(t, database, id, state)
		if err := store.InsertRevised([]sqlite.RevisedStateEstimate{row}); err != nil {
			t.Fatalf("%s: %v", stage, err)
		}
		got, err := store.ListVersionSolidBodies(sqlite.EstimateVersionKey{SourceID: id.SourceID, EstimatorID: e.EstimatorID,
			ObservationModelID: sb.ObservationModelID, ParamHash: e.ParamHash, Stage: e.Stage})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || !reflect.DeepEqual(got[0].TrackSolidBody, *sb) {
			t.Fatalf("%s: read back %+v, wrote %+v", stage, got, *sb)
		}
	}

	// Without a reading, a state files no solid body.
	row, err := RefinedStateEstimate(id, "sha256:x", releasedState())
	if err != nil || row.SolidBody != nil {
		t.Fatalf("a state without a reading filed %+v (%v)", row.SolidBody, err)
	}
}

// insertObservationFor stores the immutable observation a refined state names,
// so the version reader can read its sensor. It is a no-op the second time.
func insertObservationFor(t *testing.T, database *db.DB, id RefinedEstimateIdentity, state l5tracks.SmoothedState) {
	t.Helper()
	observationID, err := l4bobserve.ObservationID(id.SourceID, id.CalibrationID, state.FrameUnixNanos, state.Observation.ClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT OR IGNORE INTO lidar_observations
		(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
		 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
		VALUES (?, 1, ?, ?, 'sensor', 'frame', ?, ?, ?, '{}', 1)`,
		observationID, id.SourceID, id.CalibrationID, state.FrameUnixNanos, state.FrameUnixNanos, state.Observation.ClusterID); err != nil {
		t.Fatal(err)
	}
}

// A reading that is not the state the smoother revised belongs to another
// filter, whose revision this is not: refused rather than filed.
func TestRefinedStateEstimateRefusesAReadingOfAnotherState(t *testing.T) {
	id := refinedIdentity()
	for name, mutate := range map[string]func(*l5tracks.SolidBodyReading){
		"another position":  func(r *l5tracks.SolidBodyReading) { r.Estimate.X += 0.5 },
		"another lateral":   func(r *l5tracks.SolidBodyReading) { r.Estimate.Y += 0.5 },
		"another velocity":  func(r *l5tracks.SolidBodyReading) { r.VX += 1 },
		"another heading":   func(r *l5tracks.SolidBodyReading) { r.VY += 1 },
		"another reference": func(r *l5tracks.SolidBodyReading) { r.Estimate.Reference = l5tracks.ReferenceClusterMedoid },
	} {
		state := trackedBodyState(l5tracks.RefinementFixedLag)
		reading := *state.SolidBody
		mutate(&reading)
		state.SolidBody = &reading
		if _, err := RefinedStateEstimate(id, "sha256:x", state); err == nil || !strings.Contains(err.Error(), "not the") {
			t.Errorf("%s: error %v", name, err)
		}
	}
}

// An observation the filter did not apply is recorded as the online row
// recorded it; an applied one keeps the fixed-assignment reason.
func TestRefinedResidualKeepsTheOnlineDisposition(t *testing.T) {
	id := refinedIdentity()
	for _, tc := range []struct{ disposition, reason, wantDisposition, wantReason string }{
		{"", "", "accepted", "fixed_assignment"},
		{l5tracks.ResidualNotApplied, "no_usable_face", l5tracks.ResidualNotApplied, "no_usable_face"},
		{l5tracks.ResidualReferenceChanged, "body_centre_lapsed", l5tracks.ResidualReferenceChanged, "body_centre_lapsed"},
	} {
		state := releasedState()
		state.Observation.Disposition, state.Observation.Reason = tc.disposition, tc.reason
		row, err := RefinedStateEstimate(id, "sha256:x", state)
		if err != nil {
			t.Fatal(err)
		}
		if row.Residual.Disposition != tc.wantDisposition || row.Residual.Reason != tc.wantReason {
			t.Errorf("online %q/%q: refined residual %q/%q", tc.disposition, tc.reason, row.Residual.Disposition, row.Residual.Reason)
		}
	}
}
