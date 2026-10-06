package fieldrun

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/headway"
	"github.com/banshee-data/velocity.report/internal/report/typst"
	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

var testSource = "source/v1/" + strings.Repeat("ab", 32)

func openDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.NewDB(filepath.Join(t.TempDir(), "evidence.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// version names one estimate version to seed.
type version struct {
	estimator, obsModel, paramHash, stage, measurement string
}

var online = version{"cv_kf_v1", "obb_centre_v1", "sha256:online", "online", "obb_centre_v1"}

// seed writes a scenario's poses as persisted estimates of one version, each
// linked to an immutable observation row, as a replay's evidence store
// would. Only what a row carries is taken: position, velocity and
// covariance. Each row states the reference a filter measured from the
// version's geometry states, and observed support, as the online sink does.
func seed(t *testing.T, database sqlite.DBClient, trajectories []l8behaviour.Trajectory, v version) {
	t.Helper()
	measured := l5tracks.TrackedObject{}
	measured.ObservationCount = 1
	measured.LastMeasurementSource = l5tracks.MeasurementSource(v.measurement)
	for _, tr := range trajectories {
		for _, s := range tr.Samples {
			obs := fmt.Sprintf("observation/%s/%d", tr.Passage.TrackID, s.CaptureUnixNanos)
			if _, err := database.Exec(`INSERT OR IGNORE INTO lidar_observations
				(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
				 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
				VALUES (?, 1, ?, 'calibration/test', 'sensor_test', 'frame', ?, ?, 1, '{}', 1)`,
				obs, testSource, s.CaptureUnixNanos, s.CaptureUnixNanos); err != nil {
				t.Fatal(err)
			}
			id := fmt.Sprintf("estimate/%s/%s/%s/%s/%d", tr.Passage.TrackID, v.estimator, v.paramHash, v.stage, s.CaptureUnixNanos)
			var cov [16]float32
			for i, c := range s.Covariance {
				cov[i] = float32(c)
			}
			e := sqlite.TrackEstimate{
				EstimateID: id, TrackID: tr.Passage.TrackID, ObservationID: obs, SourceID: testSource,
				CalibrationID: "calibration/test", FrameUnixNanos: s.CaptureUnixNanos, MeasurementUnixNanos: s.CaptureUnixNanos,
				EstimatorID: v.estimator, ObservationModelID: v.obsModel, ParamHash: v.paramHash, Stage: v.stage,
				MeasurementSource: v.measurement, X: float32(s.X), Y: float32(s.Y), VX: float32(s.VX), VY: float32(s.VY),
				Covariance: cov, Reference: measured.PositionReference(), Support: l5tracks.SupportObserved,
			}
			r := sqlite.TrackResidual{EstimateID: id, ObservationID: obs, Disposition: "accepted", Reason: "association_accepted"}
			if err := sqlite.InsertStateEstimate(database, e, r); err != nil {
				t.Fatal(err)
			}
		}
	}
}

var onlineBodies = version{"cv_kf_v1", string(l5tracks.MeasurementNearEdgeCandidateV1), "sha256:online", "online",
	string(l5tracks.MeasurementNearEdgeCandidateV1)}

// seedSolidBodies writes a scenario's samples as persisted solid bodies of
// one version, as the online sink files a near-edge body beside each point
// estimate: the reference, heading, extents, faces and class the samples
// carry, each row linked to an immutable observation.
func seedSolidBodies(t *testing.T, database sqlite.DBClient, trajectories []l8behaviour.Trajectory, v version) {
	t.Helper()
	provenance := func(p l8behaviour.BeliefProvenance) l5tracks.Provenance {
		if !p.Valid() {
			return l5tracks.ProvenanceNone
		}
		out, err := l5tracks.ParseProvenance(p.String())
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	dimension := func(e l8behaviour.ExtentBelief) l5tracks.DimensionBelief {
		return l5tracks.DimensionBelief{Metres: float32(e.Metres), SigmaMetres: float32(e.SigmaMetres),
			AdmissibleFrames: e.AdmissibleFrames, Provenance: provenance(e.Provenance)}
	}
	for _, tr := range trajectories {
		class, err := l5tracks.ParseMotionClass(tr.Passage.MotionClass.String())
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range tr.Samples {
			obs := fmt.Sprintf("observation/%s/%d", tr.Passage.TrackID, s.CaptureUnixNanos)
			if _, err := database.Exec(`INSERT OR IGNORE INTO lidar_observations
				(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
				 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
				VALUES (?, 1, ?, 'calibration/test', 'sensor_test', 'frame', ?, ?, 1, '{}', 1)`,
				obs, testSource, s.CaptureUnixNanos, s.CaptureUnixNanos); err != nil {
				t.Fatal(err)
			}
			reference, err := l5tracks.ParseReferencePoint(s.Reference.String())
			if err != nil {
				t.Fatal(err)
			}
			estimation, err := l5tracks.ParseEstimationState(s.Estimation.String())
			if err != nil {
				t.Fatal(err)
			}
			var faces []string
			if s.Faces.FrontObserved {
				faces = append(faces, "front")
			}
			if s.Faces.RearObserved {
				faces = append(faces, "rear")
			}
			visible, err := l5tracks.ParseVisibleFaces(strings.Join(faces, ","))
			if err != nil {
				t.Fatal(err)
			}
			var cov [16]float32
			for i, c := range s.Covariance {
				cov[i] = float32(c)
			}
			// Acquired a few milliseconds into the frame, as a cluster is.
			acquired := s.CaptureUnixNanos + 3_000_000
			sb := sqlite.TrackSolidBody{
				EstimateID: fmt.Sprintf("solid_body/%s/%s/%s/%d", tr.Passage.TrackID, v.estimator, v.paramHash, s.CaptureUnixNanos),
				TrackID:    tr.Passage.TrackID, ObservationID: obs, SourceID: testSource, CalibrationID: "calibration/test",
				FrameUnixNanos: s.CaptureUnixNanos, MeasurementUnixNanos: acquired,
				EstimatorID: v.estimator, ObservationModelID: v.obsModel, ParamHash: v.paramHash, Stage: v.stage,
				Reading: l5tracks.SolidBodyReading{
					Estimate: l5tracks.SolidBodyEstimate{
						StateModel: l5tracks.StateModelCVCartesianV1, Reference: reference,
						X: float32(s.X), Y: float32(s.Y), PositionCovariance: [4]float32{cov[0], cov[1], cov[4], cov[5]},
						Orientation: l5tracks.OrientationBelief{PsiRad: float32(s.Heading.Rad),
							VarianceRad2: float32(s.Heading.VarianceRad2), Provenance: provenance(s.Heading.Provenance)},
						Length: dimension(s.Length), Width: dimension(s.Width),
						Motion:     l5tracks.MotionClassBelief{Class: class, Posterior: 0.9},
						Estimation: estimation, Stage: l5tracks.StageLive, LastObservedUnixNanos: acquired,
						Support: l5tracks.SupportState{PointCount: 40, Instant: l5tracks.SupportObserved},
					},
					VX: float32(s.VX), VY: float32(s.VY), Covariance: cov,
					Measurement: l5tracks.SolidBodyMeasurement{Source: l5tracks.MeasurementNearEdgeCandidateV1,
						Rank: len(faces), Faces: visible},
				},
			}
			if err := sqlite.InsertSolidBody(database, sb); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func steadyApproach() []l8behaviour.Trajectory {
	return l8behaviour.ScenarioSteadyApproach().Trajectories
}

func run(t *testing.T, database sqlite.DBClient, spec Spec) Result {
	t.Helper()
	res, err := Run(database, spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

// reportData is the report's data.json as the renderer writes it.
func reportData(t *testing.T, res Result) []byte {
	t.Helper()
	r, err := Report(res)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	rendered, _, err := headway.Assemble(r, chart.PaperA4)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	data, err := typst.MarshalData(rendered)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestRunOnlineSolidBodies takes the steady approach's samples, persisted as
// online solid bodies, through the whole slice. The rows carry a body-centre
// reference, heading, extents and class, so the pair is found on its path and
// every instant is valid following time. Each value stays provisional,
// because the stage is online: the one reason left is estimate_not_final.
func TestRunOnlineSolidBodies(t *testing.T) {
	database := openDB(t)
	seedSolidBodies(t, database, steadyApproach(), onlineBodies)
	res := run(t, database, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline, SolidBodies: true})

	if !res.SolidBodies || res.Estimates.Stage != "online" || res.Estimates.EstimatorID != "cv_kf_v1" || len(res.Trajectories) != 2 {
		t.Fatalf("read %+v, %d trajectories", res.Estimates, len(res.Trajectories))
	}
	for _, tr := range res.Trajectories {
		if tr.Passage.MotionClass != l8behaviour.MotionRigidVehicle {
			t.Errorf("%s class %s, want the rows' rigid_vehicle", tr.Passage.TrackID, tr.Passage.MotionClass)
		}
		for _, s := range tr.Samples {
			if s.Reference != l8behaviour.ReferenceBodyCentre || !s.Heading.Resolved() || !s.Length.Converged ||
				s.LastObservedUnixNanos != s.CaptureUnixNanos || s.AcquisitionUnixNanos != s.CaptureUnixNanos+3_000_000 {
				t.Fatalf("%s sample %+v", tr.Passage.TrackID, s)
			}
		}
	}
	wantVersion := l8behaviour.InteractionVersion{
		EstimateStage: l8behaviour.StageOnline, EstimatorID: "cv_kf_v1", ObsModelID: onlineBodies.obsModel,
		MethodID: l8behaviour.FollowingEncounterMethodID + "/" + Params().Hash(), ParamHash: "sha256:online",
	}
	if res.Version != wantVersion || res.AlreadyStored != 0 {
		t.Errorf("version %+v, already stored %d", res.Version, res.AlreadyStored)
	}
	if len(res.Analysis.Encounters) != 1 || len(res.Interactions) != 1 {
		t.Fatalf("%d encounters, %d interactions; want the one pair", len(res.Analysis.Encounters), len(res.Interactions))
	}

	s := res.Summary()
	if s.Tracks != 2 || s.PathsFitted != 2 || s.Encounters != 1 || s.ValidNanos == 0 || s.ValidNanos != s.AccountedNanos ||
		len(s.Suppressed) != 0 || s.Written != 1 {
		t.Errorf("summary = %+v", s)
	}
	if len(s.InstantReasons) != 1 || s.InstantReasons[l8behaviour.ReasonEstimateNotFinal] != s.EvaluatedInstants || s.EvaluatedInstants == 0 {
		t.Errorf("instant reasons %v at %d evaluated instants; want only estimate_not_final, at all", s.InstantReasons, s.EvaluatedInstants)
	}
	if lines := strings.Join(s.Lines(), "\n"); !strings.Contains(lines, "Encounters: 1 (1 written, 0 already stored)") ||
		!strings.Contains(lines, "estimate_not_final: 50 (100%)") {
		t.Errorf("summary lines:\n%s", lines)
	}

	store := sqlite.NewInteractionStore(database)
	events, err := store.ListEvents(testSource, res.Version)
	if err != nil || len(events) != 1 {
		t.Fatalf("stored events = %d (%v)", len(events), err)
	}
	fi, err := store.Get(events[0].EventID)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Event.SourceID != testSource || fi.Event.Version.EstimateStage != l8behaviour.StageOnline ||
		len(fi.Event.Provisional) == 0 || len(fi.Instants) != len(res.Analysis.Encounters[0].Instants) {
		t.Errorf("stored interaction = %+v with %d instants", fi.Event, len(fi.Instants))
	}
	for _, v := range []any{fi.Event, fi.Instants, fi.Windows} {
		if err := l8behaviour.AuditSurfaceJSON(mustJSON(t, v)); err != nil {
			t.Errorf("persisted rows fail the surface audit: %v", err)
		}
	}

	r, err := Report(res)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != headway.StatusProvisional || r.StatusLabel != "PROVISIONAL" || len(r.Encounters) != 1 ||
		r.Encounters[0].ValueBlock != headway.ValueBlockProvisional {
		t.Errorf("report status %s %q, encounters %+v", r.Status, r.StatusLabel, r.Encounters)
	}
	c := r.Captures[0]
	if c.ID != "source_abababababab" ||
		c.Source != "lidar_track_solid_bodies/"+testSource+"/cv_kf_v1/near_edge_candidate_v1/sha256:online/online" ||
		!strings.Contains(c.Description, "Persisted online solid bodies: 2 tracks, 100 samples") ||
		!strings.Contains(c.Description, "body-centre reference: 100") || !strings.Contains(c.Description, "rigid vehicles: 2") ||
		strings.Contains(c.Description, testSource) || strings.Contains(c.Description, "sha256:") {
		t.Errorf("capture = %s, %s, %q", c.ID, c.Source, c.Description)
	}
	data := reportData(t, res)
	if err := l8behaviour.AuditSurfaceJSON(data); err != nil {
		t.Errorf("data.json fails the surface audit: %v", err)
	}
	if m := headway.VerdictPattern.FindAll(data, -1); len(m) > 0 {
		t.Errorf("data.json prints %q", m)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// tableRows reads every stored interaction row, payload and insertion time,
// in a total order.
func tableRows(t *testing.T, database sqlite.DBClient) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT event_id || ' ' || event_json || ' ' || inserted_at_ns FROM lidar_interaction_events ORDER BY event_id`,
		`SELECT event_id || ' ' || capture_unix_nanos || ' ' || instant_json FROM lidar_interaction_instants ORDER BY event_id, capture_unix_nanos`,
		`SELECT window_id || ' ' || window_json || ' ' || inserted_at_ns FROM lidar_exposure_windows ORDER BY window_id`,
	} {
		rows, err := database.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			b.WriteString(s + "\n")
		}
		rows.Close()
	}
	return b.String()
}

// TestRunIsReproducible: a second run of the same version on the same
// database writes nothing (the stored rows, insertion times included, are
// untouched) and builds a byte-identical data.json.
func TestRunIsReproducible(t *testing.T) {
	database := openDB(t)
	seedSolidBodies(t, database, steadyApproach(), onlineBodies)
	spec := Spec{SourceID: testSource, Stage: l8behaviour.StageOnline, SolidBodies: true}
	first := run(t, database, spec)
	rowsAfterFirst := tableRows(t, database)
	dataFirst := reportData(t, first)

	second := run(t, database, spec)
	if second.AlreadyStored != len(second.Interactions) || second.Summary().Written != 0 {
		t.Errorf("second run already stored %d of %d", second.AlreadyStored, len(second.Interactions))
	}
	if rowsAfterSecond := tableRows(t, database); rowsAfterSecond != rowsAfterFirst || rowsAfterFirst == "" {
		t.Error("a second run of the same version changed the stored rows")
	}
	if dataSecond := reportData(t, second); !bytes.Equal(dataFirst, dataSecond) {
		t.Error("a second run built a different data.json")
	}
}

// TestRunReadsRefinedStages: fixed_lag and final rows beside the online ones
// are separate versions. A final run reads the final rows, and a fixed_lag
// stage with several horizons must be narrowed to one. The rows' geometry is
// the visible box centre at every stage: a smoother revises the state it was
// given, and no stage makes that state a place on the body, so no follower's
// path is fitted from a final row either (review R1).
func TestRunReadsRefinedStages(t *testing.T) {
	database := openDB(t)
	trajectories := steadyApproach()
	refined := "cv_kf_v1+" + l5tracks.SmootherID
	seed(t, database, trajectories, online)
	seed(t, database, trajectories, version{refined, "obb_centre_v1", "sha256:lag_0.5s", "fixed_lag", "obb_centre_v1"})
	seed(t, database, trajectories, version{refined, "obb_centre_v1", "sha256:lag_1s", "fixed_lag", "obb_centre_v1"})
	seed(t, database, trajectories, version{refined, "obb_centre_v1", "sha256:track_end", "final", "obb_centre_v1"})

	final := run(t, database, Spec{SourceID: testSource, Stage: l8behaviour.StageFinal})
	if final.Estimates.Stage != "final" || final.Version.EstimateStage != l8behaviour.StageFinal ||
		final.Version.EstimatorID != refined || final.Version.ParamHash != "sha256:track_end" {
		t.Errorf("final run read %+v at version %+v", final.Estimates, final.Version)
	}
	for _, tr := range final.Trajectories {
		for _, s := range tr.Samples {
			if s.Stage != l8behaviour.StageFinal || s.Reference != l8behaviour.ReferenceVisibleOBBCentre {
				t.Fatalf("final sample at %s refers to %s", s.Stage, s.Reference)
			}
		}
	}
	if s := final.Summary(); s.Encounters != 0 || s.PathsFitted != 0 || s.PathRefusals[l8behaviour.PathWeakSupport] != 2 {
		t.Errorf("a final run over visible box centres: %+v", s)
	}

	_, err := Run(database, Spec{SourceID: testSource, Stage: l8behaviour.StageFixedLag})
	if err == nil || !strings.Contains(err.Error(), "holds 2 fixed_lag versions of estimates") ||
		!strings.Contains(err.Error(), "sha256:lag_0.5s") {
		t.Errorf("ambiguous fixed_lag: error %v", err)
	}
	lag := run(t, database, Spec{SourceID: testSource, Stage: l8behaviour.StageFixedLag, ParamHash: "sha256:lag_1s"})
	if lag.Version.EstimateStage != l8behaviour.StageFixedLag || lag.Version.ParamHash != "sha256:lag_1s" {
		t.Errorf("narrowed fixed_lag run version = %+v", lag.Version)
	}
}

// TestRunWithoutAnEncounter: point-estimate rows under either measurement
// model refer to the medoid or the visible box centre, neither of which is on
// the body, so no follower's path can be fitted and there is no encounter.
// That is a result: the run stores nothing and the report still builds,
// listing every follower's refusal.
func TestRunWithoutAnEncounter(t *testing.T) {
	for _, v := range []version{
		{"cv_kf_v1", "medoid_v0", "sha256:medoid", "online", "medoid_v0"},
		online,
	} {
		t.Run(v.obsModel, func(t *testing.T) {
			database := openDB(t)
			seed(t, database, steadyApproach(), v)
			res := run(t, database, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline})
			s := res.Summary()
			if s.Encounters != 0 || s.PathsFitted != 0 || s.PathRefusals[l8behaviour.PathWeakSupport] != 2 {
				t.Errorf("summary = %+v", s)
			}
			r, err := Report(res)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Encounters) != 0 || len(r.Aggregates) != 0 || len(r.Captures[0].Followers) != 2 {
				t.Errorf("report has %d encounters, %d aggregates, %d followers", len(r.Encounters), len(r.Aggregates), len(r.Captures[0].Followers))
			}
			if c := r.Captures[0]; !strings.Contains(c.Description, "body-centre reference: 0") ||
				!strings.HasPrefix(c.Source, "lidar_track_estimates/") {
				t.Errorf("capture = %s, %q", c.Source, c.Description)
			}
			for _, f := range r.Captures[0].Followers {
				if len(f.PathConditions) != 1 || f.PathConditions[0] != l8behaviour.PathWeakSupport {
					t.Errorf("follower %s path conditions %v", f.TrackID, f.PathConditions)
				}
			}
			data := reportData(t, res)
			if err := l8behaviour.AuditSurfaceJSON(data); err != nil {
				t.Error(err)
			}
			// Empty lists, not null: the template reads their length.
			var doc map[string]json.RawMessage
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"encounters", "aggregates"} {
				if string(doc[key]) != "[]" {
					t.Errorf("data.json %s = %s, want []", key, doc[key])
				}
			}
		})
	}
}

// estimateReferences reads one version through the store and the adapter and
// returns each sample's reference, track by track in frame order.
func estimateReferences(t *testing.T, database sqlite.DBClient, key sqlite.EstimateVersionKey) ([]string, error) {
	t.Helper()
	trajectories, err := estimateTrajectories(sqlite.NewStateEstimateStore(database), key)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, tr := range trajectories {
		for _, s := range tr.Samples {
			out = append(out, s.Reference.String())
		}
	}
	return out, nil
}

// TestRunReadsTheStatedReference: every row here was measured at the OBB
// centre, and each sample refers to whatever its row's reference_point
// states, never what that measurement source would suggest. A row whose
// support is not observed is refused rather than read with a last observed
// time the row does not carry.
func TestRunReadsTheStatedReference(t *testing.T) {
	database := openDB(t)
	seed(t, database, steadyApproach(), online)
	key := sqlite.EstimateVersionKey{SourceID: testSource, EstimatorID: online.estimator, ObservationModelID: online.obsModel,
		ParamHash: online.paramHash, Stage: online.stage}
	for _, stated := range []l5tracks.ReferencePoint{l5tracks.ReferenceVisibleOBBCentre, l5tracks.ReferenceClusterMedoid,
		l5tracks.ReferenceBodyCentre} {
		if _, err := database.Exec(`UPDATE lidar_track_estimates SET reference_point = ?`, stated.String()); err != nil {
			t.Fatal(err)
		}
		references, err := estimateReferences(t, database, key)
		if err != nil {
			t.Fatalf("%s: %v", stated, err)
		}
		if len(references) == 0 {
			t.Fatal("no samples")
		}
		for i, got := range references {
			if got != stated.String() {
				t.Fatalf("stated %s: sample %d refers to %s", stated, i, got)
			}
		}
	}
	if _, err := database.Exec(`UPDATE lidar_track_estimates SET support_instant = 'coasted' WHERE frame_unix_nanos = ?`,
		l8behaviour.FixtureBaseUnixNanos); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(database, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline}); err == nil ||
		!strings.Contains(err.Error(), `support "coasted"`) {
		t.Errorf("a coasted row: error %v", err)
	}
}

// TestRunReadsAMigratedDatabaseAsBefore: rows written before migration
// 000057, when the adapter inferred the reference from measurement_source,
// read after it exactly as the adapter read them then. An OBB centre is the
// visible box centre, a medoid or the OBB model's medoid fallback is the
// cluster medoid, and every row is observed. A row whose source that mapping
// did not know was refused then and is refused now.
func TestRunReadsAMigratedDatabaseAsBefore(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	legacy := &db.DB{DB: sqlDB}
	migrations := os.DirFS(filepath.Join("..", "..", "..", "db", "migrations"))
	if err := legacy.MigrateTo(migrations, 56); err != nil {
		t.Fatal(err)
	}
	// One version under the OBB-centre model, whose rows alternate between the
	// OBB centre and its medoid fallback; one under the medoid model; one
	// whose measurement source no mapping knows.
	sources := map[string][]string{
		"obb_centre_v1":          {"obb_centre_v1", "medoid_fallback_v1"},
		"medoid_v0":              {"medoid_v0"},
		"near_edge_candidate_v1": {"near_edge_candidate_v1"},
	}
	for model, measured := range sources {
		for i, tr := range steadyApproach() {
			for j, s := range tr.Samples {
				obs := fmt.Sprintf("observation/%s/%s/%d", model, tr.Passage.TrackID, s.CaptureUnixNanos)
				if _, err := legacy.Exec(`INSERT INTO lidar_observations
					(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
					 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
					VALUES (?, 1, ?, 'calibration/test', 'sensor_test', 'frame', ?, ?, 1, '{}', 1)`,
					obs, testSource, s.CaptureUnixNanos, s.CaptureUnixNanos); err != nil {
					t.Fatal(err)
				}
				if _, err := legacy.Exec(`INSERT INTO lidar_track_estimates
					(estimate_id, track_id, creation_sequence, observation_id, source_id, calibration_id, frame_unix_nanos,
					 measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, measurement_source,
					 x, y, vx, vy, covariance_json, inserted_at_ns)
					VALUES (?, ?, ?, ?, ?, 'calibration/test', ?, ?, 'cv_kf_v1', ?, 'sha256:online', 'online', ?, ?, ?, ?, ?,
					        '[0.04,0,0,0,0,0.04,0,0,0,0,0.25,0,0,0,0,0.25]', 1)`,
					"estimate/"+obs, tr.Passage.TrackID, i+1, obs, testSource, s.CaptureUnixNanos, s.CaptureUnixNanos,
					model, measured[j%len(measured)], s.X, s.Y, s.VX, s.VY); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := legacy.MigrateUp(migrations); err != nil {
		t.Fatal(err)
	}
	key := func(model string) sqlite.EstimateVersionKey {
		return sqlite.EstimateVersionKey{SourceID: testSource, EstimatorID: "cv_kf_v1", ObservationModelID: model,
			ParamHash: "sha256:online", Stage: "online"}
	}
	obb, err := estimateReferences(t, legacy, key("obb_centre_v1"))
	if err != nil {
		t.Fatal(err)
	}
	medoid, err := estimateReferences(t, legacy, key("medoid_v0"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obb) == 0 || len(obb) != len(medoid) {
		t.Fatalf("%d and %d samples", len(obb), len(medoid))
	}
	// Each track's rows alternate from its first frame, and samples come
	// back track by track in frame order.
	var want []string
	for _, tr := range steadyApproach() {
		for j := range tr.Samples {
			want = append(want, []string{"visible_obb_centre", "cluster_medoid"}[j%2])
		}
	}
	if strings.Join(obb, ",") != strings.Join(want, ",") {
		t.Errorf("OBB-centre version reads %v, want %v", obb, want)
	}
	for i, r := range medoid {
		if r != "cluster_medoid" {
			t.Fatalf("medoid version sample %d refers to %s", i, r)
		}
	}
	if _, err := estimateReferences(t, legacy, key("near_edge_candidate_v1")); err == nil ||
		!strings.Contains(err.Error(), `unknown reference point ""`) {
		t.Errorf("a row whose measurement source no mapping knows: error %v", err)
	}
}

// typstAvailable reports whether typst is embedded or on PATH.
func typstAvailable() bool {
	_, err := exec.LookPath("typst")
	return err == nil || typstbin.Embedded()
}

// requireTypst skips when typst is neither embedded nor on PATH, as the
// oracle's compile test does, and never downloads it.
func requireTypst(t *testing.T) {
	t.Helper()
	t.Setenv(typstbin.EnvNoDownload, "1")
	if !typstAvailable() {
		t.Skip("typst not embedded or on PATH; run make install-typst and add bin/ to PATH")
	}
}

// TestGenerateCompilesFieldReports typesets a field report with an encounter
// and one without, through the real template: the second must still compile
// and say that nothing was found.
func TestGenerateCompilesFieldReports(t *testing.T) {
	src, err := typst.SourcesFor(typst.EntryHeadway)
	if err != nil {
		t.Fatal(err)
	}
	if tpl := string(src[typst.EntryHeadway]); !strings.Contains(tpl, "data.encounters.len() == 0") ||
		!strings.Contains(tpl, "data.aggregates.len() == 0") {
		t.Error("headway.typ does not state an empty encounter list")
	}
	requireTypst(t)
	for name, bodies := range map[string]bool{"with an encounter": true, "without one": false} {
		t.Run(name, func(t *testing.T) {
			database := openDB(t)
			if bodies {
				seedSolidBodies(t, database, steadyApproach(), onlineBodies)
			} else {
				seed(t, database, steadyApproach(), version{"cv_kf_v1", "medoid_v0", "sha256:medoid", "online", "medoid_v0"})
			}
			r, err := Report(run(t, database, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline, SolidBodies: bodies}))
			if err != nil {
				t.Fatal(err)
			}
			res, err := headway.Generate(r, headway.Options{Paper: chart.PaperA4, OutputDir: t.TempDir()})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			pdf, err := os.ReadFile(res.PDFPath)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(res.PDFPath) != "headway_provisional_report.pdf" || !bytes.HasPrefix(pdf, []byte("%PDF-")) ||
				!bytes.Contains(pdf, []byte("status:provisional")) {
				t.Errorf("%s: %d bytes, not a provisional PDF", res.PDFPath, len(pdf))
			}
		})
	}
}

// TestFieldDataIsNeverSyntheticOrPromoted: the run's capture cannot be
// labelled a synthetic oracle, and nothing can be promoted.
func TestFieldDataIsNeverSyntheticOrPromoted(t *testing.T) {
	database := openDB(t)
	seed(t, database, steadyApproach(), online)
	res := run(t, database, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline})
	capture := res.Capture()
	if _, err := headway.Build(headway.Input{Status: headway.StatusSyntheticOracle, Captures: []headway.CaptureInput{capture}}); err == nil ||
		!strings.Contains(err.Error(), "only analytic fixture trajectories") {
		t.Errorf("field data labelled synthetic: error %v", err)
	}
	if _, err := headway.Build(headway.Input{Status: headway.StatusPromoted, Captures: []headway.CaptureInput{capture}}); !errors.Is(err, headway.ErrPromotionGated) {
		t.Errorf("field data promoted: error %v", err)
	}
}

// TestRunRefuses: a missing source or stage, a source with no estimates, a
// frame period the record-gap bound cannot tell from a missing row, and a
// store holding events at the run's version that the run did not produce.
func TestRunRefuses(t *testing.T) {
	database := openDB(t)
	seed(t, database, steadyApproach(), online)
	seedSolidBodies(t, database, steadyApproach(), onlineBodies)
	for name, c := range map[string]struct {
		spec Spec
		want string
	}{
		"no source":      {Spec{Stage: l8behaviour.StageOnline}, "needs a source id"},
		"no stage":       {Spec{SourceID: testSource}, "needs an estimate stage"},
		"unknown source": {Spec{SourceID: "source/v1/other", Stage: l8behaviour.StageOnline}, "no estimates for source"},
		"absent stage":   {Spec{SourceID: testSource, Stage: l8behaviour.StageFinal}, "holds no final estimates"},
		"no such hash":   {Spec{SourceID: testSource, Stage: l8behaviour.StageOnline, ParamHash: "sha256:x"}, "sha256:online"},
		"no solid bodies": {Spec{SourceID: "source/v1/other", Stage: l8behaviour.StageOnline, SolidBodies: true},
			"no solid bodies for source"},
		"no final solid bodies": {Spec{SourceID: testSource, Stage: l8behaviour.StageFinal, SolidBodies: true},
			"holds no final solid bodies"},
	} {
		if _, err := Run(database, c.spec); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want %q", name, err, c.want)
		}
	}

	// 20 Hz: the 10 Hz scenario's rows re-timed 50 ms apart.
	fast := openDB(t)
	trajectories := steadyApproach()
	for i := range trajectories {
		for j := range trajectories[i].Samples {
			s := &trajectories[i].Samples[j]
			s.CaptureUnixNanos = l8behaviour.FixtureBaseUnixNanos + int64(j)*50_000_000
			s.LastObservedUnixNanos = s.CaptureUnixNanos
		}
	}
	seed(t, fast, trajectories, online)
	if _, err := Run(fast, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline}); err == nil ||
		!strings.Contains(err.Error(), "50 ms frame period") {
		t.Errorf("20 Hz rows: error %v", err)
	}

	// A version's events must be exactly this run's: remove the leader's
	// rows and the stored encounter is no longer one the analysis produces.
	bodies := Spec{SourceID: testSource, Stage: l8behaviour.StageOnline, SolidBodies: true}
	if res := run(t, database, bodies); len(res.Interactions) != 1 {
		t.Fatalf("%d interactions stored, want the one encounter", len(res.Interactions))
	}
	if _, err := database.Exec(`DELETE FROM lidar_track_solid_bodies WHERE track_id = 'trk_s_steady_leader'`); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(database, bodies); err == nil ||
		!strings.Contains(err.Error(), "changed without a version change") {
		t.Errorf("foreign stored events: error %v", err)
	}
}
