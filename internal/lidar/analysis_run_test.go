package lidar

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
)

func TestRunParams_Serialization(t *testing.T) {
	params := DefaultRunParams()
	params.Timestamp = time.Date(2025, 12, 1, 12, 0, 0, 0, time.UTC)

	// Test ToJSON
	jsonBytes, err := params.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Test ParseRunParams
	parsed, err := ParseRunParams(jsonBytes)
	if err != nil {
		t.Fatalf("ParseRunParams failed: %v", err)
	}

	if parsed.Version != params.Version {
		t.Errorf("Version mismatch: got %s, want %s", parsed.Version, params.Version)
	}

	if parsed.Background.BackgroundUpdateFraction != params.Background.BackgroundUpdateFraction {
		t.Errorf("BackgroundUpdateFraction mismatch: got %f, want %f",
			parsed.Background.BackgroundUpdateFraction, params.Background.BackgroundUpdateFraction)
	}

	if parsed.Clustering.Eps != params.Clustering.Eps {
		t.Errorf("Eps mismatch: got %f, want %f", parsed.Clustering.Eps, params.Clustering.Eps)
	}

	if parsed.Tracking.MaxTracks != params.Tracking.MaxTracks {
		t.Errorf("MaxTracks mismatch: got %d, want %d",
			parsed.Tracking.MaxTracks, params.Tracking.MaxTracks)
	}
}

func TestFromBackgroundParams(t *testing.T) {
	bg := BackgroundParams{
		BackgroundUpdateFraction:       0.05,
		ClosenessSensitivityMultiplier: 2.5,
		SafetyMarginMeters:             0.3,
		NeighborConfirmationCount:      4,
		NoiseRelativeFraction:          0.02,
		SeedFromFirstObservation:       true,
		FreezeDurationNanos:            3e9,
	}

	export := FromBackgroundParams(bg)

	if export.BackgroundUpdateFraction != bg.BackgroundUpdateFraction {
		t.Errorf("BackgroundUpdateFraction mismatch")
	}
	if export.ClosenessSensitivityMultiplier != bg.ClosenessSensitivityMultiplier {
		t.Errorf("ClosenessSensitivityMultiplier mismatch")
	}
	if export.SafetyMarginMeters != bg.SafetyMarginMeters {
		t.Errorf("SafetyMarginMeters mismatch")
	}
	if export.NeighborConfirmationCount != bg.NeighborConfirmationCount {
		t.Errorf("NeighborConfirmationCount mismatch")
	}
	if export.NoiseRelativeFraction != bg.NoiseRelativeFraction {
		t.Errorf("NoiseRelativeFraction mismatch")
	}
	if export.SeedFromFirstObservation != bg.SeedFromFirstObservation {
		t.Errorf("SeedFromFirstObservation mismatch")
	}
	if export.FreezeDurationNanos != bg.FreezeDurationNanos {
		t.Errorf("FreezeDurationNanos mismatch")
	}
}

func TestFromDBSCANParams(t *testing.T) {
	dbscan := DBSCANParams{
		Eps:    0.8,
		MinPts: 15,
	}

	export := FromDBSCANParams(dbscan)

	if export.Eps != dbscan.Eps {
		t.Errorf("Eps mismatch: got %f, want %f", export.Eps, dbscan.Eps)
	}
	if export.MinPts != dbscan.MinPts {
		t.Errorf("MinPts mismatch: got %d, want %d", export.MinPts, dbscan.MinPts)
	}
}

func TestFromTrackerConfig(t *testing.T) {
	config := TrackerConfig{
		MaxTracks:               50,
		MaxMisses:               5,
		HitsToConfirm:           4,
		GatingDistanceSquared:   36.0,
		ProcessNoisePos:         0.15,
		ProcessNoiseVel:         0.6,
		MeasurementNoise:        0.25,
		DeletedTrackGracePeriod: 10 * time.Second,
	}

	export := FromTrackerConfig(config)

	if export.MaxTracks != config.MaxTracks {
		t.Errorf("MaxTracks mismatch")
	}
	if export.MaxMisses != config.MaxMisses {
		t.Errorf("MaxMisses mismatch")
	}
	if export.HitsToConfirm != config.HitsToConfirm {
		t.Errorf("HitsToConfirm mismatch")
	}
	if export.GatingDistanceSquared != config.GatingDistanceSquared {
		t.Errorf("GatingDistanceSquared mismatch")
	}
	if export.ProcessNoisePos != config.ProcessNoisePos {
		t.Errorf("ProcessNoisePos mismatch")
	}
	if export.ProcessNoiseVel != config.ProcessNoiseVel {
		t.Errorf("ProcessNoiseVel mismatch")
	}
	if export.MeasurementNoise != config.MeasurementNoise {
		t.Errorf("MeasurementNoise mismatch")
	}
	if export.DeletedTrackGracePeriod != config.DeletedTrackGracePeriod {
		t.Errorf("DeletedTrackGracePeriod mismatch")
	}
}

func TestRunTrackFromTrackedObject(t *testing.T) {
	track := &TrackedObject{
		TrackID:              "track_1",
		SensorID:             "sensor_1",
		State:                TrackConfirmed,
		FirstUnixNanos:       1000000000,
		LastUnixNanos:        2000000000,
		ObservationCount:     10,
		AvgSpeedMps:          5.0,
		PeakSpeedMps:         8.0,
		BoundingBoxLengthAvg: 2.5,
		BoundingBoxWidthAvg:  1.5,
		BoundingBoxHeightAvg: 1.7,
		HeightP95Max:         1.9,
		IntensityMeanAvg:     120.0,
		ObjectClass:          "pedestrian",
		ObjectConfidence:     0.85,
		ClassificationModel:  "rule-based-v1.0",
		speedHistory:         []float32{4.0, 5.0, 6.0, 5.5, 5.0},
	}

	runTrack := RunTrackFromTrackedObject("run_123", track)

	if runTrack.RunID != "run_123" {
		t.Errorf("RunID mismatch")
	}
	if runTrack.TrackID != track.TrackID {
		t.Errorf("TrackID mismatch")
	}
	if runTrack.SensorID != track.SensorID {
		t.Errorf("SensorID mismatch")
	}
	if runTrack.TrackState != string(track.State) {
		t.Errorf("TrackState mismatch")
	}
	if runTrack.StartUnixNanos != track.FirstUnixNanos {
		t.Errorf("StartUnixNanos mismatch")
	}
	if runTrack.EndUnixNanos != track.LastUnixNanos {
		t.Errorf("EndUnixNanos mismatch")
	}
	if runTrack.ObservationCount != track.ObservationCount {
		t.Errorf("ObservationCount mismatch")
	}
	if runTrack.AvgSpeedMps != track.AvgSpeedMps {
		t.Errorf("AvgSpeedMps mismatch")
	}
	if runTrack.ObjectClass != track.ObjectClass {
		t.Errorf("ObjectClass mismatch")
	}

	// Verify percentiles were computed
	if runTrack.P50SpeedMps <= 0 {
		t.Errorf("P50SpeedMps should be computed")
	}
}

func TestAnalysisRun_JSONParams(t *testing.T) {
	params := DefaultRunParams()
	paramsJSON, err := params.ToJSON()
	if err != nil {
		t.Fatalf("Failed to serialize params: %v", err)
	}

	run := &AnalysisRun{
		RunID:            "test_run_001",
		CreatedAt:        time.Now(),
		SourceType:       "pcap",
		SourcePath:       "/path/to/test.pcap",
		SensorID:         "sensor_1",
		ParamsJSON:       paramsJSON,
		DurationSecs:     120.5,
		TotalFrames:      1000,
		TotalClusters:    500,
		TotalTracks:      50,
		ConfirmedTracks:  45,
		ProcessingTimeMs: 5000,
		Status:           "completed",
	}

	// Verify we can round-trip the params
	parsed, err := ParseRunParams(run.ParamsJSON)
	if err != nil {
		t.Fatalf("Failed to parse params from run: %v", err)
	}

	if parsed.Version != params.Version {
		t.Errorf("Version mismatch after round-trip")
	}
}

func TestDefaultRunParams_HasCorrectValues(t *testing.T) {
	params := DefaultRunParams()

	if params.Version != "1.0" {
		t.Errorf("Version should be 1.0, got %s", params.Version)
	}

	// Structural: background fields are within valid ranges.
	if params.Background.BackgroundUpdateFraction <= 0 || params.Background.BackgroundUpdateFraction > 1 {
		t.Errorf("BackgroundUpdateFraction must be in (0, 1], got %f", params.Background.BackgroundUpdateFraction)
	}
	if params.Background.ClosenessSensitivityMultiplier <= 0 {
		t.Errorf("ClosenessSensitivityMultiplier must be positive, got %f", params.Background.ClosenessSensitivityMultiplier)
	}

	// Clustering defaults must match the loaded config (dynamic, not hardcoded).
	cfg := config.MustLoadDefaultConfig()
	if params.Clustering.Eps != cfg.GetForegroundDBSCANEps() {
		t.Errorf("Eps should match config foreground_dbscan_eps")
	}
	if params.Clustering.MinPts != cfg.GetForegroundMinClusterPoints() {
		t.Errorf("MinPts should match config foreground_min_cluster_points")
	}

	// Structural: tracking fields are within valid ranges.
	if params.Tracking.MaxTracks < 1 {
		t.Errorf("MaxTracks must be >= 1, got %d", params.Tracking.MaxTracks)
	}
	if params.Tracking.MaxMisses < 1 {
		t.Errorf("MaxMisses must be >= 1, got %d", params.Tracking.MaxMisses)
	}
	if params.Tracking.HitsToConfirm < 1 {
		t.Errorf("HitsToConfirm must be >= 1, got %d", params.Tracking.HitsToConfirm)
	}

	// Check classification defaults.
	if params.Classification.ModelType != "rule_based" {
		t.Errorf("ModelType should be rule_based")
	}
}

func TestRunTrack_LinkedTrackIDs_Serialization(t *testing.T) {
	track := &RunTrack{
		RunID:          "run_1",
		TrackID:        "track_1",
		LinkedTrackIDs: []string{"track_2", "track_3"},
	}

	// Marshal to JSON
	data, err := json.Marshal(track)
	if err != nil {
		t.Fatalf("Failed to marshal RunTrack: %v", err)
	}

	// Unmarshal back
	var parsed RunTrack
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal RunTrack: %v", err)
	}

	if len(parsed.LinkedTrackIDs) != 2 {
		t.Errorf("LinkedTrackIDs should have 2 elements, got %d", len(parsed.LinkedTrackIDs))
	}
	if parsed.LinkedTrackIDs[0] != "track_2" {
		t.Errorf("First linked track should be track_2")
	}
	if parsed.LinkedTrackIDs[1] != "track_3" {
		t.Errorf("Second linked track should be track_3")
	}
}

func TestRunComparison_Structure(t *testing.T) {
	comparison := RunComparison{
		Run1ID: "run_1",
		Run2ID: "run_2",
		ParamDiff: map[string]any{
			"clustering.eps": map[string]any{
				"run1": 0.6,
				"run2": 0.8,
			},
		},
		TracksOnlyRun1: []string{"track_1"},
		TracksOnlyRun2: []string{"track_2", "track_3"},
		SplitCandidates: []TrackSplit{
			{
				OriginalTrack: "track_4",
				SplitTracks:   []string{"track_4a", "track_4b"},
				Confidence:    0.85,
			},
		},
		MergeCandidates: []TrackMerge{
			{
				MergedTrack:  "track_5",
				SourceTracks: []string{"track_5a", "track_5b"},
				Confidence:   0.90,
			},
		},
		MatchedTracks: []TrackMatch{
			{Track1ID: "track_6", Track2ID: "track_6", OverlapPct: 0.95},
		},
	}

	// Verify serialization
	data, err := json.Marshal(comparison)
	if err != nil {
		t.Fatalf("Failed to marshal RunComparison: %v", err)
	}

	var parsed RunComparison
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal RunComparison: %v", err)
	}

	if parsed.Run1ID != comparison.Run1ID {
		t.Errorf("Run1ID mismatch")
	}
	if len(parsed.SplitCandidates) != 1 {
		t.Errorf("SplitCandidates should have 1 element")
	}
	if len(parsed.MergeCandidates) != 1 {
		t.Errorf("MergeCandidates should have 1 element")
	}
}

func TestCompareRuns_EmptyRuns(t *testing.T) {
	// Test with empty runs
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Create two empty runs
	run1 := &AnalysisRun{
		RunID:      "empty_run_1",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor_1",
		Status:     "completed",
	}
	run2 := &AnalysisRun{
		RunID:      "empty_run_2",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor_1",
		Status:     "completed",
	}

	if err := store.InsertRun(run1); err != nil {
		t.Fatalf("Failed to insert run1: %v", err)
	}
	if err := store.InsertRun(run2); err != nil {
		t.Fatalf("Failed to insert run2: %v", err)
	}

	// Compare empty runs
	comparison, err := CompareRuns(store, run1.RunID, run2.RunID)
	if err != nil {
		t.Fatalf("CompareRuns failed: %v", err)
	}

	if comparison.Run1ID != run1.RunID {
		t.Errorf("Run1ID mismatch")
	}
	if comparison.Run2ID != run2.RunID {
		t.Errorf("Run2ID mismatch")
	}
	if len(comparison.MatchedTracks) != 0 {
		t.Errorf("Expected no matched tracks, got %d", len(comparison.MatchedTracks))
	}
	if len(comparison.TracksOnlyRun1) != 0 {
		t.Errorf("Expected no tracks only in run1")
	}
	if len(comparison.TracksOnlyRun2) != 0 {
		t.Errorf("Expected no tracks only in run2")
	}
}

func TestCompareRuns_MatchedTracks(t *testing.T) {
	// Test with overlapping tracks
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Create two runs
	run1 := &AnalysisRun{
		RunID:      "matched_run_1",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor_1",
		Status:     "completed",
	}
	run2 := &AnalysisRun{
		RunID:      "matched_run_2",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor_1",
		Status:     "completed",
	}

	if err := store.InsertRun(run1); err != nil {
		t.Fatalf("Failed to insert run1: %v", err)
	}
	if err := store.InsertRun(run2); err != nil {
		t.Fatalf("Failed to insert run2: %v", err)
	}

	// Create tracks with high temporal overlap
	now := time.Now().UnixNano()
	track1 := &RunTrack{
		RunID:            run1.RunID,
		TrackID:          "track_1",
		SensorID:         "sensor_1",
		TrackState:       "confirmed",
		StartUnixNanos:   now,
		EndUnixNanos:     now + int64(10*time.Second),
		ObservationCount: 100,
	}
	track2 := &RunTrack{
		RunID:            run2.RunID,
		TrackID:          "track_1_matched",
		SensorID:         "sensor_1",
		TrackState:       "confirmed",
		StartUnixNanos:   now + int64(1*time.Second), // Slight offset
		EndUnixNanos:     now + int64(11*time.Second),
		ObservationCount: 100,
	}

	if err := store.InsertRunTrack(track1); err != nil {
		t.Fatalf("Failed to insert track1: %v", err)
	}
	if err := store.InsertRunTrack(track2); err != nil {
		t.Fatalf("Failed to insert track2: %v", err)
	}

	// Compare runs
	comparison, err := CompareRuns(store, run1.RunID, run2.RunID)
	if err != nil {
		t.Fatalf("CompareRuns failed: %v", err)
	}

	if len(comparison.MatchedTracks) != 1 {
		t.Fatalf("Expected 1 matched track, got %d", len(comparison.MatchedTracks))
	}

	match := comparison.MatchedTracks[0]
	if match.Track1ID != track1.TrackID {
		t.Errorf("Track1ID mismatch: got %s, want %s", match.Track1ID, track1.TrackID)
	}
	if match.Track2ID != track2.TrackID {
		t.Errorf("Track2ID mismatch: got %s, want %s", match.Track2ID, track2.TrackID)
	}
	if match.OverlapPct < 30.0 {
		t.Errorf("Expected high overlap, got %.2f%%", match.OverlapPct)
	}
}

func TestCompareRuns_NoOverlap(t *testing.T) {
	// Test with non-overlapping tracks
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Create two runs
	run1 := &AnalysisRun{
		RunID:      "no_overlap_run_1",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor_1",
		Status:     "completed",
	}
	run2 := &AnalysisRun{
		RunID:      "no_overlap_run_2",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor_1",
		Status:     "completed",
	}

	if err := store.InsertRun(run1); err != nil {
		t.Fatalf("Failed to insert run1: %v", err)
	}
	if err := store.InsertRun(run2); err != nil {
		t.Fatalf("Failed to insert run2: %v", err)
	}

	// Create tracks with no temporal overlap
	now := time.Now().UnixNano()
	track1 := &RunTrack{
		RunID:            run1.RunID,
		TrackID:          "track_early",
		SensorID:         "sensor_1",
		TrackState:       "confirmed",
		StartUnixNanos:   now,
		EndUnixNanos:     now + int64(5*time.Second),
		ObservationCount: 50,
	}
	track2 := &RunTrack{
		RunID:            run2.RunID,
		TrackID:          "track_late",
		SensorID:         "sensor_1",
		TrackState:       "confirmed",
		StartUnixNanos:   now + int64(10*time.Second), // No overlap
		EndUnixNanos:     now + int64(15*time.Second),
		ObservationCount: 50,
	}

	if err := store.InsertRunTrack(track1); err != nil {
		t.Fatalf("Failed to insert track1: %v", err)
	}
	if err := store.InsertRunTrack(track2); err != nil {
		t.Fatalf("Failed to insert track2: %v", err)
	}

	// Compare runs
	comparison, err := CompareRuns(store, run1.RunID, run2.RunID)
	if err != nil {
		t.Fatalf("CompareRuns failed: %v", err)
	}

	if len(comparison.MatchedTracks) != 0 {
		t.Errorf("Expected no matched tracks due to no overlap, got %d", len(comparison.MatchedTracks))
	}
	if len(comparison.TracksOnlyRun1) != 1 {
		t.Errorf("Expected 1 track only in run1")
	}
	if len(comparison.TracksOnlyRun2) != 1 {
		t.Errorf("Expected 1 track only in run2")
	}
}

func TestCompareRuns_SplitDetection(t *testing.T) {
	// Test split detection: one run1 track matched to multiple run2 tracks
	// Note: Current implementation uses 1:1 Hungarian matching, so splits are not yet detected
	// This test documents the future expected behaviour
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Create two runs
	run1 := &AnalysisRun{
		RunID:      "split_run_1",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor_1",
		Status:     "completed",
	}
	run2 := &AnalysisRun{
		RunID:      "split_run_2",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor_1",
		Status:     "completed",
	}

	if err := store.InsertRun(run1); err != nil {
		t.Fatalf("Failed to insert run1: %v", err)
	}
	if err := store.InsertRun(run2); err != nil {
		t.Fatalf("Failed to insert run2: %v", err)
	}

	// Create one track in run1 and two overlapping tracks in run2
	now := time.Now().UnixNano()
	track1 := &RunTrack{
		RunID:            run1.RunID,
		TrackID:          "track_original",
		SensorID:         "sensor_1",
		TrackState:       "confirmed",
		StartUnixNanos:   now,
		EndUnixNanos:     now + int64(20*time.Second),
		ObservationCount: 200,
	}
	track2a := &RunTrack{
		RunID:            run2.RunID,
		TrackID:          "track_split_a",
		SensorID:         "sensor_1",
		TrackState:       "confirmed",
		StartUnixNanos:   now,
		EndUnixNanos:     now + int64(10*time.Second),
		ObservationCount: 100,
	}
	track2b := &RunTrack{
		RunID:            run2.RunID,
		TrackID:          "track_split_b",
		SensorID:         "sensor_1",
		TrackState:       "confirmed",
		StartUnixNanos:   now + int64(10*time.Second),
		EndUnixNanos:     now + int64(20*time.Second),
		ObservationCount: 100,
	}

	if err := store.InsertRunTrack(track1); err != nil {
		t.Fatalf("Failed to insert track1: %v", err)
	}
	if err := store.InsertRunTrack(track2a); err != nil {
		t.Fatalf("Failed to insert track2a: %v", err)
	}
	if err := store.InsertRunTrack(track2b); err != nil {
		t.Fatalf("Failed to insert track2b: %v", err)
	}

	// Compare runs
	comparison, err := CompareRuns(store, run1.RunID, run2.RunID)
	if err != nil {
		t.Fatalf("CompareRuns failed: %v", err)
	}

	// With Hungarian matching, only one of the split tracks will be matched
	// The other will appear as unmatched
	if len(comparison.MatchedTracks) != 1 {
		t.Logf("Expected 1 matched track (Hungarian gives 1:1 matching), got %d", len(comparison.MatchedTracks))
	}

	// Future enhancement: split detection would find that track_original overlaps with both track_split_a and track_split_b
	// For now, we just verify the comparison completes without error
}

func TestCompareParams_NoDifference(t *testing.T) {
	params := DefaultRunParams()

	diff := compareParams(&params, &params)

	if len(diff) != 0 {
		t.Errorf("Expected no differences, got %v", diff)
	}
}

func TestCompareParams_BackgroundDifference(t *testing.T) {
	params1 := DefaultRunParams()
	params2 := DefaultRunParams()
	params2.Background.BackgroundUpdateFraction = 0.05

	diff := compareParams(&params1, &params2)

	if len(diff) == 0 {
		t.Fatalf("Expected differences, got none")
	}

	bgDiff, ok := diff["background"].(map[string]any)
	if !ok {
		t.Fatalf("Expected background diff")
	}

	updateDiff, ok := bgDiff["background_update_fraction"].(map[string]any)
	if !ok {
		t.Fatalf("Expected background_update_fraction diff")
	}

	if updateDiff["run1"] != params1.Background.BackgroundUpdateFraction {
		t.Errorf("run1 value mismatch")
	}
	if updateDiff["run2"] != params2.Background.BackgroundUpdateFraction {
		t.Errorf("run2 value mismatch")
	}
}
