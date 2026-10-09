package sqlite

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// testSolidBody is a fully populated solid body filed beside the point
// estimate testFrameStateEstimate builds for the same observation.
func testSolidBody(observationID string, frameUnixNanos int64) TrackSolidBody {
	cov := [16]float32{
		0.04, 0.01, 0.002, 0,
		0.01, 0.05, 0, 0.003,
		0.002, 0, 0.3, 0,
		0, 0.003, 0, 0.3,
	}
	return TrackSolidBody{
		EstimateID: "solid_body/track/frame/" + observationID, TrackID: "track/frame", ObservationID: observationID,
		SourceID: "source/v1/frame", CalibrationID: "calibration/v1/test", FrameUnixNanos: frameUnixNanos,
		MeasurementUnixNanos: frameUnixNanos + 1, EstimatorID: "cv_kf_v1",
		ObservationModelID: string(l5tracks.MeasurementNearEdgeCandidateV1), ParamHash: "params/frame", Stage: "online",
		CreationSequence: 7,
		Reading: l5tracks.SolidBodyReading{
			Estimate: l5tracks.SolidBodyEstimate{
				StateModel:         l5tracks.StateModelCVCartesianV1,
				Reference:          l5tracks.ReferenceBodyCentre,
				X:                  12.5,
				Y:                  -3.25,
				PositionCovariance: [4]float32{cov[0], cov[1], cov[4], cov[5]},
				Orientation: l5tracks.OrientationBelief{
					PsiRad: 0.125, VarianceRad2: 0.01, Provenance: l5tracks.ProvenanceObserved,
				},
				Length:                l5tracks.DimensionBelief{Metres: 4.375, SigmaMetres: 0.5, AdmissibleFrames: 9, Provenance: l5tracks.ProvenanceAccumulated},
				Width:                 l5tracks.DimensionBelief{Metres: 1.875, SigmaMetres: 0.5, AdmissibleFrames: 9, Provenance: l5tracks.ProvenanceAccumulated},
				Height:                l5tracks.DimensionBelief{Metres: 1.6, SigmaMetres: 1.5, Provenance: l5tracks.ProvenanceClassPrior},
				GroundZ:               -0.25,
				GroundSurfaceModel:    l5tracks.GroundSurfaceClusterMinZ,
				Motion:                l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.75},
				Estimation:            l5tracks.EstimationEstablished,
				Stage:                 l5tracks.StageLive,
				LastObservedUnixNanos: frameUnixNanos + 1,
				Support:               l5tracks.SupportState{PointCount: 212},
			},
			VX: 11.5, VY: 0.25, Covariance: cov,
			Measurement: l5tracks.SolidBodyMeasurement{
				Source: l5tracks.MeasurementNearEdgeCandidateV1, Rank: 2,
				Faces:     mustFaces("rear,right"),
				AspectRad: 0.5, AspectKnown: true, NIS: 1.25,
			},
		},
	}
}

func mustFaces(s string) l5tracks.VisibleFaces {
	v, err := l5tracks.ParseVisibleFaces(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestSolidBodyRoundTripsThroughItsOwnTable(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)

	withAspect := testSolidBody("observation/v1/a", 100)
	withoutAspect := testSolidBody("observation/v1/b", 200)
	withoutAspect.Reading.Estimate.Orientation = l5tracks.OrientationBelief{}
	withoutAspect.Reading.Measurement = l5tracks.SolidBodyMeasurement{
		Source: l5tracks.MeasurementMedoidV0, Rank: 2, FallbackReason: "missing_heading",
	}
	withoutAspect.Reading.Estimate.Reference = l5tracks.ReferenceClusterMedoid
	for _, sb := range []TrackSolidBody{withoutAspect, withAspect} {
		if err := store.InsertSolidBody(sb); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListSolidBodiesBySource("source/v1/frame")
	if err != nil {
		t.Fatal(err)
	}
	want := []TrackSolidBody{withAspect, withoutAspect}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("solid bodies did not round-trip:\n got %+v\nwant %+v", got, want)
	}
	var stateModel string
	if err := database.QueryRow(`SELECT state_model FROM lidar_track_solid_bodies WHERE estimate_id = ?`, withAspect.EstimateID).Scan(&stateModel); err != nil {
		t.Fatal(err)
	}
	if stateModel != l5tracks.StateModelCVCartesianV1 {
		t.Fatalf("persisted state model %q", stateModel)
	}
	var aspect any
	if err := database.QueryRow(`SELECT aspect_rad FROM lidar_track_solid_bodies WHERE estimate_id = ?`, withoutAspect.EstimateID).Scan(&aspect); err != nil {
		t.Fatal(err)
	}
	if aspect != nil {
		t.Fatalf("an unknown aspect was stored as %v rather than NULL", aspect)
	}
}

// The containment detail (migration 000059 and 000060) round-trips, and a
// row without a share reads back unknown rather than zero.
func TestSolidBodyContainmentDetailRoundTrips(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)

	held := testSolidBody("observation/v1/a", 100)
	held.Reading.Measurement.ContainmentKnown, held.Reading.Measurement.ContainmentShare, held.Reading.Measurement.ContainedPoints = true, 0.97, 212
	held.Reading.Measurement.ObservedSpanAlongMetres, held.Reading.Measurement.ObservedSpanAcrossMetres = 4.25, 1.75
	held.Reading.Measurement.ExtentFloor = "length"
	held.Reading.Measurement.ContainmentShiftAlongMetres, held.Reading.Measurement.ContainmentShiftAcrossMetres = 0.12, -0.3
	unknown := testSolidBody("observation/v1/b", 200)
	for _, sb := range []TrackSolidBody{held, unknown} {
		if err := store.InsertSolidBody(sb); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListSolidBodiesBySource("source/v1/frame")
	if err != nil {
		t.Fatal(err)
	}
	if want := []TrackSolidBody{held, unknown}; !reflect.DeepEqual(got, want) {
		t.Fatalf("containment detail did not round-trip:\n got %+v\nwant %+v", got[0].Reading.Measurement, want[0].Reading.Measurement)
	}
	var share any
	if err := database.QueryRow(`SELECT containment_share FROM lidar_track_solid_bodies WHERE estimate_id = ?`, unknown.EstimateID).Scan(&share); err != nil {
		t.Fatal(err)
	}
	if share != nil {
		t.Fatalf("an unknown containment share was stored as %v rather than NULL", share)
	}
}

// TestSolidBodySupportDetailRoundTrips: an explained absence, a fragmented
// or truncated cluster, and a row written before the support token was
// stored (migration 000053), which reads back unrecorded rather than
// invented. An unknown token is refused.
func TestSolidBodySupportDetailRoundTrips(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)

	occluded := testSolidBody("observation/v1/a", 100)
	occluded.Reading.Estimate.Support = l5tracks.SupportState{CoastedFrames: 2, Instant: l5tracks.SupportOccludedInferred}
	split := testSolidBody("observation/v1/b", 200)
	split.Reading.Estimate.Support = l5tracks.SupportState{PointCount: 30, Instant: l5tracks.SupportObserved,
		Fragmented: true, Truncated: true}
	legacy := testSolidBody("observation/v1/c", 300)
	for _, sb := range []TrackSolidBody{occluded, split, legacy} {
		if err := store.InsertSolidBody(sb); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListSolidBodiesBySource("source/v1/frame")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !reflect.DeepEqual(got[0], occluded) || !reflect.DeepEqual(got[1], split) || !reflect.DeepEqual(got[2], legacy) {
		t.Fatalf("support detail did not round-trip:\n got %+v", got)
	}
	if got[2].Reading.Estimate.Support.Instant != l5tracks.SupportUnrecorded {
		t.Fatalf("an unrecorded token read back as %s", got[2].Reading.Estimate.Support.Instant)
	}

	if _, err := database.Exec(`UPDATE lidar_track_solid_bodies SET support_instant = 'seen' WHERE estimate_id = ?`, legacy.EstimateID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListSolidBodiesBySource("source/v1/frame"); err == nil || !strings.Contains(err.Error(), "support token") {
		t.Fatalf("an unknown support token: error %v", err)
	}
}

func TestSolidBodyStoreRefusesWhatItCouldNotReadBack(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	cases := map[string]func(*TrackSolidBody){
		"unknown state model": func(sb *TrackSolidBody) { sb.Reading.Estimate.StateModel = "ctrv_v1" },
		"stage mismatch":      func(sb *TrackSolidBody) { sb.Stage = "final" },
		"covariance from another frame": func(sb *TrackSolidBody) {
			sb.Reading.Covariance[0] = 9
		},
		"missing observation": func(sb *TrackSolidBody) { sb.ObservationID = "" },
		"rank out of range":   func(sb *TrackSolidBody) { sb.Reading.Measurement.Rank = 3 },
	}
	for name, mutate := range cases {
		sb := testSolidBody("observation/v1/refused", 100)
		mutate(&sb)
		if err := store.InsertSolidBody(sb); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM lidar_track_solid_bodies`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d refused rows were written", n)
	}
}

func TestSolidBodyReaderRefusesUnknownLayoutsAndNames(t *testing.T) {
	cases := map[string]string{
		"state model": `UPDATE lidar_track_solid_bodies SET state_model = 'cv_polar_v9'`,
		"provenance":  `UPDATE lidar_track_solid_bodies SET width_provenance = 'measured'`,
		"estimation":  `UPDATE lidar_track_solid_bodies SET estimation_state = 'settled'`,
		"faces":       `UPDATE lidar_track_solid_bodies SET visible_faces = 'front,roof'`,
	}
	for name, corrupt := range cases {
		database, cleanup := setupTestDB(t)
		store := NewStateEstimateStore(database)
		if err := store.InsertSolidBody(testSolidBody("observation/v1/corrupt", 100)); err != nil {
			cleanup()
			t.Fatal(err)
		}
		if _, err := database.Exec(corrupt); err != nil {
			cleanup()
			t.Fatalf("%s: %v", name, err)
		}
		_, err := store.ListSolidBodiesBySource("source/v1/frame")
		cleanup()
		if err == nil {
			t.Errorf("%s: a row the reader cannot interpret was returned", name)
		}
	}

	// A stage outside the estimates' vocabulary has no in-memory stage. The
	// source-wide reader reads online rows only, so the version reader,
	// which reads any stage it is asked for, is the one that must refuse it.
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	sb := testSolidBody("observation/v1/corrupt", 100)
	if err := store.InsertSolidBody(sb); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE lidar_track_solid_bodies SET stage = 'draft'`); err != nil {
		t.Fatal(err)
	}
	key := EstimateVersionKey{SourceID: sb.SourceID, EstimatorID: sb.EstimatorID, ObservationModelID: sb.ObservationModelID,
		ParamHash: sb.ParamHash, Stage: "draft"}
	if _, err := store.ListVersionSolidBodies(key); err == nil || !strings.Contains(err.Error(), "no in-memory estimate stage") {
		t.Errorf("an unknown stage: error %v", err)
	}
}

func TestPointEstimatesDeclareTheirLayoutAndRefuseAnother(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	pair := testFrameStateEstimate("observation/v1/layout", 100)
	if err := store.Insert(pair.Estimate, pair.Residual); err != nil {
		t.Fatal(err)
	}
	var stateModel string
	if err := database.QueryRow(`SELECT state_model FROM lidar_track_estimates`).Scan(&stateModel); err != nil {
		t.Fatal(err)
	}
	if stateModel != l5tracks.StateModelCVCartesianV1 {
		t.Fatalf("point estimate written with state model %q", stateModel)
	}
	if _, err := database.Exec(`UPDATE lidar_track_estimates SET state_model = 'ca_cartesian_v2'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListBySource("source/v1/frame"); err == nil || !strings.Contains(err.Error(), "state model") {
		t.Fatalf("a six-state row was decoded as a 4x4: %v", err)
	}
}

// existingEstimateReads captures everything the pre-existing readers of
// lidar_track_estimates return for a source.
func existingEstimateReads(t *testing.T, database DBClient) string {
	t.Helper()
	store := NewStateEstimateStore(database)
	bySource, err := store.ListBySource("source/v1/frame")
	if err != nil {
		t.Fatal(err)
	}
	frames, err := store.ListFrameStateEstimatesBySource("source/v1/frame")
	if err != nil {
		t.Fatal(err)
	}
	versions, err := store.ListEstimateVersions()
	if err != nil {
		t.Fatal(err)
	}
	var positions []EstimatePosition
	for _, v := range versions {
		p, err := store.ListEstimatePositions(v)
		if err != nil {
			t.Fatal(err)
		}
		positions = append(positions, p...)
	}
	return fmt.Sprintf("%+v\n%+v\n%+v\n%+v", bySource, frames, versions, positions)
}

func TestExistingEstimateReadersNeverSeeSolidBodies(t *testing.T) {
	// The same frames written twice, once with solid bodies beside the point
	// estimates: every existing reader must return exactly the same thing.
	write := func(withSolidBodies bool) string {
		database, cleanup := setupTrackingPipelineTestDB(t)
		defer cleanup()
		store := NewFrameEvidenceStore(database)
		defer store.Close()
		for frame := int64(100); frame <= 300; frame += 100 {
			observation := testObservation(t, fmt.Sprintf("observation/v1/readers/%d", frame), "source/v1/frame", frame, frame)
			pair := testFrameStateEstimate(observation.Snapshot().ObservationID, frame)
			if withSolidBodies {
				sb := testSolidBody(observation.Snapshot().ObservationID, frame)
				pair.SolidBody = &sb
			}
			if err := store.InsertFrame([]l4bobserve.DetectionObservation{observation}, []FrameStateEstimate{pair}); err != nil {
				t.Fatal(err)
			}
		}
		if withSolidBodies {
			bodies, err := NewStateEstimateStore(database).ListSolidBodiesBySource("source/v1/frame")
			if err != nil {
				t.Fatal(err)
			}
			if len(bodies) != 3 {
				t.Fatalf("%d solid bodies written for three frames", len(bodies))
			}
		}
		return existingEstimateReads(t, database)
	}
	without, with := write(false), write(true)
	if without != with {
		t.Fatalf("existing readers changed when solid bodies were written beside the point estimates:\nwithout %s\n   with %s", without, with)
	}
}

func TestFrameEvidenceStoreRollsBackTheFrameOnAnInvalidSolidBody(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	observation := testObservation(t, "observation/v1/bad-body", "source/v1/frame", 100, 1)
	pair := testFrameStateEstimate(observation.Snapshot().ObservationID, 100)
	sb := testSolidBody(observation.Snapshot().ObservationID, 100)
	sb.Reading.Estimate.StateModel = "ctrv_v1"
	pair.SolidBody = &sb
	if err := NewFrameEvidenceStore(database).InsertFrame([]l4bobserve.DetectionObservation{observation}, []FrameStateEstimate{pair}); err == nil {
		t.Fatal("accepted a solid body with an unreadable state model")
	}
	for _, table := range []string{"lidar_observations", "lidar_track_estimates", "lidar_track_residuals", "lidar_track_solid_bodies"} {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after a rolled-back frame = %d, want 0", table, count)
		}
	}
}

func TestFrameEvidenceStorePreparesTheSolidBodyStatementOnlyWhenUsed(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	countingDB := &countingPrepareDB{DB: database}
	store := NewFrameEvidenceStore(countingDB)
	for frame := int64(100); frame <= 300; frame += 100 {
		observation := testObservation(t, fmt.Sprintf("observation/v1/lazy/%d", frame), "source/v1/frame", frame, frame)
		pair := testFrameStateEstimate(observation.Snapshot().ObservationID, frame)
		if frame > 100 {
			sb := testSolidBody(observation.Snapshot().ObservationID, frame)
			pair.SolidBody = &sb
		}
		if err := store.InsertFrame([]l4bobserve.DetectionObservation{observation}, []FrameStateEstimate{pair}); err != nil {
			t.Fatal(err)
		}
		want := 3
		if frame > 100 {
			want = 4
		}
		if countingDB.prepares != want {
			t.Fatalf("frame %d: %d statements prepared, want %d", frame, countingDB.prepares, want)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
