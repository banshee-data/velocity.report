package configasset

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

func TestMakeRequestedParamSet_CanonicalizesJSON(t *testing.T) {
	paramSet, err := MakeRequestedParamSet(json.RawMessage(`{"z":1,"m":[3,2,1],"a":{"d":4,"c":3}}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet failed: %v", err)
	}

	const want = `{"schema_version":"requested/v1","param_set_type":"requested","params":{"a":{"c":3,"d":4},"m":[3,2,1],"z":1}}`
	if got := string(paramSet.ParamsJSON); got != want {
		t.Fatalf("canonical params JSON mismatch:\n got: %s\nwant: %s", got, want)
	}
	if paramSet.ParamSetType != ParamSetTypeRequested {
		t.Fatalf("expected param set type %q, got %q", ParamSetTypeRequested, paramSet.ParamSetType)
	}
	if paramSet.SchemaVersion != SchemaVersionRequestedV1 {
		t.Fatalf("expected schema version %q, got %q", SchemaVersionRequestedV1, paramSet.SchemaVersion)
	}
	if got, want := paramSet.ParamsHash, HashJSON([]byte(want)); got != want {
		t.Fatalf("expected params hash %q, got %q", want, got)
	}
}

func TestComposeRunConfig_StableAndNormalized(t *testing.T) {
	paramSet, err := MakeRequestedParamSet(json.RawMessage(`{"z":1,"m":[3,2,1],"a":{"d":4,"c":3}}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet failed: %v", err)
	}

	build := BuildIdentity{BuildVersion: "  ", BuildGitSHA: ""}
	composedA, err := ComposeRunConfig(paramSet, build)
	if err != nil {
		t.Fatalf("ComposeRunConfig(first) failed: %v", err)
	}
	composedB, err := ComposeRunConfig(paramSet, build)
	if err != nil {
		t.Fatalf("ComposeRunConfig(second) failed: %v", err)
	}

	const want = `{"schema_version":"run_config/v1","param_set_type":"requested","build":{"build_version":"unknown","build_git_sha":"unknown"},"params":{"a":{"c":3,"d":4},"m":[3,2,1],"z":1}}`
	if got := string(composedA); got != want {
		t.Fatalf("composed run config JSON mismatch:\n got: %s\nwant: %s", got, want)
	}
	if string(composedA) != string(composedB) {
		t.Fatalf("expected stable composed JSON across calls, got %s vs %s", composedA, composedB)
	}
}

func TestStoreFindByHash_NotFound(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewStore(testDB)

	if _, err := store.findParamSetByHash("sha256:missing-param-set"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for missing param set, got %v", err)
	}
	if _, err := store.findRunConfigByHash("sha256:missing-run-config"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for missing run config, got %v", err)
	}
}

func TestEnsureParamSet_InsertsOnMissThenDeduplicates(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewStore(testDB)
	paramSetA, err := MakeRequestedParamSet(json.RawMessage(`{"beta":2,"alpha":1}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet(first) failed: %v", err)
	}
	paramSetB, err := MakeRequestedParamSet(json.RawMessage(`{"alpha":1,"beta":2}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet(second) failed: %v", err)
	}
	paramSetB.ParamSetID = "should-not-be-used"
	paramSetB.CreatedAt = 123

	storedA, err := store.EnsureParamSet(paramSetA)
	if err != nil {
		t.Fatalf("EnsureParamSet(first) failed: %v", err)
	}
	storedB, err := store.EnsureParamSet(paramSetB)
	if err != nil {
		t.Fatalf("EnsureParamSet(second) failed: %v", err)
	}

	if storedA.ParamSetID == "" {
		t.Fatalf("expected generated param_set_id on insert")
	}
	if storedA.CreatedAt == 0 {
		t.Fatalf("expected created_at to be set on insert")
	}
	if storedA.ParamSetID != storedB.ParamSetID {
		t.Fatalf("expected hash dedupe to reuse param_set_id %q, got %q", storedA.ParamSetID, storedB.ParamSetID)
	}
	if storedA.ParamsHash != storedB.ParamsHash {
		t.Fatalf("expected identical params_hash across deduped inserts, got %q vs %q", storedA.ParamsHash, storedB.ParamsHash)
	}

	var count int
	if err := testDB.QueryRow(`SELECT COUNT(*) FROM lidar_param_sets`).Scan(&count); err != nil {
		t.Fatalf("count lidar_param_sets: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 param set row after dedupe, got %d", count)
	}
}

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

	composedWant, err := ComposeRunConfig(paramSet, buildA)
	if err != nil {
		t.Fatalf("ComposeRunConfig(buildA) failed: %v", err)
	}
	if got := string(runConfigA1.ComposedJSON); got != string(composedWant) {
		t.Fatalf("expected stored composed JSON to match canonical composition:\n got: %s\nwant: %s", got, composedWant)
	}
}
