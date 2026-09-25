package fieldrun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
// covariance.
func seed(t *testing.T, database sqlite.DBClient, trajectories []l8behaviour.Trajectory, v version) {
	t.Helper()
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
				Covariance: cov,
			}
			r := sqlite.TrackResidual{EstimateID: id, ObservationID: obs, Disposition: "accepted", Reason: "association_accepted"}
			if err := sqlite.InsertStateEstimate(database, e, r); err != nil {
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

// TestRunOnlineEstimates takes the steady approach's poses, persisted as
// online estimates, through the whole slice. The rows carry no heading,
// extent or class, so the pair is found on its path and every instant is
// suppressed, first for want of a class: an integration result that shows
// its missing evidence rather than a number.
func TestRunOnlineEstimates(t *testing.T) {
	database := openDB(t)
	seed(t, database, steadyApproach(), online)
	res := run(t, database, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline})

	if res.Estimates.Stage != "online" || res.Estimates.EstimatorID != "cv_kf_v1" || len(res.Trajectories) != 2 {
		t.Fatalf("read %+v, %d trajectories", res.Estimates, len(res.Trajectories))
	}
	wantVersion := l8behaviour.InteractionVersion{
		EstimateStage: l8behaviour.StageOnline, EstimatorID: "cv_kf_v1", ObsModelID: "obb_centre_v1",
		MethodID: l8behaviour.FollowingEncounterMethodID + "/" + Params().Hash(), ParamHash: "sha256:online",
	}
	if res.Version != wantVersion || res.AlreadyStored != 0 {
		t.Errorf("version %+v, already stored %d", res.Version, res.AlreadyStored)
	}
	if len(res.Analysis.Encounters) != 1 || len(res.Interactions) != 1 {
		t.Fatalf("%d encounters, %d interactions; want the one pair", len(res.Analysis.Encounters), len(res.Interactions))
	}

	s := res.Summary()
	if s.Tracks != 2 || s.PathsFitted != 2 || s.Encounters != 1 || s.ValidNanos != 0 || s.Written != 1 {
		t.Errorf("summary = %+v", s)
	}
	class := s.Suppressed[l8behaviour.ReasonClassNotSupported]
	if len(s.Suppressed) != 1 || class.Nanos != s.AccountedNanos || class.Nanos == 0 {
		t.Errorf("suppressed = %+v of %d ns; want every accounted second under class_not_supported", s.Suppressed, s.AccountedNanos)
	}
	for _, r := range []l8behaviour.SuppressionReason{l8behaviour.ReasonClassNotSupported,
		l8behaviour.ReasonOrientationUnresolved, l8behaviour.ReasonEstimateNotFinal} {
		if s.InstantReasons[r] != s.EvaluatedInstants || s.EvaluatedInstants == 0 {
			t.Errorf("%s applied at %d of %d evaluated instants, want all", r, s.InstantReasons[r], s.EvaluatedInstants)
		}
	}
	if lines := strings.Join(s.Lines(), "\n"); !strings.Contains(lines, "Encounters: 1 (1 written, 0 already stored)") ||
		!strings.Contains(lines, "suppressed class_not_supported") {
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
		c.Source != "lidar_track_estimates/"+testSource+"/cv_kf_v1/obb_centre_v1/sha256:online/online" ||
		!strings.Contains(c.Description, "with a resolved heading: 0") || !strings.Contains(c.Description, "rigid vehicles: 0") {
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
	seed(t, database, steadyApproach(), online)
	spec := Spec{SourceID: testSource, Stage: l8behaviour.StageOnline}
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
// are separate versions. A final run reads the final rows and its report
// reads the production block; a fixed_lag stage with several horizons must
// be narrowed to one.
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
	r, err := Report(final)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != headway.StatusProvisional || r.Encounters[0].ValueBlock != headway.ValueBlockMeasurements {
		t.Errorf("a final run's report is %s reading %s; want provisional reading measurements",
			r.Status, r.Encounters[0].ValueBlock)
	}

	_, err = Run(database, Spec{SourceID: testSource, Stage: l8behaviour.StageFixedLag})
	if err == nil || !strings.Contains(err.Error(), "holds 2 fixed_lag estimate versions") ||
		!strings.Contains(err.Error(), "sha256:lag_0.5s") {
		t.Errorf("ambiguous fixed_lag: error %v", err)
	}
	lag := run(t, database, Spec{SourceID: testSource, Stage: l8behaviour.StageFixedLag, ParamHash: "sha256:lag_1s"})
	if lag.Version.EstimateStage != l8behaviour.StageFixedLag || lag.Version.ParamHash != "sha256:lag_1s" {
		t.Errorf("narrowed fixed_lag run version = %+v", lag.Version)
	}
	versions, err := sqlite.NewInteractionStore(database).Versions(testSource)
	if err != nil || len(versions) != 2 {
		t.Errorf("stored interaction versions = %+v (%v); want final and one fixed_lag, apart", versions, err)
	}
}

// TestRunWithoutAnEncounter: medoid-referenced rows (the production
// measurement model) are not on the body, so no follower's path can be
// fitted and there is no encounter. That is a result: the run stores
// nothing and the report still builds, listing every follower's refusal.
func TestRunWithoutAnEncounter(t *testing.T) {
	database := openDB(t)
	seed(t, database, steadyApproach(), version{"cv_kf_v1", "medoid_v0", "sha256:medoid", "online", "medoid_v0"})
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
	for _, f := range r.Captures[0].Followers {
		if len(f.PathConditions) != 1 || f.PathConditions[0] != l8behaviour.PathWeakSupport {
			t.Errorf("follower %s path conditions %v", f.TrackID, f.PathConditions)
		}
	}
	if err := l8behaviour.AuditSurfaceJSON(reportData(t, res)); err != nil {
		t.Error(err)
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
	for name, c := range map[string]struct {
		spec Spec
		want string
	}{
		"no source":      {Spec{Stage: l8behaviour.StageOnline}, "needs a source id"},
		"no stage":       {Spec{SourceID: testSource}, "needs an estimate stage"},
		"unknown source": {Spec{SourceID: "source/v1/other", Stage: l8behaviour.StageOnline}, "no estimates for source"},
		"absent stage":   {Spec{SourceID: testSource, Stage: l8behaviour.StageFinal}, "holds no final estimates"},
		"no such hash":   {Spec{SourceID: testSource, Stage: l8behaviour.StageOnline, ParamHash: "sha256:x"}, "sha256:online"},
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
	run(t, database, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline})
	if _, err := database.Exec(`DELETE FROM lidar_track_estimates WHERE track_id = 'trk_s_steady_leader'`); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(database, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline}); err == nil ||
		!strings.Contains(err.Error(), "changed without a version change") {
		t.Errorf("foreign stored events: error %v", err)
	}
}
