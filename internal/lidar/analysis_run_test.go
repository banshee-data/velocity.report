package lidar

import (
	"encoding/json"
	"testing"
	"time"
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

	// Check background defaults
	if params.Background.BackgroundUpdateFraction != 0.02 {
		t.Errorf("BackgroundUpdateFraction should be 0.02")
	}
	if params.Background.ClosenessSensitivityMultiplier != 3.0 {
		t.Errorf("ClosenessSensitivityMultiplier should be 3.0")
	}

	// Check clustering defaults match DBSCAN defaults
	if params.Clustering.Eps != DefaultDBSCANEps {
		t.Errorf("Eps should match DefaultDBSCANEps")
	}
	if params.Clustering.MinPts != DefaultDBSCANMinPts {
		t.Errorf("MinPts should match DefaultDBSCANMinPts")
	}

	// Check tracking defaults
	if params.Tracking.MaxTracks != 100 {
		t.Errorf("MaxTracks should be 100")
	}
	if params.Tracking.MaxMisses != 3 {
		t.Errorf("MaxMisses should be 3")
	}
	if params.Tracking.HitsToConfirm != 5 {
		t.Errorf("HitsToConfirm should be 5")
	}

	// Check classification defaults
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
