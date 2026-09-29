package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func defaultSurvey(setPath string) surveyFlags {
	return surveyFlags{setPath: setPath, percentile: replayeval.DefaultCoverageRangePercentile, gap: replayeval.DefaultCoverageMinSectorGapDeg}
}

// A survey measures the default replay, from an evidence database, into a
// set that does not already declare the case; anything else is refused
// before a case replays.
func TestSurveyFlagsRefuseAnythingButTheDefaultReplay(t *testing.T) {
	if err := (surveyFlags{}).validate([]string{"solid_body"}, "obb_centre_v1", true, "", nil); err != nil {
		t.Fatalf("no survey requested, yet refused: %v", err)
	}
	setPath := filepath.Join(t.TempDir(), "coverage.json")
	ok := defaultSurvey(setPath)
	for _, tc := range []struct {
		name          string
		f             surveyFlags
		experiments   []string
		mode          string
		surfaceGround bool
		dbPath        string
		want          string
	}{
		{"experiments", ok, []string{"solid_body"}, "medoid_v0", false, "e.db", "drop -experiment"},
		{"measurement mode", ok, nil, "obb_centre_v1", false, "e.db", "-measurement-mode must be medoid_v0"},
		{"surface ground", ok, nil, "medoid_v0", true, "e.db", "drop -surface-ground"},
		{"no evidence", ok, nil, "medoid_v0", false, "", "give -evidence-dir"},
		{"percentile", surveyFlags{setPath: setPath, percentile: 0, gap: 90}, nil, "medoid_v0", false, "e.db", "percentile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.f.validate(tc.experiments, tc.mode, tc.surfaceGround, tc.dbPath, []string{"kirk0"}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
	if err := ok.validate(nil, "medoid_v0", false, "e.db", []string{"kirk0"}); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]replayeval.ContinuityCoverage{"kirk0": {Source: "hand", MaxRangeMetres: 92, AzimuthHalfWidthDeg: 180}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(setPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ok.validate(nil, "medoid_v0", false, "e.db", []string{"kirk0"}); err == nil || !strings.Contains(err.Error(), "already declared") {
		t.Fatalf("a case already in the set: %v", err)
	}
}

// seedReplay writes a default replay's manifest and an evidence database
// holding its online estimates, one per range, a quarter-turn apart.
func seedReplay(t *testing.T, ranges ...float32) (replayDir, dbPath string) {
	t.Helper()
	replayDir = t.TempDir()
	const source, params = "source/v1/test", "sha256:params"
	manifest, err := json.Marshal(map[string]any{
		"source_sha256s": []string{"sha256:" + strings.Repeat("c", 64)}, "params_sha256": params,
		"build_version": "dev", "build_git_sha": "unknown", "measurement_source_mode": "medoid_v0",
		"observation_source_id": source, "frames_recorded": len(ranges),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replayDir, "replay_manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath = filepath.Join(replayDir, "evidence.db")
	database, err := db.NewDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for i, r := range ranges {
		obs, id := fmt.Sprintf("observation/%d", i), fmt.Sprintf("estimate/%d", i)
		if _, err := database.Exec(`INSERT INTO lidar_observations
			(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
			 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
			VALUES (?, 1, ?, 'calibration/test', 'sensor', 'frame', ?, ?, 1, '{}', 1)`, obs, source, i, i); err != nil {
			t.Fatal(err)
		}
		x, y := r, float32(0)
		if i%2 == 1 {
			x, y = 0, r
		}
		e := sqlite.TrackEstimate{EstimateID: id, TrackID: "track", ObservationID: obs, SourceID: source, CalibrationID: "calibration/test",
			FrameUnixNanos: int64(i), EstimatorID: "cv_kf_v1", ObservationModelID: "medoid_v0", ParamHash: params,
			Stage: "online", MeasurementSource: "medoid_v0", X: x, Y: y,
			Reference: l5tracks.ReferenceClusterMedoid, Support: l5tracks.SupportObserved}
		r := sqlite.TrackResidual{EstimateID: id, ObservationID: obs, Disposition: "accepted", Reason: "association_accepted"}
		if err := sqlite.InsertStateEstimate(database, e, r); err != nil {
			t.Fatal(err)
		}
	}
	return replayDir, dbPath
}

// The survey reads the first run's evidence and adds the case to the set; a
// second survey of the case, a missing or unreadable database and a replay
// without a manifest are refused.
func TestSurveyAddsTheCaseToTheSet(t *testing.T) {
	replayDir, dbPath := seedReplay(t, 12.5, 40.25)
	setPath := filepath.Join(t.TempDir(), "coverage.json")
	f := defaultSurvey(setPath)
	s, err := f.survey(dbPath, replayDir, "site")
	if err != nil {
		t.Fatal(err)
	}
	if s.Declaration.MaxRangeMetres != 41 || s.Stats.Tool != surveyTool || !strings.Contains(s.Declaration.Source, "measured by "+surveyTool) {
		t.Fatalf("survey %+v", s)
	}
	set, err := replayeval.LoadContinuityCoverageSet(setPath)
	if err != nil || set["site"] != s.Declaration {
		t.Fatalf("set %+v, %v", set, err)
	}
	if _, err := f.survey(dbPath, replayDir, "site"); err == nil || !strings.Contains(err.Error(), "already declared") {
		t.Fatalf("a second survey of one case: %v", err)
	}
	if _, err := f.survey(filepath.Join(replayDir, "missing.db"), replayDir, "other"); err == nil || !strings.Contains(err.Error(), "evidence database") {
		t.Fatalf("a missing database: %v", err)
	}
	garbage := filepath.Join(t.TempDir(), "garbage.db")
	if err := os.WriteFile(garbage, []byte("not a database, not at all, not even close to one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.survey(garbage, replayDir, "other"); err == nil {
		t.Fatal("a file that is not a database was surveyed")
	}
	if _, err := f.survey(dbPath, t.TempDir(), "other"); err == nil || !strings.Contains(err.Error(), "replay_manifest.json") {
		t.Fatalf("a replay without a manifest: %v", err)
	}
}
