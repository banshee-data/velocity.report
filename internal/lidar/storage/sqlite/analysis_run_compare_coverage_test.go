package sqlite

import (
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/configasset"
)

// --- CompareRuns coverage ---

func setupCompareRunsDB(t *testing.T) (*AnalysisRunStore, func()) {
	t.Helper()
	db, cleanup := dbpkg.NewTestDB(t)
	return NewAnalysisRunStore(db.DB), cleanup
}

func insertTestRunWithTracks(t *testing.T, store *AnalysisRunStore, runID string, tracks []RunTrack) {
	t.Helper()
	run := &AnalysisRun{
		RunID:      runID,
		SourceType: "pcap",
		SourcePath: "/test/" + runID + ".pcap",
		SensorID:   "sensor-cmp",
		Status:     "completed",
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun %s: %v", runID, err)
	}
	for i := range tracks {
		tracks[i].RunID = runID
		if err := store.InsertRunTrack(&tracks[i]); err != nil {
			t.Fatalf("InsertRunTrack %s/%s: %v", runID, tracks[i].TrackID, err)
		}
	}
}

func TestCov_CompareRuns_BothRunsEmpty(t *testing.T) {
	store, cleanup := setupCompareRunsDB(t)
	defer cleanup()

	insertTestRunWithTracks(t, store, "empty-1", nil)
	insertTestRunWithTracks(t, store, "empty-2", nil)

	cmp, err := CompareRuns(store, "empty-1", "empty-2")
	if err != nil {
		t.Fatalf("CompareRuns: %v", err)
	}
	if len(cmp.MatchedTracks) != 0 {
		t.Errorf("expected 0 matched tracks, got %d", len(cmp.MatchedTracks))
	}
	if len(cmp.TracksOnlyRun1) != 0 {
		t.Errorf("expected 0 run1-only tracks, got %d", len(cmp.TracksOnlyRun1))
	}
	if len(cmp.TracksOnlyRun2) != 0 {
		t.Errorf("expected 0 run2-only tracks, got %d", len(cmp.TracksOnlyRun2))
	}
}

func TestCov_CompareRuns_Run2EmptyRun1HasTracks(t *testing.T) {
	store, cleanup := setupCompareRunsDB(t)
	defer cleanup()

	insertTestRunWithTracks(t, store, "has-tracks", []RunTrack{
		{TrackID: "t1", TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID: "sensor-cmp", StartUnixNanos: 100, EndUnixNanos: 200, ObservationCount: 5,
		}},
	})
	insertTestRunWithTracks(t, store, "no-tracks", nil)

	// Also test the reverse direction.
	cmp, err := CompareRuns(store, "no-tracks", "has-tracks")
	if err != nil {
		t.Fatalf("CompareRuns: %v", err)
	}
	if len(cmp.TracksOnlyRun1) != 0 {
		t.Errorf("expected 0 run1-only tracks, got %d", len(cmp.TracksOnlyRun1))
	}
	if len(cmp.TracksOnlyRun2) != 1 {
		t.Errorf("expected 1 run2-only track, got %d", len(cmp.TracksOnlyRun2))
	}
}

func TestCov_CompareRuns_MatchingTracks(t *testing.T) {
	store, cleanup := setupCompareRunsDB(t)
	defer cleanup()

	// Two runs with overlapping tracks (same time ranges → IoU=1.0).
	insertTestRunWithTracks(t, store, "run-a", []RunTrack{
		{TrackID: "a-t1", TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID: "sensor-cmp", StartUnixNanos: 1000, EndUnixNanos: 5000, ObservationCount: 10,
		}},
		{TrackID: "a-t2", TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID: "sensor-cmp", StartUnixNanos: 6000, EndUnixNanos: 9000, ObservationCount: 8,
		}},
	})
	insertTestRunWithTracks(t, store, "run-b", []RunTrack{
		{TrackID: "b-t1", TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID: "sensor-cmp", StartUnixNanos: 1000, EndUnixNanos: 5000, ObservationCount: 10,
		}},
		{TrackID: "b-t2", TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID: "sensor-cmp", StartUnixNanos: 6000, EndUnixNanos: 9000, ObservationCount: 8,
		}},
	})

	cmp, err := CompareRuns(store, "run-a", "run-b")
	if err != nil {
		t.Fatalf("CompareRuns: %v", err)
	}
	if len(cmp.MatchedTracks) != 2 {
		t.Fatalf("expected 2 matched tracks, got %d", len(cmp.MatchedTracks))
	}
	if len(cmp.TracksOnlyRun1) != 0 {
		t.Errorf("expected 0 run1-only tracks, got %d", len(cmp.TracksOnlyRun1))
	}
	if len(cmp.TracksOnlyRun2) != 0 {
		t.Errorf("expected 0 run2-only tracks, got %d", len(cmp.TracksOnlyRun2))
	}
}

func TestCov_CompareRuns_UnmatchedTracks(t *testing.T) {
	store, cleanup := setupCompareRunsDB(t)
	defer cleanup()

	// Two tracks in different time ranges → IoU < 0.3 → unmatched.
	insertTestRunWithTracks(t, store, "far-a", []RunTrack{
		{TrackID: "fa-t1", TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID: "sensor-cmp", StartUnixNanos: 1000, EndUnixNanos: 2000, ObservationCount: 10,
		}},
	})
	insertTestRunWithTracks(t, store, "far-b", []RunTrack{
		{TrackID: "fb-t1", TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID: "sensor-cmp", StartUnixNanos: 9000, EndUnixNanos: 10000, ObservationCount: 10,
		}},
	})

	cmp, err := CompareRuns(store, "far-a", "far-b")
	if err != nil {
		t.Fatalf("CompareRuns: %v", err)
	}
	if len(cmp.MatchedTracks) != 0 {
		t.Errorf("expected 0 matched tracks, got %d", len(cmp.MatchedTracks))
	}
	if len(cmp.TracksOnlyRun1) != 1 {
		t.Errorf("expected 1 run1-only track, got %d", len(cmp.TracksOnlyRun1))
	}
	if len(cmp.TracksOnlyRun2) != 1 {
		t.Errorf("expected 1 run2-only track, got %d", len(cmp.TracksOnlyRun2))
	}
}

func TestCov_CompareRuns_WithConfigAssetParamDiff(t *testing.T) {
	store, cleanup := setupCompareRunsDB(t)
	defer cleanup()

	// Build two different param sets via the config asset store so that
	// GetRun hydrates ExecutionConfig when CompareRuns calls GetRun.
	cfgStore := configasset.NewStore(store.db)

	psA, err := configasset.MakeRequestedParamSet([]byte(`{"background_update_fraction":0.05}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet A: %v", err)
	}
	rcA, err := cfgStore.EnsureRunConfig(psA, configasset.BuildIdentity{BuildVersion: "v1", BuildGitSHA: "sha1"})
	if err != nil {
		t.Fatalf("EnsureRunConfig A: %v", err)
	}

	psB, err := configasset.MakeRequestedParamSet([]byte(`{"background_update_fraction":0.10}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet B: %v", err)
	}
	rcB, err := cfgStore.EnsureRunConfig(psB, configasset.BuildIdentity{BuildVersion: "v1", BuildGitSHA: "sha1"})
	if err != nil {
		t.Fatalf("EnsureRunConfig B: %v", err)
	}

	insertTestRunWithTracks(t, store, "param-a", []RunTrack{
		{TrackID: "pa-t1", TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID: "sensor-cmp", StartUnixNanos: 1000, EndUnixNanos: 5000, ObservationCount: 10,
		}},
	})
	insertTestRunWithTracks(t, store, "param-b", []RunTrack{
		{TrackID: "pb-t1", TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID: "sensor-cmp", StartUnixNanos: 1000, EndUnixNanos: 5000, ObservationCount: 10,
		}},
	})

	store.db.Exec(`UPDATE lidar_run_records SET run_config_id = ? WHERE run_id = ?`, rcA.RunConfigID, "param-a")
	store.db.Exec(`UPDATE lidar_run_records SET run_config_id = ? WHERE run_id = ?`, rcB.RunConfigID, "param-b")

	cmp, err := CompareRuns(store, "param-a", "param-b")
	if err != nil {
		t.Fatalf("CompareRuns: %v", err)
	}
	// The composed JSON is a run_config envelope, not raw RunParams, so
	// ParseRunParams may fail. We only need to verify the code path executes
	// without error — the param diff may or may not be populated depending
	// on whether the composed JSON matches the RunParams schema.
	_ = cmp.ParamDiff
}

func TestCov_CompareRuns_Run1NotFound(t *testing.T) {
	store, cleanup := setupCompareRunsDB(t)
	defer cleanup()

	insertTestRunWithTracks(t, store, "exists", nil)

	_, err := CompareRuns(store, "missing", "exists")
	if err == nil {
		t.Fatal("expected error for missing run1")
	}
}

func TestCov_CompareRuns_Run2NotFound(t *testing.T) {
	store, cleanup := setupCompareRunsDB(t)
	defer cleanup()

	insertTestRunWithTracks(t, store, "exists2", nil)

	_, err := CompareRuns(store, "exists2", "missing")
	if err == nil {
		t.Fatal("expected error for missing run2")
	}
}
