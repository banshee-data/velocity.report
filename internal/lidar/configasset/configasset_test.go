package configasset

import (
	"testing"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

func TestEnsureRunConfig_DeduplicatesByEffectiveParamsAndBuild(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewStore(testDB)
	cfg := cfgpkg.MustLoadDefaultConfig()
	paramSet, err := MakeEffectiveParamSet(cfg)
	if err != nil {
		t.Fatalf("MakeEffectiveParamSet failed: %v", err)
	}

	buildA := BuildIdentity{BuildVersion: "0.5.0-test", BuildGitSHA: "abc123"}
	buildB := BuildIdentity{BuildVersion: "0.5.0-test", BuildGitSHA: "def456"}

	runConfigA1, err := store.EnsureRunConfig(paramSet, buildA)
	if err != nil {
		t.Fatalf("EnsureRunConfig(buildA #1) failed: %v", err)
	}
	runConfigA2, err := store.EnsureRunConfig(paramSet, buildA)
	if err != nil {
		t.Fatalf("EnsureRunConfig(buildA #2) failed: %v", err)
	}
	runConfigB, err := store.EnsureRunConfig(paramSet, buildB)
	if err != nil {
		t.Fatalf("EnsureRunConfig(buildB) failed: %v", err)
	}

	if runConfigA1.RunConfigID != runConfigA2.RunConfigID {
		t.Fatalf("expected identical run_config_id for same build, got %q and %q", runConfigA1.RunConfigID, runConfigA2.RunConfigID)
	}
	if runConfigA1.ConfigHash != runConfigA2.ConfigHash {
		t.Fatalf("expected identical config_hash for same build, got %q and %q", runConfigA1.ConfigHash, runConfigA2.ConfigHash)
	}
	if runConfigA1.ConfigHash == runConfigB.ConfigHash {
		t.Fatalf("expected config_hash to change when build identity changes, got %q", runConfigA1.ConfigHash)
	}

	var paramSetCount int
	if err := testDB.QueryRow(`SELECT COUNT(*) FROM lidar_param_sets`).Scan(&paramSetCount); err != nil {
		t.Fatalf("count lidar_param_sets: %v", err)
	}
	if paramSetCount != 1 {
		t.Fatalf("expected 1 effective param set row, got %d", paramSetCount)
	}

	var runConfigCount int
	if err := testDB.QueryRow(`SELECT COUNT(*) FROM lidar_run_configs`).Scan(&runConfigCount); err != nil {
		t.Fatalf("count lidar_run_configs: %v", err)
	}
	if runConfigCount != 2 {
		t.Fatalf("expected 2 run config rows after build divergence, got %d", runConfigCount)
	}
}
