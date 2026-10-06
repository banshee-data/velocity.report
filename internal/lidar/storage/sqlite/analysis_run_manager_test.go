package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

func setupAnalysisRunDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	db, cleanup := dbpkg.NewTestDB(t)
	return db.DB, cleanup
}

func TestNewAnalysisRunManager(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	if manager == nil {
		t.Fatal("NewAnalysisRunManager returned nil")
	}

	if manager.sensorID != "test-sensor" {
		t.Errorf("Expected sensorID 'test-sensor', got %s", manager.sensorID)
	}

	if manager.store == nil {
		t.Error("Expected store to be initialized")
	}

	if manager.tracksSeen == nil {
		t.Error("Expected tracksSeen map to be initialized")
	}

	if manager.currentRun != nil {
		t.Error("Expected currentRun to be nil initially")
	}
}

func TestNewAnalysisRunManagerDI(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	// Clear registry for test isolation
	armMu.Lock()
	armRegistry = make(map[string]*AnalysisRunManager)
	armMu.Unlock()

	manager := NewAnalysisRunManagerDI(db, "sensor-di")
	if manager == nil {
		t.Fatal("NewAnalysisRunManagerDI returned nil")
	}

	if manager.sensorID != "sensor-di" {
		t.Errorf("Expected sensorID 'sensor-di', got %s", manager.sensorID)
	}

	if manager.store == nil {
		t.Error("Expected store to be initialised")
	}

	if manager.tracksSeen == nil {
		t.Error("Expected tracksSeen map to be initialised")
	}

	if manager.currentRun != nil {
		t.Error("Expected currentRun to be nil initially")
	}

	// Verify DI constructor does NOT register in global registry
	retrieved := GetAnalysisRunManager("sensor-di")
	if retrieved != nil {
		t.Error("Expected DI constructor NOT to register in global registry")
	}
}

func TestAnalysisRunManagerRegistry(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	// Clear registry for test isolation
	armMu.Lock()
	armRegistry = make(map[string]*AnalysisRunManager)
	armMu.Unlock()

	manager := NewAnalysisRunManager(db, "sensor-1")
	RegisterAnalysisRunManager("sensor-1", manager)

	retrieved := GetAnalysisRunManager("sensor-1")
	if retrieved == nil {
		t.Fatal("GetAnalysisRunManager returned nil")
	}

	if retrieved != manager {
		t.Error("Retrieved manager is not the same instance")
	}

	// Test non-existent sensor
	notFound := GetAnalysisRunManager("non-existent")
	if notFound != nil {
		t.Error("Expected nil for non-existent sensor")
	}
}

func TestStartRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	runID, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	if runID == "" {
		t.Error("Expected non-empty run ID")
	}

	// Verify run is active
	if !manager.IsRunActive() {
		t.Error("Expected run to be active after StartRun")
	}

	// Verify current run ID matches
	currentID := manager.CurrentRunID()
	if currentID != runID {
		t.Errorf("CurrentRunID mismatch: got %s, want %s", currentID, runID)
	}

	// Verify run is in database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_run_records WHERE run_id = ?", runID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 run in database, got %d", count)
	}
}

func TestRecordFrame(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	_, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Record frames
	for i := 0; i < 100; i++ {
		manager.RecordFrame(int64(1000000000 + i*100000000)) // 1s + i*100ms in nanos
	}

	// Verify internal counter
	manager.mu.RLock()
	frameCount := manager.totalFrames
	manager.mu.RUnlock()

	if frameCount != 100 {
		t.Errorf("Expected 100 frames, got %d", frameCount)
	}
}

func TestRecordClusters(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	_, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Record clusters
	manager.RecordClusters(5)
	manager.RecordClusters(3)
	manager.RecordClusters(7)

	// Verify internal counter
	manager.mu.RLock()
	clusterCount := manager.totalClusters
	manager.mu.RUnlock()

	if clusterCount != 15 {
		t.Errorf("Expected 15 clusters, got %d", clusterCount)
	}
}

func TestRecordTrack(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	runID, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Create a test track
	track := &TrackedObject{
		TrackID: "track-001", TrackMeasurement: TrackMeasurement{SensorID: "test-sensor",
			TrackState:       TrackConfirmed,
			StartUnixNanos:   time.Now().UnixNano(),
			EndUnixNanos:     time.Now().Add(5 * time.Second).UnixNano(),
			ObservationCount: 50,
			AvgSpeedMps:      10.5,
			MaxSpeedMps:      15.2,

			ObjectClass:      "vehicle",
			ObjectConfidence: 0.85}, TrackLengthMeters: 52.5,
		TrackDurationSecs: 5.0,
		OcclusionCount:    2,
	}

	// Record track - first time should return true
	isNew := manager.RecordTrack(track)
	if !isNew {
		t.Error("Expected RecordTrack to return true for new track")
	}

	// Record same track again - should return false
	isNew = manager.RecordTrack(track)
	if isNew {
		t.Error("Expected RecordTrack to return false for duplicate track")
	}

	// Verify track is in database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_run_tracks WHERE run_id = ? AND track_id = ?",
		runID, track.TrackID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 track in database, got %d", count)
	}

	// Verify internal tracking
	manager.mu.RLock()
	seen := manager.tracksSeen[track.TrackID]
	manager.mu.RUnlock()

	if !seen {
		t.Error("Expected track to be marked as seen")
	}
}

func TestCompleteRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	runID, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Record some activity
	baseNs := int64(1700000000000000000) // ~2023 timestamp
	for i := 0; i < 100; i++ {
		manager.RecordFrame(baseNs + int64(i)*100000000) // 100ms apart = 10s total
	}
	manager.RecordClusters(50)

	// Record a few tracks
	for i := 0; i < 5; i++ {
		track := &TrackedObject{
			TrackID: fmt.Sprintf("track-%d", i), TrackMeasurement: TrackMeasurement{SensorID: "test-sensor",
				TrackState:       TrackConfirmed,
				StartUnixNanos:   time.Now().UnixNano(),
				EndUnixNanos:     time.Now().Add(time.Second).UnixNano(),
				ObservationCount: 10}, TrackLengthMeters: 10.0,
			TrackDurationSecs: 1.0,
		}
		manager.RecordTrack(track)
	}

	// Sleep briefly to ensure measurable wall-clock (unused now since we use frame timestamps)
	time.Sleep(10 * time.Millisecond)

	// Complete the run
	err = manager.CompleteRun()
	if err != nil {
		t.Fatalf("CompleteRun failed: %v", err)
	}

	// Verify run is no longer active
	if manager.IsRunActive() {
		t.Error("Expected run to be inactive after CompleteRun")
	}

	if manager.CurrentRunID() != "" {
		t.Error("Expected CurrentRunID to be empty after CompleteRun")
	}

	// Verify database status
	var status string
	var totalFrames, totalClusters, totalTracks int
	var durationSecs float64

	err = db.QueryRow(`
SELECT status, total_frames, total_clusters, total_tracks, duration_secs
FROM lidar_run_records WHERE run_id = ?`, runID).Scan(
		&status, &totalFrames, &totalClusters, &totalTracks, &durationSecs)
	if err != nil {
		t.Fatalf("Failed to query completed run: %v", err)
	}

	if status != "completed" {
		t.Errorf("Expected status 'completed', got %s", status)
	}

	if totalFrames != 100 {
		t.Errorf("Expected 100 frames, got %d", totalFrames)
	}

	if totalClusters != 50 {
		t.Errorf("Expected 50 clusters, got %d", totalClusters)
	}

	if totalTracks != 5 {
		t.Errorf("Expected 5 tracks, got %d", totalTracks)
	}

	if durationSecs <= 0 {
		t.Errorf("Expected positive duration, got %f", durationSecs)
	}

	// Duration should be ~9.9 seconds (99 intervals × 100ms) from frame timestamps
	expectedDuration := 9.9
	if durationSecs < expectedDuration-0.5 || durationSecs > expectedDuration+0.5 {
		t.Errorf("Expected duration ~%.1fs from frame timestamps, got %.1fs", expectedDuration, durationSecs)
	}
}

func TestFailRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	runID, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Fail the run
	errMsg := "test error: file not found"
	err = manager.FailRun(errMsg)
	if err != nil {
		t.Fatalf("FailRun failed: %v", err)
	}

	// Verify run is no longer active
	if manager.IsRunActive() {
		t.Error("Expected run to be inactive after FailRun")
	}

	// Verify database status
	var status, storedErrMsg string
	err = db.QueryRow("SELECT status, error_message FROM lidar_run_records WHERE run_id = ?", runID).Scan(&status, &storedErrMsg)
	if err != nil {
		t.Fatalf("Failed to query failed run: %v", err)
	}

	if status != "failed" {
		t.Errorf("Expected status 'failed', got %s", status)
	}

	if storedErrMsg != errMsg {
		t.Errorf("Expected error_message '%s', got '%s'", errMsg, storedErrMsg)
	}
}

func TestRecordTrack_NoActiveRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	// No StartRun → currentRun is nil
	track := &TrackedObject{TrackID: "track-1"}
	result := manager.RecordTrack(track)
	if result {
		t.Error("Expected RecordTrack to return false with no active run")
	}
}

func TestCompleteRun_NoActiveRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	// No StartRun → currentRun is nil
	err := manager.CompleteRun()
	if err != nil {
		t.Errorf("Expected CompleteRun to return nil with no active run, got: %v", err)
	}
}

func TestFailRun_NoActiveRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	// No StartRun → currentRun is nil
	err := manager.FailRun("some error")
	if err != nil {
		t.Errorf("Expected FailRun to return nil with no active run, got: %v", err)
	}
}

func TestCompleteRun_WallClockFallback(t *testing.T) {
	// When RecordFrame is called with timestampNs=0, the wall-clock
	// fallback path should be used for duration calculation.
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	runID, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Record frames with zero timestamps — should NOT set firstFrameNs/lastFrameNs
	for i := 0; i < 10; i++ {
		manager.RecordFrame(0)
	}

	// Small sleep so wall-clock duration > 0
	time.Sleep(10 * time.Millisecond)

	err = manager.CompleteRun()
	if err != nil {
		t.Fatalf("CompleteRun failed: %v", err)
	}

	// Verify duration uses wall-clock fallback (should be > 0 from time.Sleep)
	var durationSecs float64
	err = db.QueryRow("SELECT duration_secs FROM lidar_run_records WHERE run_id = ?", runID).Scan(&durationSecs)
	if err != nil {
		t.Fatalf("Failed to query duration: %v", err)
	}
	if durationSecs <= 0 {
		t.Errorf("Expected positive wall-clock duration, got %.4f", durationSecs)
	}
	// Wall-clock duration for zero timestamps should be short (< 5s)
	if durationSecs > 5 {
		t.Errorf("Wall-clock fallback duration unexpectedly high: %.1f", durationSecs)
	}
}

func TestRecordFrame_ZeroTimestampIgnored(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	_, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Zero timestamps should not set firstFrameNs/lastFrameNs
	manager.RecordFrame(0)
	manager.RecordFrame(0)

	manager.mu.RLock()
	first := manager.firstFrameNs
	last := manager.lastFrameNs
	manager.mu.RUnlock()

	if first != 0 {
		t.Errorf("Expected firstFrameNs=0 for zero timestamps, got %d", first)
	}
	if last != 0 {
		t.Errorf("Expected lastFrameNs=0 for zero timestamps, got %d", last)
	}

	// Now record a valid timestamp
	manager.RecordFrame(1700000000000000000)

	manager.mu.RLock()
	first = manager.firstFrameNs
	last = manager.lastFrameNs
	manager.mu.RUnlock()

	if first != 1700000000000000000 {
		t.Errorf("Expected firstFrameNs set after valid timestamp, got %d", first)
	}
	if last != 1700000000000000000 {
		t.Errorf("Expected lastFrameNs set after valid timestamp, got %d", last)
	}
}

func TestStartRunWithConfig_PersistsImmutableProvenance(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "immutable-sensor")
	store := NewAnalysisRunStore(db)

	cfg := cfgpkg.MustLoadDefaultConfig()
	cfg.L1.Sensor = "immutable-sensor"

	if err := store.InsertRun(&AnalysisRun{
		RunID:      "parent-run",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SourcePath: "/tmp/parent.pcap",
		SensorID:   "immutable-sensor",
		Status:     "completed",
	}); err != nil {
		t.Fatalf("InsertRun(parent) failed: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO lidar_replay_cases (
			replay_case_id, sensor_id, pcap_file, created_at_ns
		) VALUES (?, ?, ?, ?)
	`, "scene-42", "immutable-sensor", "/tmp/replay.pcap", time.Now().UnixNano()); err != nil {
		t.Fatalf("insert replay case: %v", err)
	}

	runID, err := manager.StartRunWithConfig(AnalysisRunStartOptions{
		SourcePath:          "/tmp/replay.pcap",
		SensorID:            "immutable-sensor",
		ParentRunID:         "parent-run",
		ReplayCaseID:        "scene-42",
		RequestedParamsJSON: json.RawMessage(`{"tracking":{"max_tracks":64}}`),
		EffectiveConfig:     cfg,
	})
	if err != nil {
		t.Fatalf("StartRunWithConfig failed: %v", err)
	}

	run, err := store.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if run.RunConfigID == "" {
		t.Fatal("expected run_config_id to be persisted")
	}
	if run.RequestedParamSetID == "" {
		t.Fatal("expected requested_param_set_id to be persisted")
	}
	if run.ParentRunID != "parent-run" {
		t.Fatalf("ParentRunID = %q, want %q", run.ParentRunID, "parent-run")
	}
	if run.ReplayCaseID != "scene-42" {
		t.Fatalf("ReplayCaseID = %q, want %q", run.ReplayCaseID, "scene-42")
	}

	var paramSetCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_param_sets`).Scan(&paramSetCount); err != nil {
		t.Fatalf("count lidar_param_sets: %v", err)
	}
	if paramSetCount != 2 {
		t.Fatalf("expected requested + effective param sets, got %d", paramSetCount)
	}

	var runConfigCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_run_configs`).Scan(&runConfigCount); err != nil {
		t.Fatalf("count lidar_run_configs: %v", err)
	}
	if runConfigCount != 1 {
		t.Fatalf("expected 1 run config row, got %d", runConfigCount)
	}
}

func TestCompleteRun_PersistsFrameBoundsWithImmutableConfig(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "frame-sensor")
	store := NewAnalysisRunStore(db)

	cfg := cfgpkg.MustLoadDefaultConfig()
	cfg.L1.Sensor = "frame-sensor"

	runID, err := manager.StartRunWithConfig(AnalysisRunStartOptions{
		SourcePath:      "/tmp/frames.pcap",
		SensorID:        "frame-sensor",
		EffectiveConfig: cfg,
	})
	if err != nil {
		t.Fatalf("StartRunWithConfig failed: %v", err)
	}

	manager.RecordFrame(100)
	manager.RecordFrame(250)
	manager.RecordClusters(3)

	if err := manager.CompleteRun(); err != nil {
		t.Fatalf("CompleteRun failed: %v", err)
	}

	run, err := store.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if run.Status != "completed" {
		t.Fatalf("Status = %q, want completed", run.Status)
	}
	if run.CompletedAt == nil {
		t.Fatal("expected completed_at to be persisted")
	}
	if run.FrameStartNs == nil || *run.FrameStartNs != 100 {
		t.Fatalf("FrameStartNs = %v, want 100", run.FrameStartNs)
	}
	if run.FrameEndNs == nil || *run.FrameEndNs != 250 {
		t.Fatalf("FrameEndNs = %v, want 250", run.FrameEndNs)
	}
}

// A completed run stores its statistics, computed from every track as it was
// last offered rather than as it was first seen: a track that travels further
// after its first sighting counts at its final length.
func TestCompleteRunStoresStatisticsFromFinalTracks(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager := NewAnalysisRunManager(db, "test-sensor")
	runID, err := manager.StartRun("/path/to/test.pcap", DefaultRunParams())
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}
	const second = int64(1_000_000_000)
	base := int64(1_700_000_000) * second
	mover := &TrackedObject{TrackID: "track-mover", TrackMeasurement: TrackMeasurement{
		SensorID: "test-sensor", TrackState: TrackConfirmed, StartUnixNanos: base, EndUnixNanos: base + second,
		ObservationCount: 10, ObjectClass: "car", ObjectConfidence: 0.8,
	}}
	mover.TrackLengthMeters = 4
	manager.RecordTrack(mover)

	// The tracker keeps updating the track after its first sighting. Its
	// trail is capped and holds coasted points, so the statistics read the
	// lifetime counters, not a recount of the trail: here it holds two
	// points 1 m apart and no gap, against 300 m and 4 missed frames in gaps
	// it was seen again after. The 14 more it is coasting out on are not
	// occlusions.
	mover.History = []TrackPoint{{X: 0, Y: 0, Timestamp: base}, {X: 1, Y: 0, Timestamp: base + second/10}}
	mover.TrackLengthMeters, mover.ClosedOcclusionCount, mover.OcclusionCount = 300, 4, 18
	mover.EndUnixNanos, mover.ObservationCount = base+2*second, 20
	manager.RecordTrack(mover)

	parked := &TrackedObject{TrackID: "track-parked", TrackMeasurement: TrackMeasurement{
		SensorID: "test-sensor", TrackState: TrackConfirmed, StartUnixNanos: base, EndUnixNanos: base + 2*second,
		ObservationCount: 10,
	}}
	manager.RecordTrack(parked)

	if err := manager.CompleteRun(); err != nil {
		t.Fatalf("CompleteRun failed: %v", err)
	}
	run, err := NewAnalysisRunStore(db).GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if len(run.StatisticsJSON) == 0 {
		t.Fatal("completed run has no statistics_json")
	}
	stats, err := l8analytics.ParseRunStatistics(string(run.StatisticsJSON))
	if err != nil {
		t.Fatal(err)
	}
	// The mover ends 300 m along (not the 4 m it had at its first sighting),
	// the parked car 0 m: mean 150 m, median the larger of two, 300 m.
	if stats.AvgTrackLength != 150 || stats.MedianTrackLength != 300 || stats.AvgTrackDuration != 2 {
		t.Fatalf("lengths %v / %v, duration %v; want the tracks as they ended", stats.AvgTrackLength, stats.MedianTrackLength, stats.AvgTrackDuration)
	}
	// 4 missed frames over two tracks; 20 and 10 observations in 2 s at
	// 10 Hz cover 1 and 0.5.
	if stats.AvgOcclusionCount != 2 || stats.AvgSpatialCoverage != 0.75 {
		t.Fatalf("occlusions %v, coverage %v; want the tracker's counters", stats.AvgOcclusionCount, stats.AvgSpatialCoverage)
	}
	if stats.ClassCounts["car"] != 1 || stats.ClassCounts["dynamic"] != 1 || stats.ConfirmedRatio != 1 {
		t.Fatalf("classes %v, confirmed ratio %v", stats.ClassCounts, stats.ConfirmedRatio)
	}
}

// A run that recorded no track completes with no statistics, as before.
func TestCompleteRunWithoutTracksStoresNoStatistics(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager := NewAnalysisRunManager(db, "test-sensor")
	runID, err := manager.StartRun("/path/to/empty.pcap", DefaultRunParams())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CompleteRun(); err != nil {
		t.Fatal(err)
	}
	run, err := NewAnalysisRunStore(db).GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.StatisticsJSON) != 0 {
		t.Fatalf("statistics_json %s on a run with no tracks", run.StatisticsJSON)
	}
}

// RecordTrack reads the caller's track and leaves it as it was: the pipeline
// writes the same copy to lidar_tracks next.
func TestRecordTrackLeavesTheCallersTrackUnchanged(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager := NewAnalysisRunManager(db, "test-sensor")
	if _, err := manager.StartRun("/path/to/test.pcap", DefaultRunParams()); err != nil {
		t.Fatal(err)
	}
	const second = int64(1_000_000_000)
	track := &TrackedObject{TrackID: "track-kept", TrackMeasurement: TrackMeasurement{
		SensorID: "test-sensor", TrackState: TrackConfirmed, StartUnixNanos: second, EndUnixNanos: 3 * second,
		ObservationCount: 15,
	}}
	// A trail that recounts to other values than the tracker's counters.
	track.History = []TrackPoint{{X: 0, Y: 0, Timestamp: second}, {X: 1, Y: 0, Timestamp: 2 * second}}
	track.TrackLengthMeters, track.OcclusionCount, track.MaxOcclusionFrames = 40, 9, 5
	track.ClosedOcclusionCount, track.MaxClosedOcclusionFrames = 3, 2
	before := *track
	manager.RecordTrack(track)
	if track.TrackLengthMeters != before.TrackLengthMeters || track.OcclusionCount != before.OcclusionCount ||
		track.MaxOcclusionFrames != before.MaxOcclusionFrames || track.SpatialCoverage != before.SpatialCoverage ||
		track.TrackDurationSecs != before.TrackDurationSecs {
		t.Fatalf("RecordTrack changed the track: length %v, occlusions %d/%d, coverage %v, duration %v",
			track.TrackLengthMeters, track.OcclusionCount, track.MaxOcclusionFrames, track.SpatialCoverage, track.TrackDurationSecs)
	}
}

// A second run starts without the first run's tracks.
func TestRunStatisticsDoNotCarryIntoTheNextRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager := NewAnalysisRunManager(db, "test-sensor")
	const second = int64(1_000_000_000)
	record := func(id string, length float32) {
		tr := &TrackedObject{TrackID: id, TrackMeasurement: TrackMeasurement{
			SensorID: "test-sensor", TrackState: TrackConfirmed, StartUnixNanos: second, EndUnixNanos: 2 * second,
			ObservationCount: 10,
		}}
		tr.TrackLengthMeters = length
		manager.RecordTrack(tr)
	}
	if _, err := manager.StartRun("/path/to/first.pcap", DefaultRunParams()); err != nil {
		t.Fatal(err)
	}
	record("first-a", 100)
	record("first-b", 100)
	if err := manager.CompleteRun(); err != nil {
		t.Fatal(err)
	}
	secondID, err := manager.StartRun("/path/to/second.pcap", DefaultRunParams())
	if err != nil {
		t.Fatal(err)
	}
	record("second-a", 10)
	if err := manager.CompleteRun(); err != nil {
		t.Fatal(err)
	}
	run, err := NewAnalysisRunStore(db).GetRun(secondID)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := l8analytics.ParseRunStatistics(string(run.StatisticsJSON))
	if err != nil {
		t.Fatal(err)
	}
	if stats.AvgTrackLength != 10 || stats.ClassCounts["dynamic"] != 1 {
		t.Fatalf("second run: mean length %v over %v; want 10 over its one track", stats.AvgTrackLength, stats.ClassCounts)
	}
}

// Statistics that cannot be encoded (a NaN) do not strand the run as
// running: it completes without them.
func TestCompleteRunWithUnencodableStatisticsStillCompletes(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	store := NewAnalysisRunStore(db)
	manager := NewAnalysisRunManager(db, "test-sensor")
	runID, err := manager.StartRun("/path/to/nan.pcap", DefaultRunParams())
	if err != nil {
		t.Fatal(err)
	}
	nan := float32(math.NaN())
	if err := store.CompleteRun(runID, &AnalysisStats{Statistics: &l8analytics.RunStatistics{AvgTrackLength: nan}}); err != nil {
		t.Fatalf("CompleteRun failed on unencodable statistics: %v", err)
	}
	run, err := store.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "completed" || len(run.StatisticsJSON) != 0 {
		t.Fatalf("status %q, statistics %q; want completed without statistics", run.Status, run.StatisticsJSON)
	}
}
