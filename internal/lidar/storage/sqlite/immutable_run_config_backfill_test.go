package sqlite

import (
	"encoding/json"
	"testing"
	"time"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

func TestBackfillImmutableRunConfigReferences(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	legacyParams := RunParams{
		Version:   "1.0",
		Timestamp: time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC),
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction:       0.1,
			ClosenessSensitivityMultiplier: 1.2,
			SafetyMarginMeters:             0.3,
			NeighborConfirmationCount:      4,
			NoiseRelativeFraction:          0.02,
			SeedFromFirstObservation:       true,
			FreezeDurationNanos:            5e9,
		},
		Clustering: ClusteringParamsExport{
			Eps:      0.7,
			MinPts:   5,
			CellSize: 0.7,
		},
		Tracking: TrackingParamsExport{
			MaxTracks:             128,
			MaxMisses:             4,
			HitsToConfirm:         3,
			GatingDistanceSquared: 9,
			ProcessNoisePos:       0.2,
			ProcessNoiseVel:       0.3,
			MeasurementNoise:      0.4,
		},
		Classification: ClassificationParamsExport{
			ModelType: "rule_based",
		},
	}
	legacyParamsJSON, err := legacyParams.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	if _, err := testDB.Exec(`
		INSERT INTO lidar_run_records (
			run_id, created_at, source_type, sensor_id, params_json,
			duration_secs, total_frames, total_clusters, total_tracks, confirmed_tracks,
			processing_time_ms, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"legacy-run",
		time.Now().UnixNano(),
		"pcap",
		"sensor-1",
		string(legacyParamsJSON),
		12.5,
		100,
		12,
		4,
		3,
		250,
		"completed",
	); err != nil {
		t.Fatalf("insert legacy run: %v", err)
	}

	if _, err := testDB.Exec(`
		INSERT INTO lidar_replay_cases (
			replay_case_id, sensor_id, pcap_file, optimal_params_json, created_at_ns
		) VALUES (?, ?, ?, ?, ?)
	`, "case-1", "sensor-1", "case-1.pcap", `{"tracking":{"max_tracks":64}}`, time.Now().UnixNano()); err != nil {
		t.Fatalf("insert replay case: %v", err)
	}

	result, err := BackfillImmutableRunConfigReferences(testDB.DB, false)
	if err != nil {
		t.Fatalf("BackfillImmutableRunConfigReferences() error = %v", err)
	}

	if result.RunsUpdated != 1 {
		t.Fatalf("expected 1 run update, got %+v", result)
	}
	if result.ReplayCasesUpdated != 1 {
		t.Fatalf("expected 1 replay-case update, got %+v", result)
	}

	runStore := NewAnalysisRunStore(testDB.DB)
	run, err := runStore.GetRun("legacy-run")
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.RunConfigID == "" {
		t.Fatal("expected run_config_id to be backfilled")
	}
	if run.ParamSetType != "legacy" {
		t.Fatalf("expected legacy param_set_type, got %q", run.ParamSetType)
	}
	if run.ParamsHash == "" || run.ConfigHash == "" {
		t.Fatalf("expected hashes to be hydrated, got params_hash=%q config_hash=%q", run.ParamsHash, run.ConfigHash)
	}
	if run.BuildVersion != "unknown" || run.BuildGitSHA != "unknown" {
		t.Fatalf("expected unknown build identity, got version=%q sha=%q", run.BuildVersion, run.BuildGitSHA)
	}
	if len(run.ExecutionConfig) == 0 {
		t.Fatal("expected execution_config to be hydrated for backfilled run")
	}

	sceneStore := NewReplayCaseStore(testDB.DB)
	scene, err := sceneStore.GetScene("case-1")
	if err != nil {
		t.Fatalf("GetScene() error = %v", err)
	}
	if scene.RecommendedParamSetID == "" {
		t.Fatal("expected recommended_param_set_id to be backfilled")
	}
	if scene.RecommendedParamsHash == "" {
		t.Fatal("expected recommended params hash to be hydrated")
	}

	var recommended map[string]any
	if err := json.Unmarshal(scene.RecommendedParams, &recommended); err != nil {
		t.Fatalf("unmarshal recommended params: %v", err)
	}
	if _, ok := recommended["tracking"]; !ok {
		t.Fatalf("expected recommended params payload to include tracking section, got %v", recommended)
	}
}
