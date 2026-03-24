package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/configasset"
)

type conditionalExecSQLiteDB struct {
	db         *sql.DB
	execErr    error
	shouldFail func(query string, args []any) bool
}

func (d *conditionalExecSQLiteDB) Exec(query string, args ...any) (sql.Result, error) {
	if d.shouldFail != nil && d.shouldFail(query, args) {
		return nil, d.execErr
	}
	return d.db.Exec(query, args...)
}

func (d *conditionalExecSQLiteDB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

func (d *conditionalExecSQLiteDB) QueryRow(query string, args ...any) *sql.Row {
	return d.db.QueryRow(query, args...)
}

func (d *conditionalExecSQLiteDB) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

type interceptSQLiteDB struct {
	db         *sql.DB
	execFn     func(query string, args []any) (sql.Result, error)
	queryFn    func(query string, args []any) (*sql.Rows, error)
	queryRowFn func(query string, args []any) *sql.Row
}

func (d *interceptSQLiteDB) Exec(query string, args ...any) (sql.Result, error) {
	if d.execFn != nil {
		return d.execFn(query, args)
	}
	return d.db.Exec(query, args...)
}

func (d *interceptSQLiteDB) Query(query string, args ...any) (*sql.Rows, error) {
	if d.queryFn != nil {
		return d.queryFn(query, args)
	}
	return d.db.Query(query, args...)
}

func (d *interceptSQLiteDB) QueryRow(query string, args ...any) *sql.Row {
	if d.queryRowFn != nil {
		return d.queryRowFn(query, args)
	}
	return d.db.QueryRow(query, args...)
}

func (d *interceptSQLiteDB) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

type rowsAffectedErrorResult struct {
	rows int64
	err  error
}

func (r rowsAffectedErrorResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r rowsAffectedErrorResult) RowsAffected() (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.rows, nil
}

type scannerStub struct {
	values []any
	err    error
}

func (s scannerStub) Scan(dest ...any) error {
	if s.err != nil {
		return s.err
	}
	if len(dest) != len(s.values) {
		return errors.New("destination length mismatch")
	}
	for i := range dest {
		if err := assignScannedValue(dest[i], s.values[i]); err != nil {
			return err
		}
	}
	return nil
}

type rowsStub struct {
	values  [][]any
	scanErr error
	iterErr error
	index   int
}

func (r *rowsStub) Next() bool {
	if r.index >= len(r.values) {
		return false
	}
	r.index++
	return true
}

func (r *rowsStub) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if r.index == 0 || r.index > len(r.values) {
		return errors.New("scan called without current row")
	}
	row := r.values[r.index-1]
	if len(dest) != len(row) {
		return errors.New("destination length mismatch")
	}
	for i := range dest {
		if err := assignScannedValue(dest[i], row[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *rowsStub) Err() error {
	return r.iterErr
}

func assignScannedValue(dest any, value any) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("destination must be a non-nil pointer")
	}
	target := rv.Elem()
	if value == nil {
		target.Set(reflect.Zero(target.Type()))
		return nil
	}
	val := reflect.ValueOf(value)
	if val.Type().AssignableTo(target.Type()) {
		target.Set(val)
		return nil
	}
	if val.Type().ConvertibleTo(target.Type()) {
		target.Set(val.Convert(target.Type()))
		return nil
	}
	return errors.New("incompatible scan assignment")
}

func setupReplayCaseRecommendedDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open recommended replay-case db: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE lidar_run_records (
			run_id TEXT PRIMARY KEY,
			created_at INTEGER,
			run_config_id TEXT
		)
	`); err != nil {
		t.Fatalf("create lidar_run_records: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE lidar_replay_cases (
			replay_case_id TEXT PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			pcap_file TEXT NOT NULL,
			pcap_start_secs REAL,
			pcap_duration_secs REAL,
			description TEXT,
			reference_run_id TEXT,
			created_at_ns INTEGER NOT NULL,
			updated_at_ns INTEGER,
			recommended_param_set_id TEXT
		)
	`); err != nil {
		t.Fatalf("create lidar_replay_cases: %v", err)
	}

	return db
}

func TestRunLabelRollupAndNormalisers(t *testing.T) {
	var nilRollup *RunLabelRollup
	if got := nilRollup.LabelledCount(); got != 0 {
		t.Fatalf("nil LabelledCount = %d, want 0", got)
	}

	if got := normaliseRunTrackQualityLabel(" good, , truncated , "); got != "good,truncated" {
		t.Fatalf("normaliseRunTrackQualityLabel = %q", got)
	}
	if got := normaliseRunTrackLinkedIDs([]string{" a ", "", "b "}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("normaliseRunTrackLinkedIDs = %#v", got)
	}
	if got := normaliseRunTrackLinkedIDs([]string{" ", "\t"}); got != nil {
		t.Fatalf("expected nil linked ids, got %#v", got)
	}
}

func TestStartRunAndPreparedRun_ErrorAndDefaultPaths(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "manager-sensor")

	runID, err := manager.startPreparedRun(&AnalysisRun{})
	if err != nil {
		t.Fatalf("startPreparedRun failed: %v", err)
	}
	run, err := manager.store.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if run.SourceType != "pcap" {
		t.Fatalf("SourceType = %q, want pcap", run.SourceType)
	}
	if run.SensorID != "manager-sensor" {
		t.Fatalf("SensorID = %q, want manager-sensor", run.SensorID)
	}
	if run.Status != "running" {
		t.Fatalf("Status = %q, want running", run.Status)
	}
}

func TestStartRunWithConfig_ErrorBranches(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "cfg-sensor")
	if _, err := manager.StartRunWithConfig(AnalysisRunStartOptions{}); err == nil || !strings.Contains(err.Error(), "effective config is required") {
		t.Fatalf("expected missing effective config error, got %v", err)
	}

	makeEffectiveFailCfg := cfgpkg.MustLoadDefaultConfig()
	makeEffectiveFailCfg.L3.EmaTrackAssistV2 = &cfgpkg.L3EmaTrackAssistV2{
		L3Common:           makeEffectiveFailCfg.L3.EmaBaselineV1.L3Common,
		PromotionThreshold: math.NaN(),
	}
	if _, err := manager.StartRunWithConfig(AnalysisRunStartOptions{
		SourcePath:      "/tmp/effective-fail.pcap",
		EffectiveConfig: makeEffectiveFailCfg,
	}); err == nil {
		t.Fatal("expected StartRunWithConfig effective param-set marshal error")
	}
}

func TestStartRunWithConfig_ConfigAssetErrors(t *testing.T) {
	baseDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	cfg := cfgpkg.MustLoadDefaultConfig()

	runConfigFailDB := &conditionalExecSQLiteDB{
		db:      baseDB.DB,
		execErr: errors.New("boom"),
		shouldFail: func(query string, _ []any) bool {
			return strings.Contains(query, "INSERT OR IGNORE INTO lidar_run_configs")
		},
	}
	manager := NewAnalysisRunManager(runConfigFailDB, "sensor-a")
	if _, err := manager.StartRunWithConfig(AnalysisRunStartOptions{
		SourcePath:      "/tmp/run-config-fail.pcap",
		EffectiveConfig: cfg,
	}); err == nil || !strings.Contains(err.Error(), "insert lidar_run_configs") {
		t.Fatalf("expected run config insert error, got %v", err)
	}

	requestedFailDB := &conditionalExecSQLiteDB{
		db:      baseDB.DB,
		execErr: errors.New("boom"),
		shouldFail: func(query string, args []any) bool {
			return strings.Contains(query, "INSERT OR IGNORE INTO lidar_param_sets") &&
				len(args) >= 3 &&
				args[2] == configasset.SchemaVersionRequestedV1
		},
	}
	manager = NewAnalysisRunManager(requestedFailDB, "sensor-b")
	if _, err := manager.StartRunWithConfig(AnalysisRunStartOptions{
		SourcePath:          "/tmp/requested-fail.pcap",
		RequestedParamsJSON: json.RawMessage(`{"tracking":{"max_tracks":32}}`),
		EffectiveConfig:     cfg,
	}); err == nil || !strings.Contains(err.Error(), "insert lidar_param_sets") {
		t.Fatalf("expected requested param set insert error, got %v", err)
	}

	manager = NewAnalysisRunManager(baseDB.DB, "sensor-c")
	if _, err := manager.StartRunWithConfig(AnalysisRunStartOptions{
		SourcePath:          "/tmp/requested-invalid.pcap",
		RequestedParamsJSON: json.RawMessage(`[]`),
		EffectiveConfig:     cfg,
	}); err == nil || !strings.Contains(err.Error(), "must decode to an object") {
		t.Fatalf("expected invalid requested params error, got %v", err)
	}

}

func TestMutationHelpersAndClosedDBErrors(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)
	if _, err := store.runRecordCapabilities(); err != nil {
		t.Fatalf("runRecordCapabilities failed: %v", err)
	}

	if got := nullableTimeUnixNano(nil); got != nil {
		t.Fatalf("nullableTimeUnixNano(nil) = %#v, want nil", got)
	}
	zero := time.Time{}
	if got := nullableTimeUnixNano(&zero); got != nil {
		t.Fatalf("nullableTimeUnixNano(zero) = %#v, want nil", got)
	}
	now := time.Now()
	if got := nullableTimeUnixNano(&now); got == nil {
		t.Fatal("expected non-nil unix nanos for non-zero time")
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if err := store.InsertRun(&AnalysisRun{RunID: "closed", CreatedAt: time.Now(), SourceType: "pcap", SensorID: "sensor", Status: "running"}); err == nil {
		t.Fatal("expected InsertRun error on closed db")
	}
	if err := store.UpdateTrackLabel("run", "track", "car", "good", 1, "labeler", "human_manual"); err == nil {
		t.Fatal("expected UpdateTrackLabel error on closed db")
	}
	if err := store.UpdateTrackQualityFlags("run", "track", true, false, []string{"other"}); err == nil {
		t.Fatal("expected UpdateTrackQualityFlags error on closed db")
	}
}

func TestRunRecordCapabilitiesAndScanAnalysisRunRecord(t *testing.T) {
	fullDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(fullDB.DB)
	caps, err := store.runRecordCapabilities()
	if err != nil {
		t.Fatalf("runRecordCapabilities failed: %v", err)
	}
	if !caps.RunConfigID || !caps.RequestedParamSetID || !caps.ReplayCaseID || !caps.CompletedAt || !caps.FrameStartNs || !caps.FrameEndNs || !caps.StatisticsJSON {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}

	closedDB, closedCleanup := dbpkg.NewTestDB(t)
	defer closedCleanup()
	closedStore := NewAnalysisRunStore(closedDB.DB)
	if err := closedDB.DB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if _, err := closedStore.runRecordCapabilities(); err == nil || !strings.Contains(err.Error(), "inspect lidar_run_records schema") {
		t.Fatalf("expected schema inspect error, got %v", err)
	}

	overrideDB, overrideCleanup := dbpkg.NewTestDB(t)
	defer overrideCleanup()
	overrideStore := NewAnalysisRunStore(&interceptSQLiteDB{
		db: overrideDB.DB,
		queryFn: func(query string, args []any) (*sql.Rows, error) {
			if strings.Contains(query, "PRAGMA table_info") {
				return overrideDB.DB.Query(`SELECT 1`)
			}
			return overrideDB.DB.Query(query, args...)
		},
	})
	if _, err := overrideStore.runRecordCapabilities(); err == nil || !strings.Contains(err.Error(), "scan lidar_run_records schema") {
		t.Fatalf("expected schema scan error, got %v", err)
	}

	completedAt := sql.NullInt64{Int64: time.Now().UnixNano(), Valid: true}
	frameStart := sql.NullInt64{Int64: 11, Valid: true}
	frameEnd := sql.NullInt64{Int64: 22, Valid: true}
	statsJSON := sql.NullString{String: `{"score":0.9}`, Valid: true}
	run, err := scanAnalysisRunRecord(scannerStub{
		values: []any{
			"run-1",               // RunID
			time.Now().UnixNano(), // createdAt
			"pcap",                // SourceType
			sql.NullString{String: "/tmp/case-1.pcap", Valid: true}, // sourcePath
			"sensor-1",  // SensorID
			1.5,         // DurationSecs
			10,          // TotalFrames
			2,           // TotalClusters
			3,           // TotalTracks
			4,           // ConfirmedTracks
			int64(50),   // ProcessingTimeMs
			"completed", // Status
			sql.NullString{String: "boom", Valid: true},            // errorMessage
			sql.NullString{String: "parent-1", Valid: true},        // parentRunID
			sql.NullString{String: "notes", Valid: true},           // notes
			sql.NullString{String: "/tmp/test.vrlog", Valid: true}, // vrlogPath
			sql.NullString{String: "run-config-1", Valid: true},    // runConfigID
			sql.NullString{String: "requested-set-1", Valid: true}, // requestedParamSetID
			sql.NullString{String: "scene-1", Valid: true},         // replayCaseID
			completedAt, // completedAt
			frameStart,  // frameStartNs
			frameEnd,    // frameEndNs
			statsJSON,   // statisticsJSON
		},
	}, analysisRunRecordCapabilities{
		RunConfigID:         true,
		RequestedParamSetID: true,
		ReplayCaseID:        true,
		CompletedAt:         true,
		FrameStartNs:        true,
		FrameEndNs:          true,
		StatisticsJSON:      true,
	})
	if err != nil {
		t.Fatalf("scanAnalysisRunRecord failed: %v", err)
	}
	if run.RunConfigID != "run-config-1" || run.RequestedParamSetID != "requested-set-1" || run.ReplayCaseID != "scene-1" {
		t.Fatalf("unexpected run identities: %+v", run)
	}
	if run.CompletedAt == nil || run.FrameStartNs == nil || run.FrameEndNs == nil {
		t.Fatalf("expected completed/frame bounds: %+v", run)
	}
	if run.ReplayCaseName != "case-1" {
		t.Fatalf("ReplayCaseName = %q, want case-1", run.ReplayCaseName)
	}
	if string(run.StatisticsJSON) != `{"score":0.9}` {
		t.Fatalf("StatisticsJSON = %s", run.StatisticsJSON)
	}

	if _, err := scanAnalysisRunRecord(scannerStub{err: errors.New("scan failed")}, analysisRunRecordCapabilities{}); err == nil || !strings.Contains(err.Error(), "scan failed") {
		t.Fatalf("expected scan failure, got %v", err)
	}
}

func TestAnalysisRunQueryErrorBranchesAndHydration(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(testDB.DB)
	cfg := cfgpkg.MustLoadDefaultConfig()
	paramSet, err := configasset.MakeEffectiveParamSet(cfg)
	if err != nil {
		t.Fatalf("MakeEffectiveParamSet failed: %v", err)
	}
	configStore := configasset.NewStore(testDB.DB)
	runConfig, err := configStore.EnsureRunConfig(paramSet, configasset.BuildIdentity{BuildVersion: "v1", BuildGitSHA: "sha"})
	if err != nil {
		t.Fatalf("EnsureRunConfig failed: %v", err)
	}

	run := &AnalysisRun{
		RunID:       "hydrated-run",
		CreatedAt:   time.Now(),
		SourceType:  "pcap",
		SourcePath:  "/tmp/hydrated-run.pcap",
		SensorID:    "sensor-1",
		Status:      "completed",
		RunConfigID: runConfig.RunConfigID,
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	got, err := store.GetRun("hydrated-run")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if got.ConfigHash == "" || got.ParamsHash == "" || len(got.ExecutionConfig) == 0 {
		t.Fatalf("expected hydrated immutable config fields, got %+v", got)
	}

	store.hydrateRunConfigAssets(&AnalysisRun{RunConfigID: "missing"})

	legacyDB, legacyCleanup := setupLegacyAnalysisRunTestDB(t)
	defer legacyCleanup()
	legacyStore := NewAnalysisRunStore(legacyDB)
	total, labelled, byClass, rollup, err := legacyStore.GetLabelingProgressWithRollup("run-1")
	if err != nil {
		t.Fatalf("GetLabelingProgressWithRollup on legacy schema failed: %v", err)
	}
	if total != 0 || labelled != 0 || rollup != nil || len(byClass) != 0 {
		t.Fatalf("expected empty progress on legacy schema, got total=%d labelled=%d rollup=%#v byClass=%#v", total, labelled, rollup, byClass)
	}

	if _, err := store.runRecordCapabilities(); err != nil {
		t.Fatalf("runRecordCapabilities failed: %v", err)
	}
	if err := testDB.DB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if _, err := store.GetRun("hydrated-run"); err == nil {
		t.Fatal("expected GetRun error on closed db")
	}
	if _, err := store.ListRuns(10); err == nil {
		t.Fatal("expected ListRuns error on closed db")
	}
	if _, err := store.GetRunTracks("hydrated-run"); err == nil {
		t.Fatal("expected GetRunTracks error on closed db")
	}
	if _, err := store.GetRunTrack("hydrated-run", "track-1"); err == nil {
		t.Fatal("expected GetRunTrack error on closed db")
	}
	if _, _, _, _, err := store.GetLabelingProgressWithRollup("hydrated-run"); err == nil {
		t.Fatal("expected GetLabelingProgressWithRollup error on closed db")
	}
	if _, err := store.GetRunLabelRollup("hydrated-run"); err == nil {
		t.Fatal("expected GetRunLabelRollup error on closed db")
	}
	if err := store.populateRunLabelRollups([]*AnalysisRun{{RunID: "hydrated-run"}}); err == nil {
		t.Fatal("expected populateRunLabelRollups error on closed db")
	}
	if _, err := store.GetUnlabeledTracks("hydrated-run", 10); err == nil {
		t.Fatal("expected GetUnlabeledTracks error on closed db")
	}

	store.hydrateRunConfigAssets(&AnalysisRun{RunConfigID: runConfig.RunConfigID})
}

func TestAnalysisRunQueries_SpecificErrorBranches(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(testDB.DB)
	if err := store.InsertRun(&AnalysisRun{
		RunID:      "run-errors",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		Status:     "completed",
	}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	if _, err := testDB.Exec(`DROP TABLE lidar_run_tracks`); err != nil {
		t.Fatalf("drop lidar_run_tracks: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE lidar_run_tracks (run_id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create malformed lidar_run_tracks: %v", err)
	}
	if _, err := store.GetRun("run-errors"); err == nil || !strings.Contains(err.Error(), "get run label rollup") {
		t.Fatalf("expected GetRun label rollup error, got %v", err)
	}

	typedDB, typedCleanup := dbpkg.NewTestDB(t)
	defer typedCleanup()
	if _, err := typedDB.Exec(`
		INSERT INTO lidar_run_records (
			run_id, created_at, source_type, sensor_id, status
		) VALUES (?, ?, ?, ?, ?)
	`, "run-typed", "not-a-timestamp", "pcap", "sensor-1", "completed"); err != nil {
		t.Fatalf("insert typed run: %v", err)
	}
	typedStore := NewAnalysisRunStore(typedDB.DB)
	if _, err := typedStore.ListRuns(10); err == nil || !strings.Contains(err.Error(), "scan run") {
		t.Fatalf("expected ListRuns scan error, got %v", err)
	}

	tracksDB, tracksCleanup := dbpkg.NewTestDB(t)
	defer tracksCleanup()
	trackStore := NewAnalysisRunStore(tracksDB.DB)
	if err := trackStore.InsertRun(&AnalysisRun{
		RunID:      "run-tracks",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		Status:     "completed",
	}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}
	if _, err := tracksDB.Exec(`
		INSERT INTO lidar_run_tracks (
			run_id, track_id, sensor_id, track_state, start_unix_nanos,
			observation_count, avg_speed_mps, max_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "run-tracks", "track-bad", "sensor-1", "confirmed", 1, "not-an-int", 1.2, 1.3, 1, 1, 1, 1, 1); err != nil {
		t.Fatalf("insert malformed track: %v", err)
	}
	if _, err := trackStore.GetRunTracks("run-tracks"); err == nil || !strings.Contains(err.Error(), "scan run track") {
		t.Fatalf("expected GetRunTracks scan error, got %v", err)
	}

	unlabeledDB, unlabeledCleanup := dbpkg.NewTestDB(t)
	defer unlabeledCleanup()
	unlabeledStore := NewAnalysisRunStore(unlabeledDB.DB)
	if err := unlabeledStore.InsertRun(&AnalysisRun{
		RunID:      "run-unlabeled-rich",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		Status:     "completed",
	}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}
	if _, err := unlabeledDB.Exec(`
		INSERT INTO lidar_run_tracks (
			run_id, track_id, sensor_id, track_state, start_unix_nanos,
			observation_count, avg_speed_mps, max_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg, user_label, label_confidence, labeler_id,
			labeled_at, quality_label, label_source, is_split_candidate, is_merge_candidate,
			linked_track_ids
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "run-unlabeled-rich", "track-rich", "sensor-1", "confirmed", 1, 4, 1.2, 1.3, 1, 1, 1, 1, 1, "   ", 0.8, "labeler-1", 123, "noisy", "human_manual", true, false, `["other"]`); err != nil {
		t.Fatalf("insert rich unlabeled track: %v", err)
	}
	tracks, err := unlabeledStore.GetUnlabeledTracks("run-unlabeled-rich", 10)
	if err != nil {
		t.Fatalf("GetUnlabeledTracks failed: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	if tracks[0].UserLabel != "   " || tracks[0].LabelConfidence != 0.8 || tracks[0].LabelerID != "labeler-1" || tracks[0].LabeledAt != 123 || tracks[0].QualityLabel != "noisy" || tracks[0].LabelSource != "human_manual" {
		t.Fatalf("expected optional unlabeled track fields to be populated, got %+v", tracks[0])
	}

	queryBaseDB, queryBaseCleanup := dbpkg.NewTestDB(t)
	defer queryBaseCleanup()
	baseStore := NewAnalysisRunStore(queryBaseDB.DB)
	if err := baseStore.InsertRun(&AnalysisRun{
		RunID:      "run-label-progress",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		Status:     "completed",
	}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}
	if err := baseStore.InsertRunTrack(&RunTrack{
		RunID:   "run-label-progress",
		TrackID: "track-1",
		TrackMeasurement: TrackMeasurement{
			SensorID:         "sensor-1",
			TrackState:       "confirmed",
			StartUnixNanos:   1,
			ObservationCount: 1,
		},
		UserLabel:   "car",
		LabelSource: "human_manual",
	}); err != nil {
		t.Fatalf("InsertRunTrack failed: %v", err)
	}

	erroringLabelStore := NewAnalysisRunStore(&interceptSQLiteDB{
		db: queryBaseDB.DB,
		queryFn: func(query string, args []any) (*sql.Rows, error) {
			if strings.Contains(query, "GROUP BY") {
				return nil, errors.New("label counts exploded")
			}
			return queryBaseDB.DB.Query(query, args...)
		},
	})
	if _, _, _, _, err := erroringLabelStore.GetLabelingProgressWithRollup("run-label-progress"); err == nil || !strings.Contains(err.Error(), "get label counts") {
		t.Fatalf("expected label-count query error, got %v", err)
	}

	scanFailLabelStore := NewAnalysisRunStore(&interceptSQLiteDB{
		db: queryBaseDB.DB,
		queryFn: func(query string, args []any) (*sql.Rows, error) {
			if strings.Contains(query, "GROUP BY") {
				return queryBaseDB.DB.Query(`SELECT 'car', 'bad-count'`)
			}
			return queryBaseDB.DB.Query(query, args...)
		},
	})
	if _, _, _, _, err := scanFailLabelStore.GetLabelingProgressWithRollup("run-label-progress"); err == nil || !strings.Contains(err.Error(), "scan label count") {
		t.Fatalf("expected label-count scan error, got %v", err)
	}

	missingTracksLabelStore := NewAnalysisRunStore(&interceptSQLiteDB{
		db: queryBaseDB.DB,
		queryFn: func(query string, args []any) (*sql.Rows, error) {
			if strings.Contains(query, "GROUP BY") {
				return nil, errors.New("no such table: lidar_run_tracks")
			}
			return queryBaseDB.DB.Query(query, args...)
		},
		queryRowFn: func(query string, args []any) *sql.Row {
			if strings.Contains(query, "COUNT(*) as total") {
				return queryBaseDB.DB.QueryRow(`SELECT 3, 1, 1`)
			}
			return queryBaseDB.DB.QueryRow(query, args...)
		},
	})
	total, labelled, byClass, rollup, err := missingTracksLabelStore.GetLabelingProgressWithRollup("run-label-progress")
	if err != nil {
		t.Fatalf("expected missing-table label counts to be ignored, got %v", err)
	}
	if total != 3 || labelled != 2 || rollup == nil || len(byClass) != 0 {
		t.Fatalf("unexpected missing-table labeling result: total=%d labelled=%d rollup=%#v byClass=%#v", total, labelled, rollup, byClass)
	}

	clampedRollupStore := NewAnalysisRunStore(&interceptSQLiteDB{
		db: queryBaseDB.DB,
		queryRowFn: func(query string, args []any) *sql.Row {
			if strings.Contains(query, "COUNT(*) as total") {
				return queryBaseDB.DB.QueryRow(`SELECT 1, 2, 2`)
			}
			return queryBaseDB.DB.QueryRow(query, args...)
		},
	})
	rollup, err = clampedRollupStore.GetRunLabelRollup("run-label-progress")
	if err != nil {
		t.Fatalf("GetRunLabelRollup failed: %v", err)
	}
	if rollup.Unlabelled != 0 {
		t.Fatalf("expected clamped unlabelled count, got %+v", rollup)
	}

	populateScanStore := NewAnalysisRunStore(&interceptSQLiteDB{
		db: queryBaseDB.DB,
		queryFn: func(query string, args []any) (*sql.Rows, error) {
			if strings.Contains(query, "GROUP BY run_id") {
				return queryBaseDB.DB.Query(`SELECT 'run-label-progress', 'bad-total', 1, 1`)
			}
			return queryBaseDB.DB.Query(query, args...)
		},
	})
	if err := populateScanStore.populateRunLabelRollups([]*AnalysisRun{{RunID: "run-label-progress"}}); err == nil || !strings.Contains(err.Error(), "scan run label rollup") {
		t.Fatalf("expected populateRunLabelRollups scan error, got %v", err)
	}

	populateClampStore := NewAnalysisRunStore(&interceptSQLiteDB{
		db: queryBaseDB.DB,
		queryFn: func(query string, args []any) (*sql.Rows, error) {
			if strings.Contains(query, "GROUP BY run_id") {
				return queryBaseDB.DB.Query(`SELECT 'run-label-progress', 1, 2, 2`)
			}
			return queryBaseDB.DB.Query(query, args...)
		},
	})
	runs := []*AnalysisRun{{RunID: "run-label-progress"}}
	if err := populateClampStore.populateRunLabelRollups(runs); err != nil {
		t.Fatalf("populateRunLabelRollups failed: %v", err)
	}
	if runs[0].LabelRollup == nil || runs[0].LabelRollup.Unlabelled != 0 {
		t.Fatalf("expected clamped populated rollup, got %#v", runs[0].LabelRollup)
	}

	scanFailUnlabeledStore := NewAnalysisRunStore(&interceptSQLiteDB{
		db: queryBaseDB.DB,
		queryFn: func(query string, args []any) (*sql.Rows, error) {
			if strings.Contains(query, "ORDER BY observation_count DESC") {
				return queryBaseDB.DB.Query(`SELECT 'run-label-progress', 'track-1', 'sensor-1', 'frame-1', 'confirmed', 1, 'bad-observation-count', 1, 1, 1, 1, 1, 1, 1, NULL, NULL, NULL, NULL, NULL, NULL, 0, 0, '[]'`)
			}
			return queryBaseDB.DB.Query(query, args...)
		},
	})
	if _, err := scanFailUnlabeledStore.GetUnlabeledTracks("run-label-progress", 10); err == nil || !strings.Contains(err.Error(), "scan unlabeled track") {
		t.Fatalf("expected unlabeled-track scan error, got %v", err)
	}
}

func TestAnalysisRunQueryHelpers_IterAndPopulateErrors(t *testing.T) {
	if _, err := readRunRecordCapabilitiesRows(&rowsStub{iterErr: errors.New("schema iter failed")}); err == nil || !strings.Contains(err.Error(), "iterate lidar_run_records schema") {
		t.Fatalf("expected schema iteration error, got %v", err)
	}

	if _, err := collectAnalysisRunRecords(&rowsStub{iterErr: errors.New("run iter failed")}, analysisRunRecordCapabilities{}); err == nil || !strings.Contains(err.Error(), "iterate runs") {
		t.Fatalf("expected run iteration error, got %v", err)
	}

	if _, err := finalizeAnalysisRunRecords(
		[]*AnalysisRun{{RunID: "run-1"}},
		func(*AnalysisRun) {},
		func([]*AnalysisRun) error { return errors.New("populate failed") },
	); err == nil || !strings.Contains(err.Error(), "populate failed") {
		t.Fatalf("expected populate failure, got %v", err)
	}

	if _, err := collectRunTracks(&rowsStub{iterErr: errors.New("track iter failed")}); err == nil || !strings.Contains(err.Error(), "iterate run tracks") {
		t.Fatalf("expected track iteration error, got %v", err)
	}

	if err := assignRunLabelRollups(&rowsStub{iterErr: errors.New("rollup iter failed")}, map[string]*AnalysisRun{
		"run-1": {RunID: "run-1"},
	}); err == nil || !strings.Contains(err.Error(), "iterate run label rollups") {
		t.Fatalf("expected rollup iteration error, got %v", err)
	}

	if _, err := collectUnlabeledTracks(&rowsStub{iterErr: errors.New("unlabeled iter failed")}); err == nil || !strings.Contains(err.Error(), "iterate unlabeled tracks") {
		t.Fatalf("expected unlabeled iteration error, got %v", err)
	}
}

func TestAnalysisRunStore_RunRecordSelectColumnsErrors(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	if err := testDB.DB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	getStore := NewAnalysisRunStore(testDB.DB)
	if _, err := getStore.GetRun("run-1"); err == nil || !strings.Contains(err.Error(), "inspect lidar_run_records schema") {
		t.Fatalf("expected GetRun schema error, got %v", err)
	}

	listStore := NewAnalysisRunStore(testDB.DB)
	if _, err := listStore.ListRuns(10); err == nil || !strings.Contains(err.Error(), "inspect lidar_run_records schema") {
		t.Fatalf("expected ListRuns schema error, got %v", err)
	}
}

func TestBackfillImmutableRunConfigReferences_DryRunAndErrors(t *testing.T) {
	if _, err := BackfillImmutableRunConfigReferences(nil, false); err == nil || !strings.Contains(err.Error(), "database is required") {
		t.Fatalf("expected nil-db error, got %v", err)
	}

	// After migration 000036, legacy columns are dropped, so backfill is a no-op.
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	result, err := BackfillImmutableRunConfigReferences(testDB.DB, true)
	if err != nil {
		t.Fatalf("dry-run backfill failed: %v", err)
	}
	if result.RunsUpdated != 0 || result.ReplayCasesUpdated != 0 {
		t.Fatalf("expected no updates (columns dropped), got: %+v", result)
	}
}

func TestReplayCaseStore_RecommendedParamSetPaths(t *testing.T) {
	fullDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewReplayCaseStore(fullDB.DB)
	scene := &ReplayCase{
		ReplayCaseID:      "scene-full",
		SensorID:          "sensor-1",
		PCAPFile:          "full.pcap",
		OptimalParamsJSON: json.RawMessage(`{"tracking":{"max_tracks":64}}`),
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("InsertScene failed: %v", err)
	}
	if scene.RecommendedParamSetID == "" || scene.RecommendedParamsHash == "" || len(scene.RecommendedParams) == 0 {
		t.Fatalf("expected recommended param set hydration, got %+v", scene)
	}

	emptyScene := &ReplayCase{
		ReplayCaseID:      "scene-empty",
		SensorID:          "sensor-1",
		PCAPFile:          "empty.pcap",
		OptimalParamsJSON: json.RawMessage(" "),
	}
	if err := store.InsertScene(emptyScene); err != nil {
		t.Fatalf("InsertScene(empty) failed: %v", err)
	}
	if emptyScene.RecommendedParamSetID != "" {
		t.Fatalf("expected blank recommended param set id, got %q", emptyScene.RecommendedParamSetID)
	}

	badScene := &ReplayCase{
		ReplayCaseID:      "scene-bad",
		SensorID:          "sensor-1",
		PCAPFile:          "bad.pcap",
		OptimalParamsJSON: json.RawMessage(`[]`),
	}
	if err := store.InsertScene(badScene); err == nil || !strings.Contains(err.Error(), "canonicalize recommended params") {
		t.Fatalf("expected invalid optimal params error, got %v", err)
	}

	if got := nullFloat64(nil); got != nil {
		t.Fatalf("nullFloat64(nil) = %#v, want nil", got)
	}
	f := 1.5
	if got := nullFloat64(&f); got.(float64) != 1.5 {
		t.Fatalf("nullFloat64(&f) = %#v", got)
	}
	if got := nullInt64(nil); got != nil {
		t.Fatalf("nullInt64(nil) = %#v, want nil", got)
	}
	i := int64(7)
	if got := nullInt64(&i); got.(int64) != 7 {
		t.Fatalf("nullInt64(&i) = %#v", got)
	}
}

func TestReplayCaseStore_HelperAndRowsAffectedBranches(t *testing.T) {
	t.Run("normalize nil scene", func(t *testing.T) {
		store := NewReplayCaseStore(&interceptSQLiteDB{})
		if err := store.normalizeRecommendedParamSet(nil); err == nil || !strings.Contains(err.Error(), "scene is required") {
			t.Fatalf("expected nil-scene error, got %v", err)
		}
	})

	t.Run("normalize store error", func(t *testing.T) {
		testDB, cleanup := dbpkg.NewTestDB(t)
		defer cleanup()

		store := NewReplayCaseStore(&interceptSQLiteDB{
			db: testDB.DB,
			execFn: func(query string, args []any) (sql.Result, error) {
				if strings.Contains(query, "INSERT OR IGNORE INTO lidar_param_sets") {
					return nil, errors.New("param-set insert failed")
				}
				return testDB.DB.Exec(query, args...)
			},
		})
		scene := &ReplayCase{
			ReplayCaseID:      "scene-store-error",
			SensorID:          "sensor-1",
			PCAPFile:          "scene.pcap",
			OptimalParamsJSON: json.RawMessage(`{"tracking":{"max_tracks":32}}`),
		}
		if err := store.normalizeRecommendedParamSet(scene); err == nil || !strings.Contains(err.Error(), "store recommended params") {
			t.Fatalf("expected normalize store error, got %v", err)
		}
	})

	t.Run("hydrate ignores missing param set and db errors", func(t *testing.T) {
		testDB, cleanup := dbpkg.NewTestDB(t)
		defer cleanup()

		scene := &ReplayCase{RecommendedParamSetID: "missing-param-set"}
		NewReplayCaseStore(testDB.DB).hydrateRecommendedParamSet(scene)
		if scene.RecommendedParamsHash != "" || len(scene.RecommendedParams) != 0 {
			t.Fatalf("expected missing param set to leave scene untouched, got %+v", scene)
		}

		missingSchemaDB := setupReplayCaseRecommendedDB(t)
		defer missingSchemaDB.Close()
		scene = &ReplayCase{RecommendedParamSetID: "missing-schema"}
		NewReplayCaseStore(missingSchemaDB).hydrateRecommendedParamSet(scene)
		if scene.RecommendedParamsHash != "" || len(scene.RecommendedParams) != 0 {
			t.Fatalf("expected missing schema to leave scene untouched, got %+v", scene)
		}

		closedDB, closedCleanup := dbpkg.NewTestDB(t)
		defer closedCleanup()
		if err := closedDB.DB.Close(); err != nil {
			t.Fatalf("close db: %v", err)
		}
		scene = &ReplayCase{RecommendedParamSetID: "closed-db"}
		NewReplayCaseStore(closedDB.DB).hydrateRecommendedParamSet(scene)
		if scene.RecommendedParamsHash != "" || len(scene.RecommendedParams) != 0 {
			t.Fatalf("expected closed db to leave scene untouched, got %+v", scene)
		}
	})

	t.Run("rows affected errors", func(t *testing.T) {
		testDB, cleanup := dbpkg.NewTestDB(t)
		defer cleanup()

		baseStore := NewReplayCaseStore(testDB.DB)
		if err := baseStore.InsertScene(&ReplayCase{
			ReplayCaseID: "scene-rows-affected",
			SensorID:     "sensor-1",
			PCAPFile:     "rows.pcap",
		}); err != nil {
			t.Fatalf("InsertScene failed: %v", err)
		}

		updateStore := NewReplayCaseStore(&interceptSQLiteDB{
			db: testDB.DB,
			execFn: func(query string, args []any) (sql.Result, error) {
				if strings.Contains(query, "UPDATE lidar_replay_cases") {
					return rowsAffectedErrorResult{err: errors.New("rows failed")}, nil
				}
				return testDB.DB.Exec(query, args...)
			},
		})
		if err := updateStore.UpdateScene(&ReplayCase{ReplayCaseID: "scene-rows-affected"}); err == nil || !strings.Contains(err.Error(), "check update result") {
			t.Fatalf("expected UpdateScene RowsAffected error, got %v", err)
		}

		deleteStore := NewReplayCaseStore(&interceptSQLiteDB{
			db: testDB.DB,
			execFn: func(query string, args []any) (sql.Result, error) {
				if strings.Contains(query, "DELETE FROM lidar_replay_cases") {
					return rowsAffectedErrorResult{err: errors.New("rows failed")}, nil
				}
				return testDB.DB.Exec(query, args...)
			},
		})
		if err := deleteStore.DeleteScene("scene-rows-affected"); err == nil || !strings.Contains(err.Error(), "check delete result") {
			t.Fatalf("expected DeleteScene RowsAffected error, got %v", err)
		}

		refStore := NewReplayCaseStore(&interceptSQLiteDB{
			db: testDB.DB,
			execFn: func(query string, args []any) (sql.Result, error) {
				if strings.Contains(query, "SET reference_run_id") {
					return rowsAffectedErrorResult{err: errors.New("rows failed")}, nil
				}
				return testDB.DB.Exec(query, args...)
			},
		})
		if err := refStore.SetReferenceRun("scene-rows-affected", "run-1"); err == nil || !strings.Contains(err.Error(), "check update result") {
			t.Fatalf("expected SetReferenceRun RowsAffected error, got %v", err)
		}
	})

	t.Run("update normalize error", func(t *testing.T) {
		testDB, cleanup := dbpkg.NewTestDB(t)
		defer cleanup()

		store := NewReplayCaseStore(testDB.DB)
		err := store.UpdateScene(&ReplayCase{
			ReplayCaseID:      "scene-update-normalize-error",
			OptimalParamsJSON: json.RawMessage(`[]`),
		})
		if err == nil || !strings.Contains(err.Error(), "canonicalize recommended params") {
			t.Fatalf("expected UpdateScene normalize error, got %v", err)
		}
	})
}

func TestReplayCaseStore_ErrorBranches(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	closedStore := NewReplayCaseStore(testDB.DB)
	if _, err := testDB.DB.Exec(`DROP TABLE lidar_replay_cases`); err != nil {
		t.Fatalf("drop replay cases: %v", err)
	}
	if _, err := testDB.DB.Exec(`CREATE TABLE lidar_replay_cases (replay_case_id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create malformed replay cases: %v", err)
	}
	if _, err := closedStore.GetScene("scene-1"); err == nil || !strings.Contains(err.Error(), "get scene") {
		t.Fatalf("expected GetScene query error, got %v", err)
	}

	baseDB, baseCleanup := dbpkg.NewTestDB(t)
	defer baseCleanup()
	baseStore := NewReplayCaseStore(baseDB.DB)
	if err := baseStore.InsertScene(&ReplayCase{
		ReplayCaseID: "scene-update",
		SensorID:     "sensor-1",
		PCAPFile:     "update.pcap",
	}); err != nil {
		t.Fatalf("InsertScene failed: %v", err)
	}

	updateErrStore := NewReplayCaseStore(&interceptSQLiteDB{
		db: baseDB.DB,
		execFn: func(query string, args []any) (sql.Result, error) {
			if strings.Contains(query, "UPDATE lidar_replay_cases") {
				return nil, errors.New("update failed")
			}
			return baseDB.DB.Exec(query, args...)
		},
	})
	if err := updateErrStore.UpdateScene(&ReplayCase{ReplayCaseID: "scene-update"}); err == nil || !strings.Contains(err.Error(), "update scene") {
		t.Fatalf("expected UpdateScene exec error, got %v", err)
	}

	insertErrStore := NewReplayCaseStore(&interceptSQLiteDB{
		db: baseDB.DB,
		execFn: func(query string, args []any) (sql.Result, error) {
			if strings.Contains(query, "INSERT INTO lidar_replay_cases") {
				return nil, errors.New("insert failed")
			}
			return baseDB.DB.Exec(query, args...)
		},
	})
	if err := insertErrStore.InsertScene(&ReplayCase{
		ReplayCaseID: "scene-insert-err",
		SensorID:     "sensor-1",
		PCAPFile:     "insert.pcap",
	}); err == nil || !strings.Contains(err.Error(), "insert scene") {
		t.Fatalf("expected InsertScene exec error, got %v", err)
	}

	queryErrStore := NewReplayCaseStore(&interceptSQLiteDB{
		db: baseDB.DB,
		queryFn: func(query string, args []any) (*sql.Rows, error) {
			if strings.Contains(query, "ORDER BY created_at_ns DESC") {
				return nil, errors.New("list failed")
			}
			return baseDB.DB.Query(query, args...)
		},
	})
	if _, err := queryErrStore.ListScenes(""); err == nil || !strings.Contains(err.Error(), "list scenes") {
		t.Fatalf("expected ListScenes query error, got %v", err)
	}

	scanErrStore := NewReplayCaseStore(&interceptSQLiteDB{
		db: baseDB.DB,
		queryFn: func(query string, args []any) (*sql.Rows, error) {
			if strings.Contains(query, "ORDER BY created_at_ns DESC") {
				return baseDB.DB.Query(`SELECT 'scene', 'sensor', 'pcap', 'bad-start', NULL, NULL, NULL, NULL, 1, NULL, NULL`)
			}
			return baseDB.DB.Query(query, args...)
		},
	})
	if _, err := scanErrStore.ListScenes(""); err == nil || !strings.Contains(err.Error(), "scan scene row") {
		t.Fatalf("expected ListScenes scan error, got %v", err)
	}

	refErrStore := NewReplayCaseStore(&interceptSQLiteDB{
		db: baseDB.DB,
		execFn: func(query string, args []any) (sql.Result, error) {
			if strings.Contains(query, "SET reference_run_id") {
				return nil, errors.New("set ref failed")
			}
			return baseDB.DB.Exec(query, args...)
		},
	})
	if err := refErrStore.SetReferenceRun("scene-update", "run-1"); err == nil || !strings.Contains(err.Error(), "set reference run") {
		t.Fatalf("expected SetReferenceRun exec error, got %v", err)
	}

	deleteErrStore := NewReplayCaseStore(&interceptSQLiteDB{
		db: baseDB.DB,
		execFn: func(query string, args []any) (sql.Result, error) {
			if strings.Contains(query, "DELETE FROM lidar_replay_cases") {
				return nil, errors.New("delete failed")
			}
			return baseDB.DB.Exec(query, args...)
		},
	})
	if err := deleteErrStore.DeleteScene("scene-update"); err == nil || !strings.Contains(err.Error(), "delete scene") {
		t.Fatalf("expected DeleteScene exec error, got %v", err)
	}
}
