package sqlite

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// GetRunTrack (0% → covers ~30 stmts)
// ---------------------------------------------------------------------------

func TestGetRunTrack_HappyPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert a run first.
	run := &AnalysisRun{
		RunID:      "run-grt-1",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		ParamsJSON: json.RawMessage(`{}`),
		Status:     "completed",
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	// Insert a track with all nullable fields populated.
	track := &RunTrack{
		RunID:               "run-grt-1",
		TrackID:             "track-1",
		SensorID:            "sensor-1",
		TrackState:          "confirmed",
		StartUnixNanos:      1000,
		EndUnixNanos:        2000,
		ObservationCount:    10,
		AvgSpeedMps:         5.5,
		PeakSpeedMps:        8.0,
		P50SpeedMps:         5.0,
		P85SpeedMps:         7.0,
		P95SpeedMps:         7.5,
		ObjectClass:         "vehicle",
		ObjectConfidence:    0.95,
		ClassificationModel: "rule_based",
		UserLabel:           "car",
		LabelConfidence:     0.9,
		LabelerID:           "user-1",
		LabeledAt:           3000,
		QualityLabel:        "good",
		LabelSource:         "human_manual",
		IsSplitCandidate:    true,
		IsMergeCandidate:    false,
		LinkedTrackIDs:      []string{"track-2", "track-3"},
	}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack: %v", err)
	}

	got, err := store.GetRunTrack("run-grt-1", "track-1")
	if err != nil {
		t.Fatalf("GetRunTrack: %v", err)
	}

	// Verify nullable fields.
	if got.EndUnixNanos != 2000 {
		t.Errorf("EndUnixNanos = %d, want 2000", got.EndUnixNanos)
	}
	if got.ObjectClass != "vehicle" {
		t.Errorf("ObjectClass = %q, want %q", got.ObjectClass, "vehicle")
	}
	if got.ObjectConfidence < 0.94 {
		t.Errorf("ObjectConfidence = %f, want ~0.95", got.ObjectConfidence)
	}
	if got.ClassificationModel != "rule_based" {
		t.Errorf("ClassificationModel = %q, want %q", got.ClassificationModel, "rule_based")
	}
	if got.UserLabel != "car" {
		t.Errorf("UserLabel = %q, want %q", got.UserLabel, "car")
	}
	if got.LabelConfidence < 0.89 {
		t.Errorf("LabelConfidence = %f, want ~0.9", got.LabelConfidence)
	}
	if got.LabelerID != "user-1" {
		t.Errorf("LabelerID = %q, want %q", got.LabelerID, "user-1")
	}
	if got.LabeledAt != 3000 {
		t.Errorf("LabeledAt = %d, want 3000", got.LabeledAt)
	}
	if got.QualityLabel != "good" {
		t.Errorf("QualityLabel = %q, want %q", got.QualityLabel, "good")
	}
	if got.LabelSource != "human_manual" {
		t.Errorf("LabelSource = %q, want %q", got.LabelSource, "human_manual")
	}
	if len(got.LinkedTrackIDs) != 2 {
		t.Errorf("LinkedTrackIDs len = %d, want 2", len(got.LinkedTrackIDs))
	}
}

func TestGetRunTrack_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)
	_, err := store.GetRunTrack("no-run", "no-track")
	if err == nil {
		t.Error("expected error for non-existent track")
	}
}

// ---------------------------------------------------------------------------
// PruneDeletedTracks (0% → covers ~18 stmts)
// ---------------------------------------------------------------------------

func TestPruneDeletedTracks_HappyPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	oldNanos := time.Now().Add(-2 * time.Hour).UnixNano()
	recentNanos := time.Now().UnixNano()

	// Insert a "deleted" track with old timestamp — should be pruned.
	_, err := db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, track_state,
		start_unix_nanos, end_unix_nanos, observation_count,
		avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg)
		VALUES (?, ?, 'world', 'deleted', ?, ?, 5, 1.0, 2.0, 1.0, 1.5, 1.8, 0.1, 0.1, 0.1, 0.1, 0.1)`,
		"prune-old", "sensor-prune", oldNanos, oldNanos)
	if err != nil {
		t.Fatalf("insert old track: %v", err)
	}

	// Insert a "deleted" track with recent timestamp — should NOT be pruned.
	_, err = db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, track_state,
		start_unix_nanos, end_unix_nanos, observation_count,
		avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg)
		VALUES (?, ?, 'world', 'deleted', ?, ?, 3, 1.0, 2.0, 1.0, 1.5, 1.8, 0.1, 0.1, 0.1, 0.1, 0.1)`,
		"prune-recent", "sensor-prune", recentNanos, recentNanos)
	if err != nil {
		t.Fatalf("insert recent track: %v", err)
	}

	// Insert a "confirmed" track with old timestamp — should NOT be pruned.
	_, err = db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, track_state,
		start_unix_nanos, end_unix_nanos, observation_count,
		avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg)
		VALUES (?, ?, 'world', 'confirmed', ?, ?, 8, 1.0, 2.0, 1.0, 1.5, 1.8, 0.1, 0.1, 0.1, 0.1, 0.1)`,
		"prune-confirmed", "sensor-prune", oldNanos, oldNanos)
	if err != nil {
		t.Fatalf("insert confirmed track: %v", err)
	}

	pruned, err := PruneDeletedTracks(db, "sensor-prune", 1*time.Hour)
	if err != nil {
		t.Fatalf("PruneDeletedTracks: %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want 1", pruned)
	}

	// Verify the old deleted track is gone but others remain.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM lidar_tracks WHERE sensor_id = 'sensor-prune'`).Scan(&count)
	if count != 2 {
		t.Errorf("remaining tracks = %d, want 2", count)
	}
}

func TestPruneDeletedTracks_EmptySensorID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := PruneDeletedTracks(db, "", 1*time.Hour)
	if err == nil {
		t.Error("expected error for empty sensorID")
	}
}

// ---------------------------------------------------------------------------
// SaveSweepCheckpoint + LoadSweepCheckpoint round-trip (~16 stmts)
// Uses setupTestDB (full schema with checkpoint columns) not setupTestSweepDB
// ---------------------------------------------------------------------------

func TestSweepCheckpoint_RoundTrip(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewSweepStore(db)

	// Insert a sweep via SaveSweepStart.
	startedAt := time.Now().UTC().Truncate(time.Second)
	req := json.RawMessage(`{"param":"noise","start":0.01,"end":0.2}`)
	if err := store.SaveSweepStart("sweep-ckpt-1", "sensor-1", "auto-tune", req, startedAt, "f1", "v1"); err != nil {
		t.Fatalf("SaveSweepStart: %v", err)
	}

	// Save a checkpoint.
	bounds := json.RawMessage(`{"lo":0.01,"hi":0.2}`)
	results := json.RawMessage(`[{"round":1,"score":0.85}]`)
	cpReq := json.RawMessage(`{"param":"noise"}`)
	if err := store.SaveSweepCheckpoint("sweep-ckpt-1", 3, bounds, results, cpReq); err != nil {
		t.Fatalf("SaveSweepCheckpoint: %v", err)
	}

	// Load the checkpoint.
	round, gotBounds, gotResults, gotReq, err := store.LoadSweepCheckpoint("sweep-ckpt-1")
	if err != nil {
		t.Fatalf("LoadSweepCheckpoint: %v", err)
	}
	if round != 3 {
		t.Errorf("round = %d, want 3", round)
	}
	if string(gotBounds) != string(bounds) {
		t.Errorf("bounds = %s, want %s", gotBounds, bounds)
	}
	if string(gotResults) != string(results) {
		t.Errorf("results = %s, want %s", gotResults, results)
	}
	if string(gotReq) != string(cpReq) {
		t.Errorf("request = %s, want %s", gotReq, cpReq)
	}
}

func TestLoadSweepCheckpoint_NotSuspended(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewSweepStore(db)

	// Insert a sweep that is still running (no checkpoint).
	startedAt := time.Now().UTC().Truncate(time.Second)
	req := json.RawMessage(`{"param":"test"}`)
	if err := store.SaveSweepStart("sweep-running", "sensor-1", "auto-tune", req, startedAt, "f1", "v1"); err != nil {
		t.Fatalf("SaveSweepStart: %v", err)
	}

	// Load checkpoint for a non-suspended sweep — should error.
	_, _, _, _, err := store.LoadSweepCheckpoint("sweep-running")
	if err == nil {
		t.Error("expected error loading checkpoint from running sweep")
	}
}

// ---------------------------------------------------------------------------
// GetSuspendedSweep + GetSuspendedSweepInfo (~21 stmts)
// ---------------------------------------------------------------------------

func TestGetSuspendedSweep_None(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewSweepStore(db)

	sweepID, round, err := store.GetSuspendedSweep()
	if err != nil {
		t.Fatalf("GetSuspendedSweep: %v", err)
	}
	if sweepID != "" || round != 0 {
		t.Errorf("expected empty sweep, got %q round %d", sweepID, round)
	}
}

func TestGetSuspendedSweep_WithSuspended(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewSweepStore(db)

	// Insert and suspend a sweep.
	startedAt := time.Now().UTC().Truncate(time.Second)
	req := json.RawMessage(`{"param":"test"}`)
	if err := store.SaveSweepStart("sweep-susp-1", "sensor-1", "auto-tune", req, startedAt, "f1", "v1"); err != nil {
		t.Fatalf("SaveSweepStart: %v", err)
	}
	bounds := json.RawMessage(`{"lo":0.01}`)
	if err := store.SaveSweepCheckpoint("sweep-susp-1", 5, bounds, nil, nil); err != nil {
		t.Fatalf("SaveSweepCheckpoint: %v", err)
	}

	sweepID, round, err := store.GetSuspendedSweep()
	if err != nil {
		t.Fatalf("GetSuspendedSweep: %v", err)
	}
	if sweepID != "sweep-susp-1" {
		t.Errorf("sweepID = %q, want %q", sweepID, "sweep-susp-1")
	}
	if round != 5 {
		t.Errorf("round = %d, want 5", round)
	}
}

func TestGetSuspendedSweepInfo_WithSuspended(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewSweepStore(db)

	startedAt := time.Now().UTC().Truncate(time.Second)
	req := json.RawMessage(`{"param":"test"}`)
	if err := store.SaveSweepStart("sweep-info-1", "sensor-1", "auto-tune", req, startedAt, "f1", "v1"); err != nil {
		t.Fatalf("SaveSweepStart: %v", err)
	}
	if err := store.SaveSweepCheckpoint("sweep-info-1", 7, nil, nil, nil); err != nil {
		t.Fatalf("SaveSweepCheckpoint: %v", err)
	}

	info, err := store.GetSuspendedSweepInfo()
	if err != nil {
		t.Fatalf("GetSuspendedSweepInfo: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.SweepID != "sweep-info-1" {
		t.Errorf("SweepID = %q, want %q", info.SweepID, "sweep-info-1")
	}
	if info.SensorID != "sensor-1" {
		t.Errorf("SensorID = %q, want %q", info.SensorID, "sensor-1")
	}
	if info.CheckpointRound != 7 {
		t.Errorf("CheckpointRound = %d, want 7", info.CheckpointRound)
	}
}

func TestGetSuspendedSweepInfo_None(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewSweepStore(db)

	info, err := store.GetSuspendedSweepInfo()
	if err != nil {
		t.Fatalf("GetSuspendedSweepInfo: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil info, got %+v", info)
	}
}

// ---------------------------------------------------------------------------
// GetUnlabeledTracks with populated optional fields (~10 stmts)
// ---------------------------------------------------------------------------

func TestGetUnlabeledTracks_WithOptionalFields(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert a run.
	run := &AnalysisRun{
		RunID:      "run-unlabel",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		ParamsJSON: json.RawMessage(`{}`),
		Status:     "completed",
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	// Insert unlabeled track with all optional fields populated.
	track := &RunTrack{
		RunID:               "run-unlabel",
		TrackID:             "track-opt",
		SensorID:            "sensor-1",
		TrackState:          "confirmed",
		StartUnixNanos:      1000,
		EndUnixNanos:        2000,
		ObservationCount:    15,
		AvgSpeedMps:         6.0,
		PeakSpeedMps:        9.0,
		ObjectClass:         "pedestrian",
		ObjectConfidence:    0.88,
		ClassificationModel: "cnn_v2",
		// No UserLabel — makes it "unlabeled"
		IsSplitCandidate: true,
		LinkedTrackIDs:   []string{"link-1"},
	}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack: %v", err)
	}

	// Insert labelled track — should NOT be returned.
	labelled := &RunTrack{
		RunID:            "run-unlabel",
		TrackID:          "track-labelled",
		SensorID:         "sensor-1",
		TrackState:       "confirmed",
		StartUnixNanos:   1000,
		ObservationCount: 3,
		UserLabel:        "car",
	}
	if err := store.InsertRunTrack(labelled); err != nil {
		t.Fatalf("InsertRunTrack (labelled): %v", err)
	}

	tracks, err := store.GetUnlabeledTracks("run-unlabel", 10)
	if err != nil {
		t.Fatalf("GetUnlabeledTracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(tracks))
	}

	got := tracks[0]
	if got.ObjectClass != "pedestrian" {
		t.Errorf("ObjectClass = %q, want %q", got.ObjectClass, "pedestrian")
	}
	if got.ClassificationModel != "cnn_v2" {
		t.Errorf("ClassificationModel = %q, want %q", got.ClassificationModel, "cnn_v2")
	}
	if got.EndUnixNanos != 2000 {
		t.Errorf("EndUnixNanos = %d, want 2000", got.EndUnixNanos)
	}
	if len(got.LinkedTrackIDs) != 1 {
		t.Errorf("LinkedTrackIDs len = %d, want 1", len(got.LinkedTrackIDs))
	}
}

// ---------------------------------------------------------------------------
// CompareRuns: empty-run + param-diff branches (~8 stmts)
// ---------------------------------------------------------------------------

func TestCompareRuns_OneEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	params := json.RawMessage(`{"version":"1.0","background":{"noise_relative_fraction":0.05},"clustering":{"eps":0.8},"tracking":{"max_tracks":200}}`)

	// Run 1 with one track.
	run1 := &AnalysisRun{RunID: "cmp-r1", CreatedAt: time.Now(), SourceType: "pcap", SensorID: "s1", ParamsJSON: params, Status: "completed"}
	if err := store.InsertRun(run1); err != nil {
		t.Fatalf("InsertRun run1: %v", err)
	}
	track := &RunTrack{RunID: "cmp-r1", TrackID: "t1", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 1000, ObservationCount: 5}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack: %v", err)
	}

	// Run 2 with no tracks.
	run2 := &AnalysisRun{RunID: "cmp-r2", CreatedAt: time.Now(), SourceType: "pcap", SensorID: "s1", ParamsJSON: params, Status: "completed"}
	if err := store.InsertRun(run2); err != nil {
		t.Fatalf("InsertRun run2: %v", err)
	}

	cmp, err := CompareRuns(store, "cmp-r1", "cmp-r2")
	if err != nil {
		t.Fatalf("CompareRuns: %v", err)
	}

	if len(cmp.TracksOnlyRun1) != 1 {
		t.Errorf("TracksOnlyRun1 len = %d, want 1", len(cmp.TracksOnlyRun1))
	}
	if len(cmp.TracksOnlyRun2) != 0 {
		t.Errorf("TracksOnlyRun2 len = %d, want 0", len(cmp.TracksOnlyRun2))
	}
}

func TestCompareRuns_WithParamDiff(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	params1 := json.RawMessage(`{"version":"1.0","timestamp":"2025-01-01T00:00:00Z","background":{"noise_relative_fraction":0.05},"clustering":{"eps":0.8,"min_pts":5},"tracking":{"max_tracks":200,"max_misses":10,"hits_to_confirm":3}}`)
	params2 := json.RawMessage(`{"version":"1.0","timestamp":"2025-01-01T00:00:00Z","background":{"noise_relative_fraction":0.10},"clustering":{"eps":0.8,"min_pts":5},"tracking":{"max_tracks":200,"max_misses":10,"hits_to_confirm":3}}`)

	// Run 1 with a track.
	run1 := &AnalysisRun{RunID: "cmp-pd-r1", CreatedAt: time.Now(), SourceType: "pcap", SensorID: "s1", ParamsJSON: params1, Status: "completed"}
	if err := store.InsertRun(run1); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	if err := store.InsertRunTrack(&RunTrack{RunID: "cmp-pd-r1", TrackID: "t1", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 1000, EndUnixNanos: 5000, ObservationCount: 5}); err != nil {
		t.Fatalf("InsertRunTrack r1: %v", err)
	}

	// Run 2 with matching track (same time range for IoU match).
	run2 := &AnalysisRun{RunID: "cmp-pd-r2", CreatedAt: time.Now(), SourceType: "pcap", SensorID: "s1", ParamsJSON: params2, Status: "completed"}
	if err := store.InsertRun(run2); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	if err := store.InsertRunTrack(&RunTrack{RunID: "cmp-pd-r2", TrackID: "t2", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 1000, EndUnixNanos: 5000, ObservationCount: 7}); err != nil {
		t.Fatalf("InsertRunTrack r2: %v", err)
	}

	cmp, err := CompareRuns(store, "cmp-pd-r1", "cmp-pd-r2")
	if err != nil {
		t.Fatalf("CompareRuns: %v", err)
	}

	if len(cmp.MatchedTracks) != 1 {
		t.Errorf("MatchedTracks len = %d, want 1", len(cmp.MatchedTracks))
	}
	if len(cmp.ParamDiff) == 0 {
		t.Error("expected non-empty ParamDiff")
	}
}

// ---------------------------------------------------------------------------
// ClearTracks / ClearRuns / DeleteRun — happy-path coverage
// ---------------------------------------------------------------------------

func TestClearTracks_HappyPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert a track and observation.
	_, err := db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, track_state,
		start_unix_nanos, end_unix_nanos, observation_count,
		avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg)
		VALUES ('ct-t1', 'ct-sensor', 'world', 'confirmed', 1000, 2000, 1, 1.0, 2.0, 1.0, 1.5, 1.8, 0.1, 0.1, 0.1, 0.1, 0.1)`)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}

	if err := ClearTracks(db, "ct-sensor"); err != nil {
		t.Fatalf("ClearTracks: %v", err)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM lidar_tracks WHERE sensor_id = 'ct-sensor'`).Scan(&count)
	if count != 0 {
		t.Errorf("remaining tracks = %d, want 0", count)
	}
}

func TestClearRuns_HappyPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)
	run := &AnalysisRun{RunID: "cr-run-1", CreatedAt: time.Now(), SourceType: "pcap", SensorID: "cr-sensor", ParamsJSON: json.RawMessage(`{}`), Status: "completed"}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	if err := ClearRuns(db, "cr-sensor"); err != nil {
		t.Fatalf("ClearRuns: %v", err)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM lidar_analysis_runs WHERE sensor_id = 'cr-sensor'`).Scan(&count)
	if count != 0 {
		t.Errorf("remaining runs = %d, want 0", count)
	}
}

func TestDeleteRun_HappyPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)
	run := &AnalysisRun{RunID: "dr-run-1", CreatedAt: time.Now(), SourceType: "pcap", SensorID: "dr-sensor", ParamsJSON: json.RawMessage(`{}`), Status: "completed"}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	if err := DeleteRun(db, "dr-run-1"); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}
}

func TestDeleteRun_EmptyID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := DeleteRun(db, "")
	if err == nil {
		t.Error("expected error for empty runID")
	}
}

// ---------------------------------------------------------------------------
// GetTrackObservations — happy path (~4 stmts)
// ---------------------------------------------------------------------------

func TestGetTrackObservations_HappyPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert a track.
	_, err := db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, track_state,
		start_unix_nanos, end_unix_nanos, observation_count,
		avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg)
		VALUES ('obs-t1', 'obs-sensor', 'world', 'confirmed', 1000, 2000, 2, 1.0, 2.0, 1.0, 1.5, 1.8, 0.1, 0.1, 0.1, 0.1, 0.1)`)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}

	// Insert observations.
	_, err = db.Exec(`INSERT INTO lidar_track_obs (track_id, ts_unix_nanos, world_frame,
		x, y, z, velocity_x, velocity_y, speed_mps, heading_rad,
		bounding_box_length, bounding_box_width, bounding_box_height,
		height_p95, intensity_mean)
		VALUES ('obs-t1', 1000, 'world', 1.0, 2.0, 0.5, 0.1, 0.2, 1.5, 0.3, 0.4, 0.3, 0.2, 0.1, 50.0)`)
	if err != nil {
		t.Fatalf("insert observation: %v", err)
	}

	obs, err := GetTrackObservations(db, "obs-t1", 10)
	if err != nil {
		t.Fatalf("GetTrackObservations: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("got %d observations, want 1", len(obs))
	}
	if obs[0].SpeedMps != 1.5 {
		t.Errorf("SpeedMps = %f, want 1.5", obs[0].SpeedMps)
	}
}

// ---------------------------------------------------------------------------
// GetRecentClusters — happy path (~4 stmts)
// ---------------------------------------------------------------------------

func TestGetRecentClusters_HappyPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert a cluster (lidar_cluster_id is INTEGER PRIMARY KEY, auto-assigned).
	_, err := db.Exec(`INSERT INTO lidar_clusters (sensor_id, world_frame, ts_unix_nanos,
		centroid_x, centroid_y, centroid_z,
		bounding_box_length, bounding_box_width, bounding_box_height,
		points_count, height_p95, intensity_mean)
		VALUES ('cl-sensor', 'world', 5000, 1.0, 2.0, 0.5, 0.4, 0.3, 0.2, 10, 0.1, 50.0)`)
	if err != nil {
		t.Fatalf("insert cluster: %v", err)
	}

	clusters, err := GetRecentClusters(db, "cl-sensor", 1000, 10000, 10)
	if err != nil {
		t.Fatalf("GetRecentClusters: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(clusters))
	}
	if clusters[0].PointsCount != 10 {
		t.Errorf("PointsCount = %d, want 10", clusters[0].PointsCount)
	}
}

// ---------------------------------------------------------------------------
// GetTracksInRange with state filter and optional fields (~5 stmts)
// ---------------------------------------------------------------------------

func TestGetTracksInRange_WithState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	nowNanos := time.Now().UnixNano()

	// Insert a confirmed track with optional fields.
	_, err := db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, track_state,
		start_unix_nanos, end_unix_nanos, observation_count,
		avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg, object_class, object_confidence, classification_model)
		VALUES ('range-t1', 'range-sensor', 'world', 'confirmed', ?, ?, 5, 3.0, 5.0, 3.0, 4.0, 4.5, 0.2, 0.15, 0.3, 0.25, 60.0, 'vehicle', 0.92, 'rule_based')`,
		nowNanos-1e9, nowNanos)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}

	tracks, err := GetTracksInRange(db, "range-sensor", "confirmed", nowNanos-2e9, nowNanos+1e9, 10)
	if err != nil {
		t.Fatalf("GetTracksInRange: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(tracks))
	}
	if tracks[0].ObjectClass != "vehicle" {
		t.Errorf("ObjectClass = %q, want %q", tracks[0].ObjectClass, "vehicle")
	}
	if tracks[0].ClassificationModel != "rule_based" {
		t.Errorf("ClassificationModel = %q, want %q", tracks[0].ClassificationModel, "rule_based")
	}
}

func TestGetTracksInRange_NoState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	nowNanos := time.Now().UnixNano()

	// Insert a confirmed track.
	_, err := db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, track_state,
		start_unix_nanos, end_unix_nanos, observation_count,
		avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg)
		VALUES ('range-t2', 'range-sensor2', 'world', 'confirmed', ?, ?, 5, 3.0, 5.0, 3.0, 4.0, 4.5, 0.2, 0.15, 0.3, 0.25, 60.0)`,
		nowNanos-1e9, nowNanos)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}

	// Insert a deleted track — should be excluded when state is "".
	_, err = db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, track_state,
		start_unix_nanos, end_unix_nanos, observation_count,
		avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg)
		VALUES ('range-t3', 'range-sensor2', 'world', 'deleted', ?, ?, 5, 3.0, 5.0, 3.0, 4.0, 4.5, 0.2, 0.15, 0.3, 0.25, 60.0)`,
		nowNanos-1e9, nowNanos)
	if err != nil {
		t.Fatalf("insert deleted track: %v", err)
	}

	tracks, err := GetTracksInRange(db, "range-sensor2", "", nowNanos-2e9, nowNanos+1e9, 10)
	if err != nil {
		t.Fatalf("GetTracksInRange: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1 (deleted should be excluded)", len(tracks))
	}
}

// ---------------------------------------------------------------------------
// ListSweeps with completedAt — cover the completedAt parsing branch
// ---------------------------------------------------------------------------

func TestListSweeps_WithCompletedAt(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewSweepStore(db)

	startedAt := time.Now().UTC().Truncate(time.Second)
	req := json.RawMessage(`{"param":"test"}`)
	if err := store.SaveSweepStart("sweep-ls-1", "sensor-ls", "manual", req, startedAt, "obj", "v1"); err != nil {
		t.Fatalf("SaveSweepStart: %v", err)
	}

	completedAt := startedAt.Add(10 * time.Minute)
	if err := store.SaveSweepComplete("sweep-ls-1", "completed",
		json.RawMessage(`{}`), nil, nil, completedAt, "",
		nil, nil, nil, "", ""); err != nil {
		t.Fatalf("SaveSweepComplete: %v", err)
	}

	sweeps, err := store.ListSweeps("sensor-ls", 10)
	if err != nil {
		t.Fatalf("ListSweeps: %v", err)
	}
	if len(sweeps) != 1 {
		t.Fatalf("got %d sweeps, want 1", len(sweeps))
	}
	if sweeps[0].CompletedAt == nil {
		t.Error("expected non-nil CompletedAt")
	}
}

// ---------------------------------------------------------------------------
// GetLabelingProgress — with labelled tracks (covers byClass loop)
// ---------------------------------------------------------------------------

func TestGetLabelingProgress_WithLabels(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	run := &AnalysisRun{RunID: "lp-run", CreatedAt: time.Now(), SourceType: "pcap", SensorID: "s1", ParamsJSON: json.RawMessage(`{}`), Status: "completed"}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	// Insert labelled tracks.
	for _, tc := range []struct {
		trackID string
		label   string
	}{
		{"lp-t1", "car"},
		{"lp-t2", "car"},
		{"lp-t3", "pedestrian"},
		{"lp-t4", ""}, // unlabelled
	} {
		tr := &RunTrack{
			RunID:            "lp-run",
			TrackID:          tc.trackID,
			SensorID:         "s1",
			TrackState:       "confirmed",
			StartUnixNanos:   1000,
			ObservationCount: 5,
			UserLabel:        tc.label,
		}
		if err := store.InsertRunTrack(tr); err != nil {
			t.Fatalf("InsertRunTrack %s: %v", tc.trackID, err)
		}
	}

	total, labeled, byClass, err := store.GetLabelingProgress("lp-run")
	if err != nil {
		t.Fatalf("GetLabelingProgress: %v", err)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	if labeled != 3 {
		t.Errorf("labeled = %d, want 3", labeled)
	}
	if byClass["car"] != 2 {
		t.Errorf("byClass[car] = %d, want 2", byClass["car"])
	}
	if byClass["pedestrian"] != 1 {
		t.Errorf("byClass[pedestrian] = %d, want 1", byClass["pedestrian"])
	}
}

// ---------------------------------------------------------------------------
// UpdateTrackLabel — cover the happy path (label + quality)
// ---------------------------------------------------------------------------

func TestUpdateTrackLabel_HappyPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	run := &AnalysisRun{RunID: "utl-run", CreatedAt: time.Now(), SourceType: "pcap", SensorID: "s1", ParamsJSON: json.RawMessage(`{}`), Status: "completed"}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	track := &RunTrack{RunID: "utl-run", TrackID: "utl-t1", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 1000, ObservationCount: 5}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack: %v", err)
	}

	// Apply a label.
	if err := store.UpdateTrackLabel("utl-run", "utl-t1", "car", "good", 0.95, "user-1", "human_manual"); err != nil {
		t.Fatalf("UpdateTrackLabel: %v", err)
	}

	// Verify via GetRunTrack.
	got, err := store.GetRunTrack("utl-run", "utl-t1")
	if err != nil {
		t.Fatalf("GetRunTrack: %v", err)
	}
	if got.UserLabel != "car" {
		t.Errorf("UserLabel = %q, want %q", got.UserLabel, "car")
	}
	if got.QualityLabel != "good" {
		t.Errorf("QualityLabel = %q, want %q", got.QualityLabel, "good")
	}
}

// ---------------------------------------------------------------------------
// ListByScene with evaluations — covers scanEvaluation paramsJSON.Valid branch
// ---------------------------------------------------------------------------

func TestListByScene_WithParams(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	evalStore := NewEvaluationStore(db)
	runStore := NewAnalysisRunStore(db)

	// Insert prerequisite runs.
	for _, rid := range []string{"ref-run", "cand-run"} {
		if err := runStore.InsertRun(&AnalysisRun{RunID: rid, CreatedAt: time.Now(), SourceType: "pcap", SensorID: "s1", ParamsJSON: json.RawMessage(`{}`), Status: "completed"}); err != nil {
			t.Fatalf("InsertRun %s: %v", rid, err)
		}
	}
	// Insert prerequisite scene.
	if _, err := db.Exec(`INSERT INTO lidar_scenes (scene_id, sensor_id, pcap_file, created_at_ns) VALUES ('scene-1', 's1', '/test.pcap', ?)`, time.Now().UnixNano()); err != nil {
		t.Fatalf("Insert scene: %v", err)
	}

	// Insert an evaluation with paramsJSON.
	eval := &Evaluation{
		EvaluationID:   "eval-1",
		SceneID:        "scene-1",
		ReferenceRunID: "ref-run",
		CandidateRunID: "cand-run",
		DetectionRate:  0.95,
		CompositeScore: 0.88,
		MatchedCount:   10,
		ReferenceCount: 12,
		CandidateCount: 11,
		ParamsJSON:     json.RawMessage(`{"version":"1.0"}`),
	}
	if err := evalStore.Insert(eval); err != nil {
		t.Fatalf("Insert evaluation: %v", err)
	}

	evals, err := evalStore.ListByScene("scene-1")
	if err != nil {
		t.Fatalf("ListByScene: %v", err)
	}
	if len(evals) != 1 {
		t.Fatalf("got %d evaluations, want 1", len(evals))
	}
	if string(evals[0].ParamsJSON) != `{"version":"1.0"}` {
		t.Errorf("ParamsJSON = %s, want %s", evals[0].ParamsJSON, `{"version":"1.0"}`)
	}
}

// ---------------------------------------------------------------------------
// GetTracksInRange with observations — covers history-building loop
// ---------------------------------------------------------------------------

func TestGetTracksInRange_WithObservations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	nowNanos := time.Now().UnixNano()

	// Insert a track.
	_, err := db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, track_state,
		start_unix_nanos, end_unix_nanos, observation_count,
		avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg)
		VALUES ('obs-range-t1', 'obs-range-s', 'world', 'confirmed', ?, ?, 2, 3.0, 5.0, 3.0, 4.0, 4.5, 0.2, 0.15, 0.3, 0.25, 60.0)`,
		nowNanos-1e9, nowNanos)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}

	// Insert observations within the time range.
	for _, tsOff := range []int64{-500000000, -200000000} {
		_, err = db.Exec(`INSERT INTO lidar_track_obs (track_id, ts_unix_nanos, world_frame,
			x, y, z, velocity_x, velocity_y, speed_mps, heading_rad,
			bounding_box_length, bounding_box_width, bounding_box_height,
			height_p95, intensity_mean)
			VALUES ('obs-range-t1', ?, 'world', 1.0, 2.0, 0.5, 0.1, 0.2, 3.5, 0.3, 0.4, 0.3, 0.2, 0.1, 50.0)`,
			nowNanos+tsOff)
		if err != nil {
			t.Fatalf("insert observation: %v", err)
		}
	}

	tracks, err := GetTracksInRange(db, "obs-range-s", "", nowNanos-2e9, nowNanos+1e9, 10)
	if err != nil {
		t.Fatalf("GetTracksInRange: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(tracks))
	}
	if len(tracks[0].History) != 2 {
		t.Errorf("History len = %d, want 2", len(tracks[0].History))
	}
}

// ---------------------------------------------------------------------------
// Evaluation Delete — cover happy path
// ---------------------------------------------------------------------------

func TestEvaluationDelete_HappyPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	evalStore := NewEvaluationStore(db)
	runStore := NewAnalysisRunStore(db)

	// Insert prerequisite runs.
	for _, rid := range []string{"ref", "cand"} {
		if err := runStore.InsertRun(&AnalysisRun{RunID: rid, CreatedAt: time.Now(), SourceType: "pcap", SensorID: "s1", ParamsJSON: json.RawMessage(`{}`), Status: "completed"}); err != nil {
			t.Fatalf("InsertRun %s: %v", rid, err)
		}
	}
	// Insert prerequisite scene.
	if _, err := db.Exec(`INSERT INTO lidar_scenes (scene_id, sensor_id, pcap_file, created_at_ns) VALUES ('scene-d', 's1', '/test.pcap', ?)`, time.Now().UnixNano()); err != nil {
		t.Fatalf("Insert scene: %v", err)
	}

	eval := &Evaluation{
		EvaluationID:   "eval-del-1",
		SceneID:        "scene-d",
		ReferenceRunID: "ref",
		CandidateRunID: "cand",
		CompositeScore: 0.5,
	}
	if err := evalStore.Insert(eval); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := evalStore.Delete("eval-del-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify it's gone.
	_, err := evalStore.Get("eval-del-1")
	if err == nil {
		t.Error("expected error after deletion")
	}
}

// ---------------------------------------------------------------------------
// compareParams with SafetyMargin, NeighbourCount, GatingDistance differences
// ---------------------------------------------------------------------------

func TestCompareParams_AdditionalBranches(t *testing.T) {
	p1 := &RunParams{
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction:       0.05,
			ClosenessSensitivityMultiplier: 1.5,
			SafetyMarginMeters:             0.3,
			NeighborConfirmationCount:      2,
			NoiseRelativeFraction:          0.1,
		},
		Tracking: TrackingParamsExport{
			MaxTracks:             100,
			GatingDistanceSquared: 4.0,
		},
	}
	p2 := &RunParams{
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction:       0.05,
			ClosenessSensitivityMultiplier: 1.5,
			SafetyMarginMeters:             0.5, // different
			NeighborConfirmationCount:      3,   // different
			NoiseRelativeFraction:          0.1,
		},
		Tracking: TrackingParamsExport{
			MaxTracks:             100,
			GatingDistanceSquared: 9.0, // different
		},
	}

	diff := compareParams(p1, p2)

	bgDiff, ok := diff["background"].(map[string]any)
	if !ok {
		t.Fatal("expected 'background' key in diff")
	}
	if _, ok := bgDiff["safety_margin_meters"]; !ok {
		t.Error("expected safety_margin_meters in diff")
	}
	if _, ok := bgDiff["neighbor_confirmation_count"]; !ok {
		t.Error("expected neighbor_confirmation_count in diff")
	}

	trDiff, ok := diff["tracking"].(map[string]any)
	if !ok {
		t.Fatal("expected 'tracking' key in diff")
	}
	if _, ok := trDiff["gating_distance_squared"]; !ok {
		t.Error("expected gating_distance_squared in diff")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: nullable field branches in GetRunTracks (plural)
// ---------------------------------------------------------------------------

func TestGetRunTracks_WithAllNullableFields(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert a parent run.
	run := &AnalysisRun{
		RunID:      "run-nullable-1",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		ParamsJSON: json.RawMessage(`{}`),
		Status:     "completed",
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	// Insert a track with ALL nullable fields populated —
	// covers classModel.Valid, qualityLabel.Valid, labelSource.Valid etc. in GetRunTracks.
	track := &RunTrack{
		RunID:               "run-nullable-1",
		TrackID:             "track-n1",
		SensorID:            "sensor-1",
		TrackState:          "confirmed",
		StartUnixNanos:      1000,
		EndUnixNanos:        2000,
		ObservationCount:    10,
		AvgSpeedMps:         5.5,
		PeakSpeedMps:        8.0,
		ObjectClass:         "vehicle",
		ObjectConfidence:    0.95,
		ClassificationModel: "rule_based_v2",
		UserLabel:           "car",
		LabelConfidence:     0.9,
		LabelerID:           "user-42",
		LabeledAt:           3000,
		QualityLabel:        "excellent",
		LabelSource:         "human_review",
		IsSplitCandidate:    true,
		IsMergeCandidate:    false,
		LinkedTrackIDs:      []string{"track-n2"},
	}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack: %v", err)
	}

	tracks, err := store.GetRunTracks("run-nullable-1")
	if err != nil {
		t.Fatalf("GetRunTracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	got := tracks[0]
	if got.ClassificationModel != "rule_based_v2" {
		t.Errorf("ClassificationModel = %q, want %q", got.ClassificationModel, "rule_based_v2")
	}
	if got.QualityLabel != "excellent" {
		t.Errorf("QualityLabel = %q, want %q", got.QualityLabel, "excellent")
	}
	if got.LabelSource != "human_review" {
		t.Errorf("LabelSource = %q, want %q", got.LabelSource, "human_review")
	}
	if got.LabelerID != "user-42" {
		t.Errorf("LabelerID = %q, want %q", got.LabelerID, "user-42")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: nullable field branches in ListRuns
// ---------------------------------------------------------------------------

func TestListRuns_WithNullableFields(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	run := &AnalysisRun{
		RunID:        "run-listopt-1",
		CreatedAt:    time.Now(),
		SourceType:   "pcap",
		SourcePath:   "/data/test.pcap",
		SensorID:     "sensor-1",
		ParamsJSON:   json.RawMessage(`{}`),
		Status:       "failed",
		ErrorMessage: "disk full",
		ParentRunID:  "run-parent-1",
		Notes:        "test notes",
		VRLogPath:    "/var/lib/test.vrlog",
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	runs, err := store.ListRuns(10)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	got := runs[0]
	if got.SourcePath != "/data/test.pcap" {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, "/data/test.pcap")
	}
	if got.ErrorMessage != "disk full" {
		t.Errorf("ErrorMessage = %q, want %q", got.ErrorMessage, "disk full")
	}
	if got.ParentRunID != "run-parent-1" {
		t.Errorf("ParentRunID = %q, want %q", got.ParentRunID, "run-parent-1")
	}
	if got.Notes != "test notes" {
		t.Errorf("Notes = %q, want %q", got.Notes, "test notes")
	}
	if got.VRLogPath != "/var/lib/test.vrlog" {
		t.Errorf("VRLogPath = %q, want %q", got.VRLogPath, "/var/lib/test.vrlog")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: GetSweep with transform pipeline fields
// ---------------------------------------------------------------------------

func TestGetSweep_WithTransformPipeline(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewSweepStore(db)

	rec := SweepRecord{
		SweepID:  "sweep-tp-1",
		SensorID: "sensor-1",
		Mode:     "auto",
		Status:   "running",
		Request:  json.RawMessage(`{"key":"value"}`),
	}
	if err := store.InsertSweep(rec); err != nil {
		t.Fatalf("InsertSweep: %v", err)
	}

	// Update with transform pipeline fields.
	now := time.Now().UTC()
	err := store.UpdateSweepResults(
		"sweep-tp-1",
		"completed",
		json.RawMessage(`{"results":true}`),
		json.RawMessage(`{"rec":true}`),
		json.RawMessage(`{"round":1}`),
		&now,
		"",
		json.RawMessage(`{"score":0.95}`),
		json.RawMessage(`{"explain":"good"}`),
		json.RawMessage(`{"prov":"labels"}`),
		"pipeline-v3",
		"1.2.0",
	)
	if err != nil {
		t.Fatalf("UpdateSweepResults: %v", err)
	}

	got, err := store.GetSweep("sweep-tp-1")
	if err != nil {
		t.Fatalf("GetSweep: %v", err)
	}
	if got == nil {
		t.Fatal("GetSweep returned nil")
	}
	if got.TransformPipelineName != "pipeline-v3" {
		t.Errorf("TransformPipelineName = %q, want %q", got.TransformPipelineName, "pipeline-v3")
	}
	if got.TransformPipelineVersion != "1.2.0" {
		t.Errorf("TransformPipelineVersion = %q, want %q", got.TransformPipelineVersion, "1.2.0")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: ListSweeps with error message
// ---------------------------------------------------------------------------

func TestListSweeps_WithErrorMsg(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewSweepStore(db)

	rec := SweepRecord{
		SweepID:  "sweep-err-1",
		SensorID: "sensor-err",
		Mode:     "auto",
		Status:   "running",
		Request:  json.RawMessage(`{}`),
	}
	if err := store.InsertSweep(rec); err != nil {
		t.Fatalf("InsertSweep: %v", err)
	}

	// Complete with an error message.
	now := time.Now().UTC()
	err := store.UpdateSweepResults(
		"sweep-err-1", "failed",
		nil, nil, nil, &now,
		"timeout exceeded",
		nil, nil, nil, "", "",
	)
	if err != nil {
		t.Fatalf("UpdateSweepResults: %v", err)
	}

	sweeps, err := store.ListSweeps("sensor-err", 10)
	if err != nil {
		t.Fatalf("ListSweeps: %v", err)
	}
	if len(sweeps) != 1 {
		t.Fatalf("expected 1 sweep, got %d", len(sweeps))
	}
	if sweeps[0].Error != "timeout exceeded" {
		t.Errorf("Error = %q, want %q", sweeps[0].Error, "timeout exceeded")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: AnalysisRunManager closed-DB error paths
// ---------------------------------------------------------------------------

func TestARM_StartRun_DBError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Close the DB so InsertRun fails.
	db.Close()

	m := NewAnalysisRunManagerDI(db, "sensor-1")
	_, err := m.StartRun("/data/test.pcap", DefaultRunParams())
	if err == nil {
		t.Error("expected error from StartRun with closed DB")
	}
}

func TestARM_RecordTrack_DBError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := NewAnalysisRunManagerDI(db, "sensor-1")

	// Start a run successfully first.
	_, err := m.StartRun("/data/test.pcap", DefaultRunParams())
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Close DB so InsertRunTrack fails.
	db.Close()

	track := &TrackedObject{TrackID: "track-arm-1", SensorID: "sensor-1"}
	result := m.RecordTrack(track)
	if result {
		t.Error("expected RecordTrack to return false with closed DB")
	}
}

func TestARM_CompleteRun_DBError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := NewAnalysisRunManagerDI(db, "sensor-1")

	_, err := m.StartRun("/data/test.pcap", DefaultRunParams())
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	db.Close()

	err = m.CompleteRun()
	if err == nil {
		t.Error("expected error from CompleteRun with closed DB")
	}
}

func TestARM_FailRun_DBError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := NewAnalysisRunManagerDI(db, "sensor-1")

	_, err := m.StartRun("/data/test.pcap", DefaultRunParams())
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	db.Close()

	err = m.FailRun("something broke")
	if err == nil {
		t.Error("expected error from FailRun with closed DB")
	}
}

func TestARM_GetCurrentRunParams_BadJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := NewAnalysisRunManagerDI(db, "sensor-1")

	_, err := m.StartRun("/data/test.pcap", DefaultRunParams())
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Corrupt the ParamsJSON so Unmarshal fails.
	m.currentRun.ParamsJSON = json.RawMessage(`{invalid json`)

	_, ok := m.GetCurrentRunParams()
	if ok {
		t.Error("expected GetCurrentRunParams to return false with invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: closed-DB error paths for track_store functions
// ---------------------------------------------------------------------------

func TestClosedDB_InsertCluster(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	cluster := &WorldCluster{
		SensorID:    "sensor-1",
		WorldFrame:  "frame-1",
		TSUnixNanos: 1000,
		CentroidX:   1.0,
	}
	_, err := InsertCluster(db, cluster)
	if err == nil {
		t.Error("expected error from InsertCluster with closed DB")
	}
}

func TestClosedDB_InsertTrack(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	track := &TrackedObject{TrackID: "t1", SensorID: "sensor-1"}
	err := InsertTrack(db, track, "frame-1")
	if err == nil {
		t.Error("expected error from InsertTrack with closed DB")
	}
}

func TestClosedDB_UpdateTrack(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	track := &TrackedObject{TrackID: "t1", SensorID: "sensor-1"}
	err := UpdateTrack(db, track, "frame-1")
	if err == nil {
		t.Error("expected error from UpdateTrack with closed DB")
	}
}

func TestClosedDB_InsertTrackObservation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	obs := &TrackObservation{TrackID: "t1", TSUnixNanos: 1000}
	err := InsertTrackObservation(db, obs)
	if err == nil {
		t.Error("expected error from InsertTrackObservation with closed DB")
	}
}

func TestClosedDB_GetTrackObservationsInRange(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	_, err := GetTrackObservationsInRange(db, "sensor-1", 0, 9999, 10, "")
	if err == nil {
		t.Error("expected error from GetTrackObservationsInRange with closed DB")
	}
}

func TestClosedDB_GetActiveTracks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	_, err := GetActiveTracks(db, "sensor-1", "")
	if err == nil {
		t.Error("expected error from GetActiveTracks with closed DB")
	}
}

func TestClosedDB_GetTracksInRange(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	_, err := GetTracksInRange(db, "sensor-1", "", 0, 9999, 10)
	if err == nil {
		t.Error("expected error from GetTracksInRange with closed DB")
	}
}

func TestClosedDB_GetTrackObservations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	_, err := GetTrackObservations(db, "t1", 10)
	if err == nil {
		t.Error("expected error from GetTrackObservations with closed DB")
	}
}

func TestClosedDB_GetRecentClusters(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	_, err := GetRecentClusters(db, "sensor-1", 0, 9999, 10)
	if err == nil {
		t.Error("expected error from GetRecentClusters with closed DB")
	}
}

func TestClosedDB_PruneDeletedTracks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	_, err := PruneDeletedTracks(db, "sensor-1", time.Hour)
	if err == nil {
		t.Error("expected error from PruneDeletedTracks with closed DB")
	}
}

func TestClosedDB_ClearTracks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	err := ClearTracks(db, "sensor-1")
	if err == nil {
		t.Error("expected error from ClearTracks with closed DB")
	}
}

func TestClosedDB_ClearRuns(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	err := ClearRuns(db, "sensor-1")
	if err == nil {
		t.Error("expected error from ClearRuns with closed DB")
	}
}

func TestClosedDB_DeleteRun(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	err := DeleteRun(db, "run-1")
	if err == nil {
		t.Error("expected error from DeleteRun with closed DB")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: closed-DB error paths for analysis_run store
// ---------------------------------------------------------------------------

func TestClosedDB_AnalysisRunStore_InsertRun(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewAnalysisRunStore(db)
	run := &AnalysisRun{
		RunID:      "run-err-1",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		ParamsJSON: json.RawMessage(`{}`),
		Status:     "running",
	}
	err := store.InsertRun(run)
	if err == nil {
		t.Error("expected error from InsertRun with closed DB")
	}
}

func TestClosedDB_AnalysisRunStore_UpdateRunStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewAnalysisRunStore(db)
	err := store.UpdateRunStatus("run-1", "failed", "test error")
	if err == nil {
		t.Error("expected error from UpdateRunStatus with closed DB")
	}
}

func TestClosedDB_AnalysisRunStore_UpdateRunVRLogPath(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewAnalysisRunStore(db)
	err := store.UpdateRunVRLogPath("run-1", "/path/to/file.vrlog")
	if err == nil {
		t.Error("expected error from UpdateRunVRLogPath with closed DB")
	}
}

func TestClosedDB_AnalysisRunStore_CompleteRun(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewAnalysisRunStore(db)
	stats := &AnalysisStats{TotalFrames: 100}
	err := store.CompleteRun("run-1", stats)
	if err == nil {
		t.Error("expected error from CompleteRun with closed DB")
	}
}

func TestClosedDB_AnalysisRunStore_ListRuns(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewAnalysisRunStore(db)
	_, err := store.ListRuns(10)
	if err == nil {
		t.Error("expected error from ListRuns with closed DB")
	}
}

func TestClosedDB_AnalysisRunStore_GetRunTracks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewAnalysisRunStore(db)
	_, err := store.GetRunTracks("run-1")
	if err == nil {
		t.Error("expected error from GetRunTracks with closed DB")
	}
}

func TestClosedDB_AnalysisRunStore_InsertRunTrack(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewAnalysisRunStore(db)
	track := &RunTrack{
		RunID:    "run-1",
		TrackID:  "t1",
		SensorID: "sensor-1",
	}
	err := store.InsertRunTrack(track)
	if err == nil {
		t.Error("expected error from InsertRunTrack with closed DB")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: closed-DB error paths for sweep store
// ---------------------------------------------------------------------------

func TestClosedDB_SweepStore_InsertSweep(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewSweepStore(db)
	rec := SweepRecord{
		SweepID:  "s1",
		SensorID: "sensor-1",
		Mode:     "auto",
		Status:   "running",
		Request:  json.RawMessage(`{}`),
	}
	err := store.InsertSweep(rec)
	if err == nil {
		t.Error("expected error from InsertSweep with closed DB")
	}
}

func TestClosedDB_SweepStore_GetSweep(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewSweepStore(db)
	_, err := store.GetSweep("s1")
	if err == nil {
		t.Error("expected error from GetSweep with closed DB")
	}
}

func TestClosedDB_SweepStore_ListSweeps(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewSweepStore(db)
	_, err := store.ListSweeps("sensor-1", 10)
	if err == nil {
		t.Error("expected error from ListSweeps with closed DB")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: closed-DB error paths for evaluation/scene/missed stores
// ---------------------------------------------------------------------------

func TestClosedDB_EvaluationStore_ListByScene(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewEvaluationStore(db)
	_, err := store.ListByScene("scene-1")
	if err == nil {
		t.Error("expected error from ListByScene with closed DB")
	}
}

func TestClosedDB_EvaluationStore_Get(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewEvaluationStore(db)
	_, err := store.Get("eval-1")
	if err == nil {
		t.Error("expected error from Get with closed DB")
	}
}

func TestClosedDB_SceneStore_ListScenes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create scene store before closing — needs *sql.DB.
	store := NewSceneStore(db)
	db.Close()

	_, err := store.ListScenes("sensor-1")
	if err == nil {
		t.Error("expected error from ListScenes with closed DB")
	}
}

func TestClosedDB_MissedRegionStore_ListByRun(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	db.Close()

	store := NewMissedRegionStore(db)
	_, err := store.ListByRun("run-1")
	if err == nil {
		t.Error("expected error from ListByRun with closed DB")
	}
}

// ---------------------------------------------------------------------------
// Coverage boost: DeleteRun with non-existent run ID (rows affected == 0)
// ---------------------------------------------------------------------------

func TestCovBoost_DeleteRun_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := DeleteRun(db, "non-existent-run-id")
	if err == nil {
		t.Error("expected error when deleting non-existent run")
	}
}

// ---------------------------------------------------------------------------
// setupTestDB helper for closed-DB tests that need raw *sql.DB
// ---------------------------------------------------------------------------

func openRawSQLite(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	return db
}
