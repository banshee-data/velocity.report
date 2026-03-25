package configasset

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

type execInterceptDB struct {
	db         *sql.DB
	execErr    error
	shouldFail func(query string, args []any) bool
}

func (d *execInterceptDB) Exec(query string, args ...any) (sql.Result, error) {
	if d.shouldFail != nil && d.shouldFail(query, args) {
		return nil, d.execErr
	}
	return d.db.Exec(query, args...)
}

func (d *execInterceptDB) QueryRow(query string, args ...any) *sql.Row {
	return d.db.QueryRow(query, args...)
}

type statefulQueryRowDB struct {
	db                *sql.DB
	runConfigQueryDB  *sql.DB
	runConfigQueryCnt int
}

func (d *statefulQueryRowDB) Exec(query string, args ...any) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

func (d *statefulQueryRowDB) QueryRow(query string, args ...any) *sql.Row {
	if strings.Contains(query, "FROM lidar_run_configs") {
		d.runConfigQueryCnt++
		if d.runConfigQueryCnt >= 2 && d.runConfigQueryDB != nil {
			return d.runConfigQueryDB.QueryRow(query, args...)
		}
	}
	return d.db.QueryRow(query, args...)
}

type runConfigQueryInterceptDB struct {
	db          *sql.DB
	runConfigDB *sql.DB
	paramSetDB  *sql.DB
}

func (d *runConfigQueryInterceptDB) Exec(query string, args ...any) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

func (d *runConfigQueryInterceptDB) QueryRow(query string, args ...any) *sql.Row {
	if strings.Contains(query, "FROM lidar_run_configs") && d.runConfigDB != nil {
		return d.runConfigDB.QueryRow(query, args...)
	}
	if strings.Contains(query, "FROM lidar_param_sets") && d.paramSetDB != nil {
		return d.paramSetDB.QueryRow(query, args...)
	}
	return d.db.QueryRow(query, args...)
}

func TestReadBuildIdentity_NormalizesValues(t *testing.T) {
	build := ReadBuildIdentity()
	if strings.TrimSpace(build.BuildVersion) == "" {
		t.Fatal("expected normalized build version")
	}
	if strings.TrimSpace(build.BuildGitSHA) == "" {
		t.Fatal("expected normalized build git sha")
	}
}

func TestMakeEffectiveParamSet_NilConfig(t *testing.T) {
	if _, err := MakeEffectiveParamSet(nil); err == nil || !strings.Contains(err.Error(), "effective tuning config is required") {
		t.Fatalf("expected nil-config error, got %v", err)
	}
}

func TestMakeLegacyParamSet_AndExtractParamsPayload(t *testing.T) {
	paramSet, err := MakeLegacyParamSet(json.RawMessage(`"{\"beta\":2,\"alpha\":1}"`))
	if err != nil {
		t.Fatalf("MakeLegacyParamSet failed: %v", err)
	}
	if paramSet.SchemaVersion != SchemaVersionLegacyV1 {
		t.Fatalf("schema version = %q, want %q", paramSet.SchemaVersion, SchemaVersionLegacyV1)
	}
	if paramSet.ParamSetType != ParamSetTypeLegacy {
		t.Fatalf("param set type = %q, want %q", paramSet.ParamSetType, ParamSetTypeLegacy)
	}

	payload, err := ExtractParamsPayload(paramSet.ParamsJSON)
	if err != nil {
		t.Fatalf("ExtractParamsPayload failed: %v", err)
	}
	if got := string(payload); got != `{"alpha":1,"beta":2}` {
		t.Fatalf("payload = %s, want canonical object", got)
	}
}

func TestComposeRunConfig_ErrorBranches(t *testing.T) {
	if _, err := ComposeRunConfig(nil, BuildIdentity{}); err == nil || !strings.Contains(err.Error(), "param set is required") {
		t.Fatalf("expected nil param-set error, got %v", err)
	}

	if _, err := ComposeRunConfig(&ParamSet{}, BuildIdentity{}); err == nil || !strings.Contains(err.Error(), "param set JSON is required") {
		t.Fatalf("expected missing params-json error, got %v", err)
	}

	if _, err := ComposeRunConfig(&ParamSet{ParamsJSON: []byte(`{not json}`)}, BuildIdentity{}); err == nil || !strings.Contains(err.Error(), "decode param set JSON") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestStoreEnsureParamSet_ErrorBranches(t *testing.T) {
	store := &Store{}
	if _, err := store.EnsureParamSet(&ParamSet{}); err == nil || !strings.Contains(err.Error(), "requires a database") {
		t.Fatalf("expected missing-db error, got %v", err)
	}

	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store = NewStore(testDB.DB)
	if _, err := store.EnsureParamSet(nil); err == nil || !strings.Contains(err.Error(), "param set is required") {
		t.Fatalf("expected nil-param-set error, got %v", err)
	}

	if err := testDB.DB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	paramSet, err := MakeRequestedParamSet(json.RawMessage(`{"alpha":1}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet failed: %v", err)
	}
	if _, err := store.EnsureParamSet(paramSet); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("expected query error from closed db, got %v", err)
	}
}

func TestStoreEnsureParamSet_InsertError(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	paramSet, err := MakeRequestedParamSet(json.RawMessage(`{"alpha":1}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet failed: %v", err)
	}

	store := NewStore(&execInterceptDB{
		db:      testDB.DB,
		execErr: errors.New("boom"),
		shouldFail: func(query string, _ []any) bool {
			return strings.Contains(query, "INSERT OR IGNORE INTO lidar_param_sets")
		},
	})
	if _, err := store.EnsureParamSet(paramSet); err == nil || !strings.Contains(err.Error(), "insert lidar_param_sets") {
		t.Fatalf("expected insert error, got %v", err)
	}
}

func TestStoreEnsureRunConfig_ErrorBranchesAndGetters(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	cfg := cfgpkg.MustLoadDefaultConfig()
	paramSet, err := MakeEffectiveParamSet(cfg)
	if err != nil {
		t.Fatalf("MakeEffectiveParamSet failed: %v", err)
	}

	store := &Store{}
	if _, err := store.EnsureRunConfig(paramSet, BuildIdentity{}); err == nil || !strings.Contains(err.Error(), "requires a database") {
		t.Fatalf("expected missing-db error, got %v", err)
	}
	if _, err := store.GetParamSet("missing"); err == nil || !strings.Contains(err.Error(), "requires a database") {
		t.Fatalf("expected missing-db GetParamSet error, got %v", err)
	}
	if _, err := store.GetRunConfig("missing"); err == nil || !strings.Contains(err.Error(), "requires a database") {
		t.Fatalf("expected missing-db GetRunConfig error, got %v", err)
	}

	runConfigStore := NewStore(&execInterceptDB{
		db:      testDB.DB,
		execErr: errors.New("boom"),
		shouldFail: func(query string, _ []any) bool {
			return strings.Contains(query, "INSERT OR IGNORE INTO lidar_run_configs")
		},
	})
	if _, err := runConfigStore.EnsureRunConfig(paramSet, BuildIdentity{}); err == nil || !strings.Contains(err.Error(), "insert lidar_run_configs") {
		t.Fatalf("expected run-config insert error, got %v", err)
	}

	successStore := NewStore(testDB.DB)
	runConfig, err := successStore.EnsureRunConfig(paramSet, BuildIdentity{BuildVersion: "v1", BuildGitSHA: "sha1"})
	if err != nil {
		t.Fatalf("EnsureRunConfig success failed: %v", err)
	}

	gotParamSet, err := successStore.GetParamSet(runConfig.ParamSetID)
	if err != nil {
		t.Fatalf("GetParamSet failed: %v", err)
	}
	if gotParamSet.ParamSetID != runConfig.ParamSetID {
		t.Fatalf("GetParamSet returned wrong id: %q", gotParamSet.ParamSetID)
	}

	gotRunConfig, err := successStore.GetRunConfig(runConfig.RunConfigID)
	if err != nil {
		t.Fatalf("GetRunConfig failed: %v", err)
	}
	if gotRunConfig.RunConfigID != runConfig.RunConfigID {
		t.Fatalf("GetRunConfig returned wrong id: %q", gotRunConfig.RunConfigID)
	}
	if len(gotRunConfig.ComposedJSON) == 0 {
		t.Fatal("expected composed JSON from GetRunConfig")
	}

	if _, err := successStore.GetParamSet("missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for missing param set, got %v", err)
	}
	if _, err := successStore.GetRunConfig("missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for missing run config, got %v", err)
	}
}

func TestStoreEnsureRunConfig_SecondaryLookupFailures(t *testing.T) {
	openDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	closedDB, closedCleanup := dbpkg.NewTestDB(t)
	defer closedCleanup()
	if err := closedDB.DB.Close(); err != nil {
		t.Fatalf("close secondary db: %v", err)
	}

	cfg := cfgpkg.MustLoadDefaultConfig()
	paramSet, err := MakeEffectiveParamSet(cfg)
	if err != nil {
		t.Fatalf("MakeEffectiveParamSet failed: %v", err)
	}

	store := NewStore(&statefulQueryRowDB{
		db:               openDB.DB,
		runConfigQueryDB: closedDB.DB,
	})
	if _, err := store.EnsureRunConfig(paramSet, BuildIdentity{BuildVersion: "v1", BuildGitSHA: "sha"}); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("expected secondary run-config lookup failure, got %v", err)
	}
}

func TestStoreEnsureRunConfig_PrimaryLookupFailure(t *testing.T) {
	openDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	closedDB, closedCleanup := dbpkg.NewTestDB(t)
	defer closedCleanup()
	if err := closedDB.DB.Close(); err != nil {
		t.Fatalf("close secondary db: %v", err)
	}

	cfg := cfgpkg.MustLoadDefaultConfig()
	paramSet, err := MakeEffectiveParamSet(cfg)
	if err != nil {
		t.Fatalf("MakeEffectiveParamSet failed: %v", err)
	}

	store := NewStore(&runConfigQueryInterceptDB{
		db:          openDB.DB,
		runConfigDB: closedDB.DB,
	})
	if _, err := store.EnsureRunConfig(paramSet, BuildIdentity{BuildVersion: "v1", BuildGitSHA: "sha"}); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("expected primary run-config lookup failure, got %v", err)
	}
}

func TestStoreEnsureRunConfig_ComposeAndEnsureParamSetFailures(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewStore(testDB.DB)
	if _, err := store.EnsureRunConfig(&ParamSet{
		ParamSetID:    "broken",
		ParamsHash:    "sha256:broken",
		SchemaVersion: SchemaVersionRequestedV1,
		ParamSetType:  ParamSetTypeRequested,
		ParamsJSON:    []byte(`{not json}`),
	}, BuildIdentity{}); err == nil || !strings.Contains(err.Error(), "decode param set JSON") {
		t.Fatalf("expected ComposeRunConfig failure, got %v", err)
	}

	closedDB, closedCleanup := dbpkg.NewTestDB(t)
	defer closedCleanup()
	if err := closedDB.DB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	closedStore := NewStore(closedDB.DB)
	paramSet, err := MakeRequestedParamSet(json.RawMessage(`{"alpha":1}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet failed: %v", err)
	}
	if _, err := closedStore.EnsureRunConfig(paramSet, BuildIdentity{}); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("expected EnsureParamSet failure on closed db, got %v", err)
	}
}

func TestGetRunConfig_ComposeError(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	if _, err := testDB.Exec(`
		INSERT INTO lidar_param_sets (
			param_set_id, params_hash, schema_version, param_set_type, params_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, "broken-param-set", "sha256:broken", SchemaVersionRequestedV1, ParamSetTypeRequested, `{"schema_version":"requested/v1","param_set_type":"requested","params":`, 1); err != nil {
		t.Fatalf("insert broken param set: %v", err)
	}
	if _, err := testDB.Exec(`
		INSERT INTO lidar_run_configs (
			run_config_id, config_hash, param_set_id, build_version, build_git_sha, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, "broken-run-config", "sha256:run", "broken-param-set", "v1", "sha", 1); err != nil {
		t.Fatalf("insert broken run config: %v", err)
	}

	store := NewStore(testDB.DB)
	if _, err := store.GetRunConfig("broken-run-config"); err == nil || !strings.Contains(err.Error(), "decode param set JSON") {
		t.Fatalf("expected compose error from broken param set JSON, got %v", err)
	}
}

func TestExtractParamsPayload_ErrorBranches(t *testing.T) {
	if _, err := ExtractParamsPayload(nil); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required error, got %v", err)
	}
	if _, err := ExtractParamsPayload([]byte(`{not json}`)); err == nil || !strings.Contains(err.Error(), "decode param set JSON") {
		t.Fatalf("expected decode error, got %v", err)
	}
	if _, err := ExtractParamsPayload([]byte(`{"schema_version":"requested/v1","param_set_type":"requested"}`)); err == nil || !strings.Contains(err.Error(), "payload is empty") {
		t.Fatalf("expected empty payload error, got %v", err)
	}
}

func TestMakeWrappedRawParamSet_ErrorBranches(t *testing.T) {
	if _, err := MakeRequestedParamSet(nil); err == nil || !strings.Contains(err.Error(), "param set JSON is required") {
		t.Fatalf("expected required raw JSON error, got %v", err)
	}
	if _, err := MakeLegacyParamSet(json.RawMessage(`[]`)); err == nil || !strings.Contains(err.Error(), "must decode to an object") {
		t.Fatalf("expected decode-object error, got %v", err)
	}
}

func TestDecodeJSONObject_ErrorBranches(t *testing.T) {
	if _, err := decodeJSONObject(json.RawMessage(`{not json}`)); err == nil || !strings.Contains(err.Error(), "decode param set JSON") {
		t.Fatalf("expected decode error, got %v", err)
	}
	if _, err := decodeJSONObject(json.RawMessage(`"   "`)); err == nil || !strings.Contains(err.Error(), "string is empty") {
		t.Fatalf("expected empty stringified JSON error, got %v", err)
	}
	if _, err := decodeJSONObject(json.RawMessage(`"{not json}"`)); err == nil || !strings.Contains(err.Error(), "decode stringified param set JSON") {
		t.Fatalf("expected stringified decode error, got %v", err)
	}
	if _, err := decodeJSONObject(json.RawMessage(`["not","an","object"]`)); err == nil || !strings.Contains(err.Error(), "must decode to an object") {
		t.Fatalf("expected non-object error, got %v", err)
	}
}

func TestMakeParamSet_MarshalError(t *testing.T) {
	if _, err := makeParamSet(paramSetEnvelope{
		SchemaVersion: SchemaVersionRequestedV1,
		ParamSetType:  ParamSetTypeRequested,
		Params:        map[string]any{"bad": make(chan int)},
	}); err == nil || !strings.Contains(err.Error(), "marshal param set JSON") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}
